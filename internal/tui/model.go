// Package tui renders the device inventory as an interactive terminal table.
package tui

import (
	"context"
	"time"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/davidbz/lantern/internal/config"
	"github.com/davidbz/lantern/internal/domain"
)

const (
	keyQuit      = "q"
	keyInterrupt = "ctrl+c"
	keyRescan    = "r"
	keySort      = "s"
	keyDetails   = "enter"
	keyBack      = "esc"
)

// ScanFunc returns a command that scans, starting from prev, and reports a ScanDoneMsg.
type ScanFunc func(prev domain.Inventory) tea.Cmd

// ScanDoneMsg carries the outcome of one scan.
type ScanDoneMsg struct {
	Result domain.ScanResult
	Err    error
}

// TickMsg triggers the next periodic scan. Generation ties it to the scan that scheduled it, so a manual
// rescan doesn't start a second refresh cycle.
type TickMsg struct {
	Generation int
}

// Model is the UI state; Update is a reducer from (state, message) to (state, command).
type Model struct {
	scan    ScanFunc
	refresh time.Duration

	inventory  domain.Inventory
	devices    []domain.Device // inventory in display order
	warnings   []error
	err        error
	sortKey    domain.SortKey
	scanning   bool
	generation int
	lastScan   time.Time
	details    bool
	width      int
	height     int

	table table.Model
}

func NewModel(_ context.Context, scan ScanFunc, cfg *config.UIConfig) Model {
	return Model{
		scan:       scan,
		refresh:    cfg.RefreshInterval,
		inventory:  domain.Inventory{Devices: nil, Unresolved: nil},
		devices:    nil,
		warnings:   nil,
		err:        nil,
		sortKey:    domain.SortByIP,
		scanning:   true, // Init starts the first scan
		generation: 0,
		lastScan:   time.Time{},
		details:    false,
		width:      0,
		height:     0,
		table:      newTable(),
	}
}

func (m Model) Init() tea.Cmd {
	return m.scan(m.inventory)
}

//nolint:ireturn // tea.Model requires Update to return the tea.Model interface.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// tea.Model fixes this signature, so there is no caller context to pass on.
	ctx := context.Background()

	switch msg := msg.(type) {
	case ScanDoneMsg:
		return m.onScanDone(ctx, msg)
	case TickMsg:
		if msg.Generation != m.generation {
			return m, nil
		}
		return m.startScan()
	case tea.WindowSizeMsg:
		return m.onResize(ctx, msg), nil
	case tea.KeyMsg:
		return m.onKey(ctx, msg)
	}

	return m, nil
}

func (m Model) View() string {
	// tea.Model fixes this signature, so there is no caller context to pass on.
	return render(context.Background(), m)
}

func (m Model) startScan() (Model, tea.Cmd) {
	if m.scanning {
		return m, nil
	}

	m.scanning = true

	return m, m.scan(m.inventory)
}

func (m Model) onScanDone(ctx context.Context, msg ScanDoneMsg) (Model, tea.Cmd) {
	m.scanning = false
	m.generation++
	m.lastScan = time.Now()
	m.err = msg.Err
	next := tea.Tick(m.refresh, func(time.Time) tea.Msg { return TickMsg{Generation: m.generation} })

	// A failed scan keeps the last inventory on screen under the error banner.
	if msg.Err == nil {
		m.inventory = msg.Result.Inventory
		m.warnings = msg.Result.Warnings
	}

	return m.withDevices(ctx), next
}

func (m Model) onKey(ctx context.Context, msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case keyQuit, keyInterrupt:
		return m, tea.Quit
	case keyRescan:
		return m.startScan()
	case keySort:
		m.sortKey = domain.NextSortKey(ctx, m.sortKey)
		return m.withDevices(ctx), nil
	case keyDetails:
		m.details = !m.details && len(m.devices) > 0
		return m, nil
	case keyBack:
		m.details = false
		return m, nil
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)

	return m, cmd
}

func (m Model) onResize(ctx context.Context, msg tea.WindowSizeMsg) Model {
	m.width = msg.Width
	m.height = msg.Height
	m.table.SetColumns(columns(msg.Width))

	return m.withDevices(ctx)
}

// withDevices re-sorts the inventory and refreshes the table, keeping the cursor in range and the table
// sized to the space the banners leave.
func (m Model) withDevices(ctx context.Context) Model {
	m.devices = domain.SortedDevices(ctx, m.inventory, m.sortKey)
	m.table.SetRows(rows(ctx, m.devices))
	m.table.SetHeight(max(m.height-chromeHeight(m), minTableHeight))
	// An empty table parks its cursor at -1; bring it back once there are rows.
	if len(m.devices) > 0 {
		m.table.SetCursor(min(max(m.table.Cursor(), 0), len(m.devices)-1))
	}

	return m
}

func (m Model) selected() (domain.Device, bool) {
	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.devices) {
		return domain.Device{}, false
	}

	return m.devices[cursor], true
}
