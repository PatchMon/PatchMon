package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/docker/docker/api/types/events"
	"github.com/sirupsen/logrus"
)

// Constants for reconnection strategy
const (
	initialBackoffDuration = 1 * time.Second
	maxBackoffDuration     = 30 * time.Second
	maxReconnectAttempts   = -1 // -1 means unlimited with backoff strategy
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

		// Attempt to establish event stream
		err := d.monitorEvents(ctx, eventChan)

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

			// Implement exponential backoff with jitter
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
func (d *Integration) monitorEvents(ctx context.Context, eventChan chan<- interface{}) error {
	// Get a fresh event stream from Docker
	eventsCh, errCh := d.client.Events(ctx, events.ListOptions{})

	d.logger.Debug("Docker event stream established")

	// Process events until stream ends or context is cancelled
	for {
		select {
		case <-ctx.Done():
			d.logger.Debug("Docker event monitoring context cancelled")
			return ctx.Err()

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
				d.handleContainerEvent(event, eventChan)
			}
		}
	}
}

// handleContainerEvent processes container events and sends status updates
func (d *Integration) handleContainerEvent(event events.Message, eventChan chan<- interface{}) {
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
		return
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

	// Send event to channel (non-blocking)
	select {
	case eventChan <- statusEvent:
	default:
		d.logger.Warn("Event channel full, dropping event")
	}
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

// exponentialBackoff calculates backoff duration using exponential strategy
// This function is kept for potential future use or testing
func exponentialBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return initialBackoffDuration
	}

	// Calculate: initialBackoffDuration * (1.5 ^ (attempt - 1))
	duration := time.Duration(float64(initialBackoffDuration) * math.Pow(1.5, float64(attempt-1)))

	if duration > maxBackoffDuration {
		return maxBackoffDuration
	}

	return duration
}
