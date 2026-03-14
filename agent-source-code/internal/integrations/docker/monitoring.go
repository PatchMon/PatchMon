package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

// Constants for reconnection strategy
const (
	initialBackoffDuration = 1 * time.Second
	maxBackoffDuration     = 30 * time.Second
	maxReconnectAttempts   = -1                     // -1 means unlimited with backoff strategy
	dockerPingTimeout      = 3 * time.Second        // Timeout for Docker ping check
	dockerPingInterval     = 1 * time.Second        // How often to check if Docker is ready
	dockerPingRetries      = 2                      // Number of consecutive successful pings required
	dockerPingRetryDelay   = 200 * time.Millisecond // Delay between ping retries
)

// StartMonitoring begins monitoring Docker events for real-time status changes
func (d *Integration) StartMonitoring(ctx context.Context, eventChan chan<- interface{}) error {
	d.monitoringMu.Lock()
	if d.monitoring {
		d.monitoringMu.Unlock()
		return fmt.Errorf("monitoring already started")
	}
	d.monitoring = true
	d.monitoringMu.Unlock()

	if d.client == nil {
		if !d.IsAvailable() {
			return fmt.Errorf("docker is not available")
		}
	}

	// Create a cancellable context
	monitorCtx, cancel := context.WithCancel(ctx)
	d.stopMonitoring = cancel

	d.logger.Info("Starting Docker event monitoring...")

	// Start the monitoring loop in a goroutine with reconnection logic
	go d.monitoringLoop(monitorCtx, eventChan)

	return nil
}

// StopMonitoring stops Docker event monitoring
func (d *Integration) StopMonitoring() error {
	d.monitoringMu.Lock()
	defer d.monitoringMu.Unlock()

	if !d.monitoring {
		return nil
	}

	if d.stopMonitoring != nil {
		d.stopMonitoring()
		d.stopMonitoring = nil
	}

	d.monitoring = false
	d.logger.Info("Stopped Docker event monitoring")

	return nil
}

// monitoringLoop manages the event stream with automatic reconnection on failure
func (d *Integration) monitoringLoop(ctx context.Context, eventChan chan<- interface{}) {
	defer func() {
		d.monitoringMu.Lock()
		d.monitoring = false
		d.monitoringMu.Unlock()
		d.logger.Info("Docker event monitoring loop stopped")
	}()

	backoffDuration := initialBackoffDuration
	reconnectAttempts := 0

	for {
		// Check if context is done
		select {
		case <-ctx.Done():
			d.logger.Debug("Docker event monitoring context cancelled")
			return
		default:
		}

		// Wait for Docker to be ready before attempting connection
		// This prevents EOF errors and long connection attempts
		if reconnectAttempts > 0 {
			d.logger.WithField("attempt", reconnectAttempts+1).
				Info("Waiting for Docker to be ready before reconnecting...")
			if !d.waitForDockerReady(ctx) {
				// Docker not ready, will retry after backoff
				err := fmt.Errorf("docker daemon not available")
				reconnectAttempts++
				d.logger.WithError(err).WithField("attempt", reconnectAttempts).
					Warn("Docker daemon not ready, will retry...")

				// Implement exponential backoff
				d.logger.WithField("backoff_seconds", backoffDuration.Seconds()).
					Info("Waiting before reconnection attempt")

				// Sleep with context cancellation support
				select {
				case <-ctx.Done():
					d.logger.Debug("Context cancelled while waiting for reconnect")
					return
				case <-time.After(backoffDuration):
					// Continue to next reconnect attempt
				}

				// Increase backoff duration with exponential growth (capped at maxBackoffDuration)
				backoffDuration = time.Duration(float64(backoffDuration) * 1.5)
				if backoffDuration > maxBackoffDuration {
					backoffDuration = maxBackoffDuration
				}
				continue
			}
			d.logger.Info("Docker daemon is ready, attempting to reconnect...")
		}

		// Attempt to establish event stream
		// Use current time to only get events from now onwards (prevents backlog replay)
		// Update startTime on each reconnect to avoid getting old events
		reconnectTime := time.Now()
		err := d.monitorEvents(ctx, eventChan, reconnectTime)

		// Check if context is done (to avoid unnecessary error logging)
		select {
		case <-ctx.Done():
			d.logger.Debug("Docker event monitoring context cancelled during reconnect")
			return
		default:
		}

		// Handle reconnection
		if err != nil {
			reconnectAttempts++
			d.logger.WithError(err).WithField("attempt", reconnectAttempts).
				Warn("Docker event stream ended, attempting to reconnect...")

			// Implement exponential backoff
			d.logger.WithField("backoff_seconds", backoffDuration.Seconds()).
				Info("Waiting before reconnection attempt")

			// Sleep with context cancellation support
			select {
			case <-ctx.Done():
				d.logger.Debug("Context cancelled while waiting for reconnect")
				return
			case <-time.After(backoffDuration):
				// Continue to next reconnect attempt
			}

			// Increase backoff duration with exponential growth (capped at maxBackoffDuration)
			backoffDuration = time.Duration(float64(backoffDuration) * 1.5)
			if backoffDuration > maxBackoffDuration {
				backoffDuration = maxBackoffDuration
			}
		} else {
			// If connection was successful, reset backoff
			backoffDuration = initialBackoffDuration
			reconnectAttempts = 0
		}
	}
}

