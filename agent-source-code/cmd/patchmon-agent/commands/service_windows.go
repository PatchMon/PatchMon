//go:build windows
// +build windows

package commands

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

// signalNotify wraps signal.Notify for Windows
var signalNotify = signal.Notify

const serviceName = "PatchMonAgent"

// patchmonService implements svc.Handler interface for Windows Service
type patchmonService struct {
	stopCh chan struct{}
}

// Execute is called by Windows Service Control Manager
func (s *patchmonService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Notify SCM that we're starting
	changes <- svc.Status{State: svc.StartPending}

	// Start the actual service logic in a goroutine
	s.stopCh = make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- runServiceLoop(s.stopCh)
	}()

	// Notify SCM that we're running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// Wait for stop signal or error
	for {
		select {
		case err := <-errCh:
			if err != nil {
				elog, _ := eventlog.Open(serviceName)
				if elog != nil {
					elog.Error(1, fmt.Sprintf("Service error: %v", err))
					elog.Close()
				}
				return false, 1
			}
			return false, 0
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				close(s.stopCh)
				// Wait briefly for clean shutdown
				select {
				case <-errCh:
				case <-time.After(10 * time.Second):
				}
				return false, 0
			default:
				elog, _ := eventlog.Open(serviceName)
				if elog != nil {
					elog.Error(1, fmt.Sprintf("Unexpected control request #%d", c))
					elog.Close()
				}
			}
		}
	}
}

// isWindowsService checks if we're running as a Windows Service
func isWindowsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		// If we can't determine, assume interactive (not a service)
		return false
	}
	return isService
}

// runWindowsService starts the Windows Service
func runWindowsService() error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		// If we can't open event log, try to continue anyway
		elog = nil
	}
	defer func() {
		if elog != nil {
			elog.Close()
		}
	}()

	if elog != nil {
		elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))
	}

	err = svc.Run(serviceName, &patchmonService{})
	if err != nil {
		if elog != nil {
			elog.Error(1, fmt.Sprintf("Service failed: %v", err))
		}
		return fmt.Errorf("service failed: %w", err)
	}

	if elog != nil {
		elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
	}
	return nil
}

// runAsService determines if running as service or interactive and runs appropriately
func runAsService() error {
	isService := isWindowsService()

	if isService {
		return runWindowsService()
	}

	// Running interactively - run directly with signal handling
	logger.Info("Running in interactive mode (not as Windows Service)")
	logger.Info("Press Ctrl+C to stop")

	stopCh := make(chan struct{})

	// Handle Ctrl+C gracefully
	go func() {
		sigCh := make(chan os.Signal, 1)
		signalNotify(sigCh, os.Interrupt)
		<-sigCh
		logger.Info("Interrupt received, shutting down...")
		close(stopCh)
	}()

	return runServiceLoop(stopCh)
}
