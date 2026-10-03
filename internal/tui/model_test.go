package tui_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/netip"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
	"github.com/davidbz/lantern/internal/mocks"
	"github.com/davidbz/lantern/internal/tui"
)

const (
	width  = 160
	height = 30
)

type scanRecorder struct {
	calls int
}

func (r *scanRecorder) scan(domain.Inventory) tea.Cmd {
	r.calls++
	return func() tea.Msg { return nil }
}

func inventory(t *testing.T) domain.Inventory {
	t.Helper()

	mac := domain.MAC{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}
	seen := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	return domain.Inventory{
		Devices: map[domain.MAC]domain.Device{mac: {
			MAC:      mac,
			IPs:      []netip.Addr{netip.MustParseAddr("192.168.1.10"), netip.MustParseAddr("192.168.1.11")},
			Hostname: "nas.local",
			Vendor:   "Acme",
			Services: []string{"smb"},
			Sources:  domain.SourceARP | domain.SourceMDNS,
			LastSeen: seen, FirstSeen: seen,
		}},
		Unresolved: map[netip.Addr]domain.Device{
			netip.MustParseAddr("192.168.1.20"): {
				IPs: []netip.Addr{netip.MustParseAddr("192.168.1.20")}, Sources: domain.SourceMDNS, LastSeen: seen,
			},
		},
	}
}

func step(t *testing.T, model tea.Model, msg tea.Msg) (tui.Model, tea.Cmd) {
	t.Helper()

	updated, cmd := model.Update(msg)
	result, ok := updated.(tui.Model)
	require.True(t, ok)

	return result, cmd
}

func key(text string) tea.KeyMsg {
	if text == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	if text == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}

	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)}
}

func newModel(t *testing.T) (tui.Model, *scanRecorder) {
	t.Helper()

	recorder := &scanRecorder{}
	model := tui.NewModel(t.Context(), recorder.scan, &config.UIConfig{RefreshInterval: time.Hour})
	require.NotNil(t, model.Init())
	model, _ = step(t, model, tea.WindowSizeMsg{Width: width, Height: height})

	return model, recorder
}

