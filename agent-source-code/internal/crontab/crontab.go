package crontab

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"patchmon-agent/internal/config"

	"github.com/sirupsen/logrus"
)

// Manager handles crontab operations
type Manager struct {
	logger *logrus.Logger
}

// New creates a new crontab manager
func New(logger *logrus.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// UpdateSchedule updates the cron schedule with the given interval and executable path
func (m *Manager) UpdateSchedule(updateInterval int, executablePath string) error {
	if updateInterval <= 0 {
		return fmt.Errorf("invalid update interval: %d", updateInterval)
	}

	// Generate crontab entries for both update and update-crontab
	expectedEntries := m.generateCronEntries(updateInterval, executablePath)

	// Check if current entries are up to date
	if currentEntries := m.GetEntries(); m.entriesMatch(currentEntries, expectedEntries) {
		m.logger.WithField("interval", updateInterval).Info("Crontab is already up to date")
		return nil
	}

	m.logger.WithField("interval", updateInterval).Info("Setting update interval")

	// Write crontab file
	content := strings.Join(expectedEntries, "\n") + "\n"
	if err := os.WriteFile(config.CronFilePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to update crontab file: %w", err)
	}

	m.logger.WithFields(logrus.Fields{
		"interval": updateInterval,
		"path":     config.CronFilePath,
	}).Info("Crontab updated successfully")
	return nil
}

// GetEntries returns all current cron entries
func (m *Manager) GetEntries() []string {
	if data, err := os.ReadFile(config.CronFilePath); err == nil {
		// Filter out empty lines and comments
		var validLines []string
		for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				validLines = append(validLines, line)
			}
		}

		return validLines
	}
	return []string{}
}

// GetSchedule returns the schedule part (first 5 fields) of the current cron entry.
// Returns empty string if no valid entry is found.
func (m *Manager) GetSchedule() string {
	entries := m.GetEntries()
	if len(entries) == 0 {
		return ""
	}
	// Use the first entry (report entry) to extract schedule
	entry := entries[0]
	if entry == "" {
		return ""
	}
	fields := strings.Fields(entry)
	if len(fields) < 5 {
		return ""
	}
	return strings.Join(fields[:5], " ")
}

// Remove removes the PatchMon agent's cron file
func (m *Manager) Remove() error {
	if err := os.Remove(config.CronFilePath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File doesn't exist, which is fine
			return nil
		}
		return fmt.Errorf("failed to remove cron file: %w", err)
	}

	m.logger.WithField("path", config.CronFilePath).Info("Removed cron file")
	return nil
}

// generateCronEntries generates cron entries for both report and update-crontab commands
func (m *Manager) generateCronEntries(updateInterval int, executablePath string) []string {
	var schedule string

	if updateInterval == 60 {
		// Hourly updates - use current minute to spread load
		currentMinute := time.Now().Minute()
		schedule = fmt.Sprintf("%d * * * *", currentMinute)
	} else {
		// Custom interval updates
		schedule = fmt.Sprintf("*/%d * * * *", updateInterval)
	}

	return []string{
		fmt.Sprintf("%s root %s report >/dev/null 2>&1", schedule, executablePath),
		fmt.Sprintf("%s root %s update-crontab >/dev/null 2>&1", schedule, executablePath),
	}
}

// entriesMatch compares two slices of cron entries for equality
func (m *Manager) entriesMatch(current, expected []string) bool {
	if len(current) != len(expected) {
		return false
	}

	for i := range current {
		if current[i] != expected[i] {
			return false
		}
	}

	return true
}
