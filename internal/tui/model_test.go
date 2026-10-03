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
		require.Contains(t, model.View(), "scanning")
	})

	t.Run("a finished scan lists devices and schedules the next one", func(t *testing.T) {
		model, _ := newModel(t)

		model, cmd := step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		require.NotNil(t, cmd)
		view := model.View()
		require.Contains(t, view, "2 devices")
		require.Contains(t, view, "192.168.1.10 +1")
		require.Contains(t, view, "00:1a:2b:3c:4d:5e")
		require.Contains(t, view, "Acme")
		require.Contains(t, view, "nas.local")
		require.Contains(t, view, "arp, mdns")
		require.Contains(t, view, "last scan")
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
		require.Contains(t, view, "network is down")
		require.Contains(t, view, "2 devices", "a failed scan keeps the last inventory")
	})

	t.Run("sort cycles through the keys", func(t *testing.T) {
		model, _ := newModel(t)

		model, _ = step(t, model, key("s"))
		require.Contains(t, model.View(), "sorted by vendor")
	})

	t.Run("enter shows details and esc goes back", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		model, _ = step(t, model, key("enter"))
		view := model.View()
		require.Contains(t, view, "First seen")
		require.Contains(t, view, "192.168.1.10, 192.168.1.11")

		model, _ = step(t, model, key("esc"))
		require.NotContains(t, model.View(), "First seen")
	})

	t.Run("enter does nothing without devices", func(t *testing.T) {
		model, _ := newModel(t)

		model, _ = step(t, model, key("enter"))
		require.NotContains(t, model.View(), "First seen")
	})

	t.Run("navigation keys move the selection", func(t *testing.T) {
		model, _ := newModel(t)
		model, _ = step(t, model, tui.ScanDoneMsg{Result: domain.ScanResult{Inventory: inventory(t)}})

		model, _ = step(t, model, tea.KeyMsg{Type: tea.KeyDown})
		model, _ = step(t, model, key("enter"))
		require.Contains(t, model.View(), "IPs         192.168.1.20")
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
		require.Contains(t, view, "1 device ")
		require.Contains(t, view, "00:1a:2b:3c:4d:5e")
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
		require.Contains(t, out.String(), "lantern")
	})

	t.Run("stops when the context is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := tui.Run(ctx, newScanner(t), cfg, strings.NewReader(""), io.Discard)

		require.ErrorIs(t, err, context.Canceled)
	})
}
