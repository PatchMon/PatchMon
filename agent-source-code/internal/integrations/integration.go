package integrations

import (
	"context"
	"patchmon-agent/pkg/models"
)

// Integration defines the interface that all integrations must implement
type Integration interface {
	// Name returns the integration name (e.g., "docker", "proxmox")
	Name() string

	// IsAvailable checks if the integration is available on this system
	IsAvailable() bool

	// Collect gathers data from the integration
	Collect(ctx context.Context) (*models.IntegrationData, error)

	// Priority returns the collection priority (lower = higher priority)
	// Used for future ordering of collection execution
	Priority() int

	// SupportsRealtime indicates if this integration supports real-time monitoring
	SupportsRealtime() bool
}

// RealtimeIntegration extends Integration with real-time monitoring capabilities
type RealtimeIntegration interface {
	Integration

	// StartMonitoring begins real-time monitoring (e.g., container events)
	// Should be run in a goroutine
	StartMonitoring(ctx context.Context, eventChan chan<- interface{}) error

	// StopMonitoring stops real-time monitoring
	StopMonitoring() error
}