func TestModel(t *testing.T) {
	t.Run("the first scan starts on init and shows progress", func(t *testing.T) {
		model, recorder := newModel(t)

		require.Equal(t, 1, recorder.calls)
		require.Contains(t, model.View(), "SCANNING")
		require.Contains(t, model.View(), "GET READY")
	})

	t.Run("a finished scan lists devices and schedules the next one", func(t *testing.T) {
		model, _ := newModel(t)

		model, cmd := step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		require.NotNil(t, cmd)
		view := model.View()
		require.Contains(t, view, "0002")
		require.Contains(t, view, "1ST")
		require.Contains(t, view, "2ND")
		require.Contains(t, view, "192.168.1.10 +1")
		require.Contains(t, view, "00:1a:2b:3c:4d:5e")
		require.Contains(t, view, "Acme")
		require.Contains(t, view, "nas.local")
		require.Contains(t, view, "arp, mdns")
		require.Contains(t, view, "LAST SCAN")
		require.Contains(t, view, "NEXT WAVE IN 1h0m0s")
	})

	t.Run("only the latest tick starts a scan", func(t *testing.T) {
		model, recorder := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{})

		model, _ = step(t, model, tui.TickMsg{Generation: 0})
		require.Equal(t, 1, recorder.calls, "stale tick is ignored")

		_, cmd := step(t, model, tui.TickMsg{Generation: 1})
		require.NotNil(t, cmd)
		require.Equal(t, 2, recorder.calls)
	})

	t.Run("rescan is ignored while a scan is running", func(t *testing.T) {
		model, recorder := newModel(t)

		model, _ = step(t, model, key("r"))
		require.Equal(t, 1, recorder.calls)

		model, _ = step(t, model, tui.ScanDoneMsg{})
		_, _ = step(t, model, key("r"))
		require.Equal(t, 2, recorder.calls)
	})

	t.Run("warnings and errors are shown as banners", func(t *testing.T) {
		model, _ := newModel(t)
		privileges := errors.Join(domain.ErrInsufficientPrivileges)

		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{
			Inventory: inventory(t),
			Warnings:  []error{privileges, privileges, domain.ErrSourceUnavailable},
		}})
		view := model.View()
		require.Contains(t, view, "setcap")
		require.Contains(t, view, domain.ErrSourceUnavailable.Error())

		model, _ = step(t, model, tui.ScanDoneMsg{Err: errors.New("network is down")})
		view = model.View()
		require.Contains(t, view, "TILT")
		require.Contains(t, view, "network is down")
		require.Contains(t, view, "nas.local", "a failed scan keeps the last inventory")
	})

	t.Run("sort cycles through the keys", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})
		require.Contains(t, model.View(), "IP ▼")

		model, _ = step(t, model, key("s"))
		require.Contains(t, model.View(), "VENDOR ▼")
		require.NotContains(t, model.View(), "IP ▼")
	})

	t.Run("enter shows details and esc goes back", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		model, _ = step(t, model, key("enter"))
		view := model.View()
		require.Contains(t, view, "FIRST SEEN")
		require.Contains(t, view, "PLAYER 01 OF 02")
		require.Contains(t, view, "192.168.1.10, 192.168.1.11")
		require.Contains(t, view, "[smb]")
		require.Contains(t, view, "◆ ARP  ◇ CACHE  ◆ MDNS")
		require.Contains(t, view, "ESC")

		model, _ = step(t, model, key("esc"))
		require.NotContains(t, model.View(), "FIRST SEEN")
	})

	t.Run("enter does nothing without devices", func(t *testing.T) {
		model, _ := newModel(t)

		model, _ = step(t, model, key("enter"))
		require.NotContains(t, model.View(), "FIRST SEEN")
	})

	t.Run("navigation keys move the selection", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		model, _ = step(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, _ = step(t, model, key("enter"))
		view := model.View()
		require.Contains(t, view, "IPS         192.168.1.20")
		require.Contains(t, view, "PLAYER 02 OF 02")
	})

	t.Run("q quits", func(t *testing.T) {
		model, _ := newModel(t)

		_, cmd := step(t, model, key("q"))

		require.Equal(t, tea.Quit(), cmd())
	})

	t.Run("the refresh tick carries the scan generation", func(t *testing.T) {
		recorder := &scanRecorder{}
		model := tui.NewModel(t.Context(), recorder.scan, &config.UIConfig{RefreshInterval: time.Nanosecond})

		_, cmd := step(t, model, tui.ScanDoneMsg{})

		require.Equal(t, tui.TickMsg{Generation: 1}, cmd())
	})

	t.Run("a single device without addresses", func(t *testing.T) {
		model, _ := newModel(t)
		mac := domain.MAC{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}

		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: domain.Inventory{
			Devices: map[domain.MAC]domain.Device{mac: {MAC: mac}},
		}}})

		view := model.View()
		require.Contains(t, view, "0001")
		require.Contains(t, view, "00:1a:2b:3c:4d:5e")

		model, _ = step(t, model, key("enter"))
		require.Contains(t, model.View(), "IPS         -")
	})

	t.Run("frames advance the animation and schedule the next frame", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{})
		now := time.Now()

		frames := make([]string, 0, 10)
		for i := range 10 {
			var cmd tea.Cmd
			model, cmd = step(t, model, tui.FrameMsg{Time: now.Add(time.Duration(i) * time.Minute)})
			require.NotNil(t, cmd)
			frames = append(frames, model.View())
		}

		require.NotEqual(t, frames[0], frames[len(frames)-1], "blinking text toggles")
		require.Contains(t, frames[len(frames)-1], "NEXT WAVE IN 51m0s")
		require.Contains(t, frames[len(frames)-1], "PRESS R TO TRY AGAIN", "blinks back on")
	})

	t.Run("the scanner sweeps back and forth", func(t *testing.T) {
		model, _ := newModel(t)

		seen := map[string]bool{}
		for range 30 {
			model, _ = step(t, model, tui.FrameMsg{Time: time.Now()})
			seen[strings.SplitN(model.View(), "\n", 4)[2]] = true
		}

		require.Greater(t, len(seen), 10, "the bar moves and the label blinks")
	})

	t.Run("an empty finished scan says so", func(t *testing.T) {
		model, _ := newModel(t)

		model, _ = step(t, model, tui.ScanDoneMsg{})

		require.Contains(t, model.View(), "NO DEVICES FOUND")
		require.Contains(t, model.View(), "0000")
	})

	t.Run("new arrivals are starred and the hi-score is kept", func(t *testing.T) {
		model, _ := newModel(t)
		inv := inventory(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inv}})
		require.NotContains(t, model.View(), "★", "nothing is new on the first scan")

		mac := domain.MAC{0x02, 0, 0, 0, 0, 0x01}
		inv.Devices[mac] = domain.Device{MAC: mac, FirstSeen: time.Now().Add(time.Hour), LastSeen: time.Now()}
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inv}})
		require.Contains(t, model.View(), "1ST★", "the new device has no IP, so it sorts first")
		require.Contains(t, model.View(), "HI-SCORE")

		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: domain.Inventory{}}})
		require.Contains(t, model.View(), "0003", "the hi-score outlives the devices")
	})

	t.Run("a long leaderboard ranks with ordinals", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tea.WindowSizeMsg{Width: width, Height: 60})
		devices := map[domain.MAC]domain.Device{}
		for i := range 23 {
			mac := domain.MAC{0x02, 0, 0, 0, 0, byte(i)}
			ip := netip.AddrFrom4([4]byte{10, 0, 0, byte(i + 1)})
			devices[mac] = domain.Device{MAC: mac, IPs: []netip.Addr{ip}}
		}

		model, _ = step(
			t,
			model,
			tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: domain.Inventory{Devices: devices}}},
		)

		view := model.View()
		for _, rank := range []string{"1ST", "2ND", "3RD", "4TH", "11TH", "12TH", "13TH", "21ST", "22ND", "23RD"} {
			require.Contains(t, view, " "+rank+" ")
		}
	})

	t.Run("details name a device by hostname, IP, MAC or nothing", func(t *testing.T) {
		mac := domain.MAC{0x00, 0x1a, 0x2b, 0x3c, 0x4d, 0x5e}
		ip := netip.MustParseAddr("192.168.1.30")
		for name, tc := range map[string]struct {
			inventory domain.Inventory
			want      string
		}{
			"by IP": {
				domain.Inventory{Unresolved: map[netip.Addr]domain.Device{ip: {IPs: []netip.Addr{ip}}}},
				"192.168.1.30",
			},
			"by MAC": {domain.Inventory{Devices: map[domain.MAC]domain.Device{mac: {MAC: mac}}}, "00:1a:2b:3c:4d:5e"},
			"unknown": {
				domain.Inventory{Unresolved: map[netip.Addr]domain.Device{{}: {}}},
				"UNKNOWN",
			},
		} {
			t.Run(name, func(t *testing.T) {
				model, _ := newModel(t)
				model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: tc.inventory}})

				model, _ = step(t, model, key("enter"))

				view := strings.ReplaceAll(model.View(), " ", "")
				require.Contains(t, view, "PLAYER01OF01")
				require.Contains(t, view, "║"+tc.want+"║", "the name heads the card on its own line")
			})
		}
	})

	t.Run("a narrow window swaps the logo for a wordmark", func(t *testing.T) {
		model, _ := newModel(t)

		model, _ = step(t, model, tea.WindowSizeMsg{Width: 60, Height: height})

		require.Contains(t, model.View(), "LANTERN")
	})

	t.Run("unknown messages are ignored", func(t *testing.T) {
		model, _ := newModel(t)

		_, cmd := step(t, model, struct{}{})

		require.Nil(t, cmd)
	})
}