// monitorEvents establishes and monitors the Docker event stream
// Returns when the stream ends (EOF, connection loss, etc.)
// startTime is used to filter out old events (only get events from startTime onwards)
func (d *Integration) monitorEvents(ctx context.Context, eventChan chan<- interface{}, startTime time.Time) error {
	// Get a fresh event stream from Docker
	// Use Since parameter to only get events from startTime onwards
	// This prevents replaying a backlog of historical events when reconnecting
	eventsCh, errCh := d.client.Events(ctx, events.ListOptions{
		Since: startTime.Format(time.RFC3339Nano),
	})

	d.logger.Debug("Docker event stream established")

	// OPTIMIZATION: Create a ticker to periodically check context and prevent goroutine buildup
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Process events until stream ends or context is cancelled
	for {
		select {
		case <-ctx.Done():
			d.logger.Debug("Docker event monitoring context cancelled")
			return ctx.Err()

		case <-ticker.C:
			// Periodic health check to prevent stuck goroutines
			continue

		case err := <-errCh:
			if err == nil {
				// Channel closed without error - Docker connection lost
				d.logger.Warn("Docker event stream closed")
				return io.EOF
			}

			// Handle specific error types
			if errors.Is(err, io.EOF) {
				d.logger.Info("Docker event stream EOF - daemon likely restarted")
				return err
			}

			if errors.Is(err, context.Canceled) {
				d.logger.Debug("Docker event stream context cancelled")
				return err
			}

			d.logger.WithError(err).Warn("Docker event stream error")
			return err

		case event := <-eventsCh:
			// Check if channel was closed
			if event.Type == "" && event.Time == 0 {
				// This might be a zero value from a closed channel
				// But we'll let the errCh handle it
				continue
			}

			if event.Type == events.ContainerEventType {
				// OPTIMIZATION: Non-blocking send to prevent goroutine blockage
				select {
				case eventChan <- d.createContainerEvent(event):
				default:
					// Channel full, skip this event to prevent blocking
					d.logger.Debug("Event channel full, skipping event")
				}
			}
		}
	}
}

// createContainerEvent creates a container event from Docker event
func (d *Integration) createContainerEvent(event events.Message) interface{} {
	return d.handleContainerEvent(event)
}

// handleContainerEvent processes container events and creates status updates
func (d *Integration) handleContainerEvent(event events.Message) interface{} {
	// We're interested in these actions:
	// - start: container started
	// - stop: container stopped
	// - die: container died (crashed)
	// - pause: container paused
	// - unpause: container unpaused
	// - kill: container killed
	// - destroy: container destroyed

	relevantActions := map[string]string{
		"start":   "container_start",
		"stop":    "container_stop",
		"die":     "container_die",
		"pause":   "container_pause",
		"unpause": "container_unpause",
		"kill":    "container_kill",
		"destroy": "container_destroy",
	}

	eventType, relevant := relevantActions[string(event.Action)]
	if !relevant {
		return nil
	}

	// Extract container information
	containerID := event.Actor.ID
	containerName := ""
	image := ""

	// Get name from attributes
	if name, ok := event.Actor.Attributes["name"]; ok {
		containerName = name
	}

	// Get image from attributes
	if img, ok := event.Actor.Attributes["image"]; ok {
		image = img
	}

	// Determine status based on action
	status := mapActionToStatus(string(event.Action))

	statusEvent := models.DockerStatusEvent{
		Type:        eventType,
		ContainerID: containerID,
		Name:        containerName,
		Image:       image,
		Status:      status,
		Timestamp:   time.Unix(event.Time, 0),
	}

	d.logger.WithFields(logrus.Fields{
		"type":         eventType,
		"container_id": containerID[:12], // Short ID
		"name":         containerName,
		"image":        image,
		"status":       status,
	}).Info("Docker container event")

	return statusEvent
}

