//go:build windows

package main

import (
	"context"
	"fmt"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "GitProjectSync"

// windowsService implements the svc.Handler interface so syncd can be managed
// by the Windows Service Control Manager (SCM). When the SCM sends a Stop or
// Shutdown control request the service sets the ctx cancel function that was
// passed in from main, which causes the regular sync loop to exit cleanly.
type windowsService struct {
	run func(ctx context.Context) int
}

func (s *windowsService) Execute(args []string, req <-chan svc.ChangeRequest, status chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	status <- svc.Status{State: svc.StartPending}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run the daemon loop in a separate goroutine so we can still handle SCM
	// control requests on the main goroutine.
	done := make(chan int, 1)
	go func() {
		done <- s.run(ctx)
	}()

	status <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	for {
		select {
		case c := <-req:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				status <- svc.Status{State: svc.StopPending}
				cancel()
				exitCode = uint32(<-done)
				return false, exitCode
			case svc.Interrogate:
				status <- c.CurrentStatus
			default:
				// Ignore unhandled control codes.
			}
		case code := <-done:
			// The daemon loop exited on its own (e.g. fatal error).
			return false, uint32(code)
		}
	}
}

// maybeRunAsService checks whether syncd was started by the Windows SCM. If it
// was, it hands control to the service dispatcher and returns (true, exitCode).
// If not (interactive / --once mode), it returns (false, 0) so main() can
// continue with its normal flow.
func maybeRunAsService(runFn func(ctx context.Context) int) (bool, int) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		fmt.Printf("syncd: failed to detect service context: %v\n", err)
		return true, 1
	}
	if !isService {
		return false, 0
	}

	err = svc.Run(windowsServiceName, &windowsService{run: runFn})
	if err != nil {
		fmt.Printf("syncd: service failed: %v\n", err)
		return true, 1
	}
	return true, 0
}
