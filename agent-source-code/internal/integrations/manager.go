package integrations

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"patchmon-agent/internal/utils"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// Manager orchestrates integration discovery and data collection
type Manager struct {
	integrations     []Integration
	logger           *logrus.Logger
	mu               sync.RWMutex
	isEnabledChecker func(string) bool // Optional function to check if integration is enabled
}

// NewManager creates a new integration manager
func NewManager(logger *logrus.Logger) *Manager {
	return &Manager{
		integrations: make([]Integration, 0),
		logger:       logger,
	}
}

// SetEnabledChecker sets the function to check if an integration is enabled
func (m *Manager) SetEnabledChecker(checker func(string) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.isEnabledChecker = checker
}

// Register adds an integration to the manager
func (m *Manager) Register(integration Integration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.integrations = append(m.integrations, integration)
	m.logger.WithField("integration", integration.Name()).Debug("Registered integration")
}

// DiscoverIntegrations checks which integrations are available and returns them
// Only returns integrations that are both available and enabled (if enabled checker is set)
func (m *Manager) DiscoverIntegrations() []Integration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	available := make([]Integration, 0)
	for _, integration := range m.integrations {
		// Check if integration is available
		if !integration.IsAvailable() {
			m.logger.WithField("integration", integration.Name()).Debug("✗ Integration not available")
			continue
		}

		// Check if integration is enabled (if enabled checker is set)
		if m.isEnabledChecker != nil && !m.isEnabledChecker(integration.Name()) {
			m.logger.WithField("integration", integration.Name()).Debug("✗ Integration disabled in config")
			continue
		}

		available = append(available, integration)
		m.logger.WithField("integration", integration.Name()).Info("✓ Integration discovered")
	}

	// Sort by priority (lower number = higher priority)
	sort.Slice(available, func(i, j int) bool {
		return available[i].Priority() < available[j].Priority()
	})

	return available
}

// CollectAll collects data from all available integrations
// Returns a map of integration name -> integration data
// Errors in individual integrations do not stop collection from others
func (m *Manager) CollectAll(ctx context.Context) map[string]*models.IntegrationData {
	available := m.DiscoverIntegrations()

	if len(available) == 0 {
		m.logger.Debug("No integrations available for collection")
		return make(map[string]*models.IntegrationData)
	}

	results := make(map[string]*models.IntegrationData)
	var wg sync.WaitGroup
	var resultsMu sync.Mutex

	m.logger.WithField("count", len(available)).Info("Collecting data from integrations...")

	for _, integration := range available {
		wg.Add(1)
		go func(integ Integration) {
			defer wg.Done()

			name := integ.Name()
			m.logger.WithField("integration", name).Debug("Starting collection")
			startTime := time.Now()

			data, err := integ.Collect(ctx)
			if err != nil {
				m.logger.WithFields(logrus.Fields{
					"integration": name,
					"error":       err.Error(),
				}).Warn("Integration collection failed")

				// Still add the result but with error
				data = &models.IntegrationData{
					Name:          name,
					Enabled:       true,
					CollectedAt:   utils.GetCurrentTimeUTC(),
					ExecutionTime: time.Since(startTime).Seconds(),
					Error:         err.Error(),
				}
			} else {
				m.logger.WithFields(logrus.Fields{
					"integration":    name,
					"execution_time": data.ExecutionTime,
				}).Info("Integration collection completed")
			}

			resultsMu.Lock()
			results[name] = data
			resultsMu.Unlock()
		}(integration)
	}

	wg.Wait()
	return results
}

// GetIntegration returns a specific integration by name
func (m *Manager) GetIntegration(name string) (Integration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, integration := range m.integrations {
		if integration.Name() == name {
			return integration, nil
		}
	}

	return nil, fmt.Errorf("integration %s not found", name)
}

// GetRealtimeIntegrations returns all integrations that support real-time monitoring
// Only returns integrations that are enabled (if enabled checker is set)
func (m *Manager) GetRealtimeIntegrations() []RealtimeIntegration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realtime := make([]RealtimeIntegration, 0)
	for _, integration := range m.integrations {
		if !integration.SupportsRealtime() {
			continue
		}

		// Check if integration is enabled (if enabled checker is set)
		if m.isEnabledChecker != nil && !m.isEnabledChecker(integration.Name()) {
			m.logger.WithField("integration", integration.Name()).Debug("✗ Realtime integration disabled in config")
			continue
		}

		if rtInteg, ok := integration.(RealtimeIntegration); ok {
			realtime = append(realtime, rtInteg)
		}
	}

	return realtime
}