// mapActionToStatus maps Docker event actions to status strings
func mapActionToStatus(action string) string {
	switch action {
	case "start":
		return "running"
	case "stop", "die", "kill":
		return "exited"
	case "pause":
		return "paused"
	case "unpause":
		return "running"
	case "destroy":
		return "removed"
	default:
		return "unknown"
	}
}

// waitForDockerReady waits for Docker daemon to be available and ready
// Returns true when Docker is ready, false if context is cancelled
// Requires multiple consecutive successful pings to ensure Docker is stable
func (d *Integration) waitForDockerReady(ctx context.Context) bool {
	// Check if socket exists first (fast check)
	if _, err := os.Stat(dockerSocketPath); os.IsNotExist(err) {
		d.logger.Debug("Docker socket not found, waiting...")
		// Wait for socket to appear
		ticker := time.NewTicker(dockerPingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return false
			case <-ticker.C:
				if _, err := os.Stat(dockerSocketPath); err == nil {
					// Socket exists, break out of for loop to try ping
					goto pingCheck
				}
			}
		}
	pingCheck:
	}

	// Socket exists, now check if daemon is responding
	// We require multiple consecutive successful pings to ensure Docker is stable
	ticker := time.NewTicker(dockerPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			// Try multiple consecutive pings to ensure Docker is stable
			if d.verifyDockerStable(ctx) {
				d.logger.Info("Docker daemon verified as stable and ready")
				return true
			}
			d.logger.Debug("Docker daemon not ready yet, will retry...")
		}
	}
}

// verifyDockerStable performs multiple consecutive ping checks to ensure Docker is stable
// Returns true only if all pings succeed consecutively
func (d *Integration) verifyDockerStable(ctx context.Context) bool {
	// Get or create client
	var cli *client.Client
	var err error
	shouldClose := false
	if d.client != nil {
		cli = d.client
	} else {
		cli, err = client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return false
		}
		shouldClose = true
	}

	// Require multiple consecutive successful pings
	for i := 0; i < dockerPingRetries; i++ {
		pingCtx, cancel := context.WithTimeout(ctx, dockerPingTimeout)
		_, err := cli.Ping(pingCtx)
		cancel()

		if err != nil {
			// Ping failed, Docker is not ready
			d.logger.WithError(err).Debugf("Docker ping %d/%d failed", i+1, dockerPingRetries)
			if shouldClose {
				_ = cli.Close()
			}
			return false
		}
		d.logger.Debugf("Docker ping %d/%d succeeded", i+1, dockerPingRetries)

		// If not the last ping, wait a bit before next ping
		if i < dockerPingRetries-1 {
			select {
			case <-ctx.Done():
				if shouldClose {
					_ = cli.Close()
				}
				return false
			case <-time.After(dockerPingRetryDelay):
				// Continue to next ping
			}
		}
	}

	// All pings succeeded, Docker is stable and ready
	if shouldClose {
		// Store the client if we created a new one
		d.client = cli
	}
	return true
}

// exponentialBackoff calculates backoff duration using exponential strategy
// This function is kept for potential future use or testing
func exponentialBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return initialBackoffDuration
	}

	// Calculate: initialBackoffDuration * (1.5 ^ (attempt - 1))
	// Using simple multiplication instead of math.Pow for efficiency
	duration := initialBackoffDuration
	for i := 1; i < attempt; i++ {
		duration = time.Duration(float64(duration) * 1.5)
		if duration > maxBackoffDuration {
			return maxBackoffDuration
		}
	}

	return duration
}
