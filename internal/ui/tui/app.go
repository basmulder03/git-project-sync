package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type StatusProvider interface {
	DashboardStatus(ctx context.Context) (DashboardStatus, error)
}

type App struct {
	provider        StatusProvider
	executor        ActionExecutor
	dashboard       *Dashboard
	in              io.Reader
	out             io.Writer
	refreshInterval time.Duration
	lastMessage     string
}

func NewApp(provider StatusProvider, in io.Reader, out io.Writer) *App {
	return &App{
		provider:        provider,
		dashboard:       NewDashboard(),
		in:              in,
		out:             out,
		refreshInterval: 2 * time.Second,
	}
}

func (a *App) SetExecutor(executor ActionExecutor) {
	a.executor = executor
}

func (a *App) Run(ctx context.Context) error {
	if a.provider == nil {
		return fmt.Errorf("status provider is required")
	}

	status, err := a.provider.DashboardStatus(ctx)
	if err != nil {
		return err
	}
	a.render(status)

	keys := make(chan string, 16)
	go a.readKeys(ctx, keys)

	ticker := time.NewTicker(a.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case key := <-keys:
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "q":
				return nil
			case "r":
				status, err = a.provider.DashboardStatus(ctx)
				if err != nil {
					return err
				}
				a.render(status)
			default:
				if request, ok := KeyToAction(strings.ToLower(strings.TrimSpace(key)), status); ok {
					a.runAction(ctx, request)
					a.render(status)
					continue
				}

				if changed, message := a.dashboard.HandleKey(key, status); changed {
					if message != "" {
						a.lastMessage = message
					}
					a.render(status)
				}
			}
		case <-ticker.C:
			status, err = a.provider.DashboardStatus(ctx)
			if err != nil {
				return err
			}
			a.render(status)
		}
	}
}

func (a *App) render(status DashboardStatus) {
	view := a.dashboard.Render(status)
	if a.lastMessage != "" {
		view += "\nAction: " + a.lastMessage + "\n"
	}
	fmt.Fprintf(a.out, "\x1b[2J\x1b[H%s", view)
}

func (a *App) runAction(ctx context.Context, request ActionRequest) {
	if a.executor == nil {
		a.lastMessage = "action handler not configured"
		return
	}

	result, err := a.executor.Execute(ctx, request)
	if err != nil {
		a.lastMessage = err.Error()
		return
	}

	a.lastMessage = result
}

func (a *App) readKeys(ctx context.Context, out chan<- string) {
	reader := bufio.NewReader(a.in)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		r, _, err := reader.ReadRune()
		if err != nil {
			return
		}

		key := string(r)
		if r == 27 {
			if seq, _ := reader.Peek(2); len(seq) == 2 && seq[0] == '[' {
				_, _ = reader.Discard(2)
				switch seq[1] {
				case 'D':
					key = "left"
				case 'C':
					key = "right"
				}
			}
		}

		out <- key
	}
}
