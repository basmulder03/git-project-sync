package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type fakeProvider struct{}

func (fakeProvider) DashboardStatus(context.Context) (DashboardStatus, error) {
	return DashboardStatus{Health: "healthy", UpdatedAt: time.Now().UTC()}, nil
}

type fakeExecutor struct {
	requests []ActionRequest
}

func (e *fakeExecutor) Execute(_ context.Context, request ActionRequest) (string, error) {
	e.requests = append(e.requests, request)
	return "ok", nil
}

func TestModelQuitOnQKey(t *testing.T) {
	t.Parallel()

	m := NewModel(fakeProvider{}, nil)
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "q", Code: 'q'}))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg, got %T", cmd())
	}
}

func TestModelPaletteRunsCommand(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{}
	m := NewModel(fakeProvider{}, executor)

	sequence := []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}),
		tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}),
		tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}),
		tea.KeyPressMsg(tea.Key{Text: "n", Code: 'n'}),
		tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}),
		tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}),
		tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}),
		tea.KeyPressMsg(tea.Key{Text: "l", Code: 'l'}),
		tea.KeyPressMsg(tea.Key{Text: "l", Code: 'l'}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}),
	}

	for _, key := range sequence {
		_, cmd := m.Update(key)
		if cmd != nil {
			msg := cmd()
			if msg != nil {
				_, _ = m.Update(msg)
			}
		}
	}

	if len(executor.requests) == 0 {
		t.Fatal("expected command palette request to be executed")
	}
	if executor.requests[0].Type != ActionRunCommand {
		t.Fatalf("expected action type %q, got %q", ActionRunCommand, executor.requests[0].Type)
	}
	if executor.requests[0].Command != "sync all" {
		t.Fatalf("unexpected command %q", executor.requests[0].Command)
	}
}

func TestModelRerunLastCommand(t *testing.T) {
	t.Parallel()

	executor := &fakeExecutor{}
	m := NewModel(fakeProvider{}, executor)

	// First run a command from palette.
	sequence := []tea.KeyPressMsg{
		tea.KeyPressMsg(tea.Key{Text: "/", Code: '/'}),
		tea.KeyPressMsg(tea.Key{Text: "s", Code: 's'}),
		tea.KeyPressMsg(tea.Key{Text: "y", Code: 'y'}),
		tea.KeyPressMsg(tea.Key{Text: "n", Code: 'n'}),
		tea.KeyPressMsg(tea.Key{Text: "c", Code: 'c'}),
		tea.KeyPressMsg(tea.Key{Text: " ", Code: ' '}),
		tea.KeyPressMsg(tea.Key{Text: "a", Code: 'a'}),
		tea.KeyPressMsg(tea.Key{Text: "l", Code: 'l'}),
		tea.KeyPressMsg(tea.Key{Text: "l", Code: 'l'}),
		tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}),
	}
	for _, key := range sequence {
		_, cmd := m.Update(key)
		if cmd != nil {
			msg := cmd()
			if msg != nil {
				_, _ = m.Update(msg)
			}
		}
	}

	// Re-run last command using '!'.
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: "!", Code: '!'}))
	if cmd == nil {
		t.Fatal("expected rerun command")
	}
	if msg := cmd(); msg != nil {
		_, _ = m.Update(msg)
	}

	if len(executor.requests) != 2 {
		t.Fatalf("expected 2 executed commands, got %d", len(executor.requests))
	}
	if executor.requests[1].Type != ActionRunCommand {
		t.Fatalf("expected rerun to execute command action, got %q", executor.requests[1].Type)
	}
	if executor.requests[1].Command != "sync all" {
		t.Fatalf("expected rerun command sync all, got %q", executor.requests[1].Command)
	}
}

func TestModelRerunWithoutHistoryShowsMessage(t *testing.T) {
	t.Parallel()

	m := NewModel(fakeProvider{}, &fakeExecutor{})
	_, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "!", Code: '!'}))
	if !strings.Contains(m.lastMessage, "no command history") {
		t.Fatalf("expected no-history message, got %q", m.lastMessage)
	}
}