func TestBindScanner(t *testing.T) {
	t.Run("runs the scan and reports the result", func(t *testing.T) {
		scanner := mocks.NewMockScanner(t)
		want := domain.ScanResult{Inventory: inventory(t)}
		scanner.EXPECT().Scan(mock.Anything, domain.Inventory{}).Return(want, nil)

		msg := tui.BindScanner(t.Context(), scanner)(domain.Inventory{})()

		require.Equal(t, tui.ScanDoneMsg{Result: want}, msg)
	})

	t.Run("reports scan errors", func(t *testing.T) {
		scanner := mocks.NewMockScanner(t)
		boom := errors.New("boom")
		scanner.EXPECT().Scan(mock.Anything, domain.Inventory{}).Return(domain.ScanResult{}, boom)

		msg := tui.BindScanner(t.Context(), scanner)(domain.Inventory{})()

		require.Equal(t, tui.ScanDoneMsg{Err: boom}, msg)
	})
}

func TestRun(t *testing.T) {
	newScanner := func(t *testing.T) *mocks.MockScanner {
		t.Helper()

		scanner := mocks.NewMockScanner(t)
		scanner.EXPECT().Scan(mock.Anything, mock.Anything).Return(domain.ScanResult{}, nil).Maybe()

		return scanner
	}
	cfg := &config.UIConfig{RefreshInterval: time.Hour}

	t.Run("returns when the user quits", func(t *testing.T) {
		var out bytes.Buffer

		err := tui.Run(t.Context(), newScanner(t), cfg, strings.NewReader("q"), &out)

		require.NoError(t, err)
		require.Contains(t, out.String(), "DEVICES")
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := tui.Run(ctx, newScanner(t), cfg, strings.NewReader(""), io.Discard)

		require.ErrorIs(t, err, context.Canceled)
	})
}
