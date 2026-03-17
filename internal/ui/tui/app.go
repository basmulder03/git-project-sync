package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// ---------------------------------------------------------------------------
// Public interfaces (unchanged – cmd/synctui wires these in)
// ---------------------------------------------------------------------------

// StatusProvider fetches the live dashboard snapshot.
type StatusProvider interface {
	DashboardStatus(ctx context.Context) (DashboardStatus, error)
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type tickMsg time.Time
type pulseMsg time.Time
type statusMsg DashboardStatus
type statusErrMsg struct{ err error }
type actionResultMsg struct{ result string }
type actionErrMsg struct{ err error }

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// Model is the root Bubbletea v2 model for synctui.
type Model struct {
	provider StatusProvider
	executor ActionExecutor

	status    DashboardStatus
	dashboard *Dashboard
	palette   *CommandPalette

	width  int
	height int

	lastMessage string
	lastCommand string
	err         error

	refreshInterval time.Duration
	pulseInterval   time.Duration
	pulseFrame      int
	lastRefreshAt   time.Time
	ready           bool
}

// NewModel constructs a ready-to-run Bubbletea v2 model.
// provider must not be nil; executor may be nil (actions will report an error).
func NewModel(provider StatusProvider, executor ActionExecutor) *Model {
	return &Model{
		provider:        provider,
		executor:        executor,
		dashboard:       NewDashboard(),
		palette:         NewCommandPalette(),
		refreshInterval: 2 * time.Second,
		pulseInterval:   260 * time.Millisecond,
	}
}

// ---------------------------------------------------------------------------
// tea.Model implementation
// ---------------------------------------------------------------------------

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchStatus(),
		m.tick(),
		m.pulse(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.lastRefreshAt = time.Now().UTC()
		return m, tea.Batch(m.fetchStatus(), m.tick())

	case pulseMsg:
		m.pulseFrame = (m.pulseFrame + 1) % 8
		return m, m.pulse()

	case statusMsg:
		m.status = DashboardStatus(msg)
		m.err = nil
		m.ready = true
		return m, nil

	case statusErrMsg:
		m.err = msg.err
		m.ready = true
		return m, nil

	case actionResultMsg:
		m.lastMessage = msg.result
		return m, nil

	case actionErrMsg:
		m.lastMessage = "error: " + msg.err.Error()
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg.String())
	}

	return m, nil
}

func (m *Model) handleKey(key string) (tea.Model, tea.Cmd) {
	// Palette takes priority.
	if m.palette.Active() {
		command, consumed, execute := m.palette.HandleKey(key)
		if consumed {
			if execute {
				return m, m.runAction(ActionRequest{Type: ActionRunCommand, Command: command})
			}
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "/":
		m.palette.Open()
		return m, nil

	case "!":
		if strings.TrimSpace(m.lastCommand) == "" {
			m.lastMessage = "no command history yet"
			return m, nil
		}
		m.lastMessage = "re-running: " + m.lastCommand
		return m, m.runAction(ActionRequest{Type: ActionRunCommand, Command: m.lastCommand})

	case "r":
		return m, m.fetchStatus()

	case "s":
		return m, m.runAction(ActionRequest{Type: ActionSyncAll})

	case "c":
		return m, m.runAction(ActionRequest{Type: ActionCacheRefresh})

	case "t":
		traceID := ""
		if len(m.status.Events) > 0 {
			traceID = m.status.Events[0].TraceID
		}
		return m, m.runAction(ActionRequest{Type: ActionTraceDrilldown, TraceID: traceID})

	default:
		if changed, msg := m.dashboard.HandleKey(key, m.status); changed {
			if msg != "" {
				m.lastMessage = msg
			}
		}
		return m, nil
	}
}

func (m *Model) View() tea.View {
	var s string
	if !m.ready {
		s = "Loading…\n"
	} else if m.err != nil {
		s = fmt.Sprintf("Error: %v\nPress r to retry, q to quit.\n", m.err)
	} else {
		s = m.dashboard.Render(m.status, m.width, m.pulseFrame, m.lastRefreshAt)

		if m.palette.Active() {
			s += m.palette.Render(m.width, m.height)
		}

		if m.lastMessage != "" {
			s += "\n> " + m.lastMessage + "\n"
		}
	}
	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

// ---------------------------------------------------------------------------
// Commands
// ---------------------------------------------------------------------------

func (m *Model) fetchStatus() tea.Cmd {
	return func() tea.Msg {
		if m.provider == nil {
			return statusErrMsg{fmt.Errorf("status provider is required")}
		}
		status, err := m.provider.DashboardStatus(context.Background())
		if err != nil {
			return statusErrMsg{err}
		}
		return statusMsg(status)
	}
}

func (m *Model) tick() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) pulse() tea.Cmd {
	return tea.Tick(m.pulseInterval, func(t time.Time) tea.Msg {
		return pulseMsg(t)
	})
}

func (m *Model) runAction(req ActionRequest) tea.Cmd {
	if req.Type == ActionRunCommand {
		m.lastCommand = strings.TrimSpace(req.Command)
	}
	if m.executor == nil {
		return func() tea.Msg {
			return actionResultMsg{"action handler not configured"}
		}
	}
	return func() tea.Msg {
		result, err := m.executor.Execute(context.Background(), req)
		if err != nil {
			return actionErrMsg{err}
		}
		return actionResultMsg{result}
	}
}
