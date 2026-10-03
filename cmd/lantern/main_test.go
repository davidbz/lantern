package main_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/netif"
)

const (
	waitTimeout  = 15 * time.Second
	pollInterval = 50 * time.Millisecond
	registry     = "Registry,Assignment,Organization Name,Organization Address\n" +
		"MA-L,001A2B,Acme Corp,1 Road\n" +
		"MA-L,AABBCC,Example Inc,2 Road\n"
)

// build compiles lantern once per top-level test; the build cache keeps repeat builds fast.
func build(t *testing.T) string {
	t.Helper()

	binary := filepath.Join(t.TempDir(), "lantern")
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, ".")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	return binary
}

// syncBuffer collects a process's output while the test reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

type process struct {
	cmd    *exec.Cmd
	stdout *syncBuffer
	stderr *syncBuffer
}

// lantern prepares the binary with an isolated home, cache and working directory, so no .env or cached registry
// from the developer's machine leaks in.
func lantern(t *testing.T, binary string, env map[string]string, args ...string) process {
	t.Helper()

	home := t.TempDir()
	cmd := exec.CommandContext(t.Context(), binary, args...)
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"HOME=" + home, "XDG_CACHE_HOME=" + filepath.Join(home, ".cache")}
	for name, value := range env {
		cmd.Env = append(cmd.Env, name+"="+value)
	}

	var stdout, stderr syncBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	return process{cmd: cmd, stdout: &stdout, stderr: &stderr}
}

func TestFlags(t *testing.T) {
	binary := build(t)

	t.Run("--version prints the version", func(t *testing.T) {
		run := lantern(t, binary, nil, "--version")

		require.NoError(t, run.cmd.Run())
		require.Equal(t, "lantern dev\n", run.stdout.String())
	})

	t.Run("-h prints usage and exits cleanly", func(t *testing.T) {
		run := lantern(t, binary, nil, "-h")

		require.NoError(t, run.cmd.Run())
		require.Contains(t, run.stderr.String(), "-update-oui")
	})

	t.Run("an unknown flag fails", func(t *testing.T) {
		run := lantern(t, binary, nil, "--bogus")

		require.Error(t, run.cmd.Run())
		require.Contains(t, run.stderr.String(), "lantern:")
	})
}

func TestUpdateOUI(t *testing.T) {
	binary := build(t)

	t.Run("downloads the registry into the cache", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(registry))
		}))
		t.Cleanup(server.Close)
		cache := filepath.Join(t.TempDir(), "oui.csv")
		run := lantern(t, binary, map[string]string{"LANTERN_OUI_URL": server.URL, "LANTERN_OUI_CACHE": cache},
			"--update-oui")

		require.NoError(t, run.cmd.Run())
		require.Equal(t, "Saved 2 vendors to "+cache+"\n", run.stdout.String())
		saved, err := os.ReadFile(cache)
		require.NoError(t, err)
		require.Equal(t, registry, string(saved))
	})

	t.Run("a failed download fails without touching the cache", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)
		cache := filepath.Join(t.TempDir(), "oui.csv")
		run := lantern(t, binary, map[string]string{"LANTERN_OUI_URL": server.URL, "LANTERN_OUI_CACHE": cache},
			"--update-oui")

		require.Error(t, run.cmd.Run())
		require.Contains(t, run.stderr.String(), "500")
		require.NoFileExists(t, cache)
	})
}

// browseEnv points the neighbor source at a fixture listing one device on a real interface, and keeps the other
// sources short: ARP needs privileges the test usually lacks (a warning), and mDNS browses a service nobody offers.
func browseEnv(t *testing.T, ip, mac string) map[string]string {
	t.Helper()

	ifaces, err := netif.NewLister(t.Context()).List(t.Context())
	require.NoError(t, err)
	if len(ifaces) == 0 {
		t.Skip("no scannable network interface")
	}

	table := filepath.Join(t.TempDir(), "arp")
	content := "IP address       HW type     Flags       HW address            Mask     Device\n" +
		ip + "    0x1         0x2         " + mac + "     *        " + ifaces[0].Name + "\n"
	require.NoError(t, os.WriteFile(table, []byte(content), 0o600))

	return map[string]string{
		"LANTERN_NEIGHBOR_TABLE":      table,
		"LANTERN_MDNS_SERVICES":       "_lantern-e2e._tcp",
		"LANTERN_MDNS_WAIT":           "200ms",
		"LANTERN_ARP_REPLY_WAIT":      "200ms",
		"LANTERN_ARP_MIN_PREFIX_BITS": "30",
	}
}

func TestBrowse(t *testing.T) {
	const (
		ip  = "10.254.254.10"
		mac = "00:1a:2b:3c:4d:5e"
	)

	binary := build(t)
	start := func(t *testing.T) (process, io.WriteCloser) {
		t.Helper()

		run := lantern(t, binary, browseEnv(t, ip, mac))
		stdin, err := run.cmd.StdinPipe()
		require.NoError(t, err)
		require.NoError(t, run.cmd.Start())
		require.Eventually(t, func() bool {
			out := run.stdout.String()
			return strings.Contains(out, ip) && strings.Contains(out, mac)
		}, waitTimeout, pollInterval, "the fixture device never appeared")

		return run, stdin
	}

	t.Run("lists discovered devices and quits on q", func(t *testing.T) {
		run, stdin := start(t)

		require.Contains(t, run.stdout.String(), "cache")
		_, err := io.WriteString(stdin, "q")
		require.NoError(t, err)

		require.NoError(t, wait(t, run))
	})

	t.Run("exits cleanly on SIGTERM", func(t *testing.T) {
		run, _ := start(t)

		require.NoError(t, run.cmd.Process.Signal(syscall.SIGTERM))

		require.NoError(t, wait(t, run))
	})
}

func wait(t *testing.T, run process) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- run.cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-time.After(waitTimeout):
		require.Fail(t, "lantern did not exit", "stderr: %s", run.stderr.String())
		return nil
	}
}
