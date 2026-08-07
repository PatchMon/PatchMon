package docker

import (
	"context"
	"fmt"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/moby/moby/client"
)

// collectVolumes collects all Docker volumes
func (d *Integration) collectVolumes(ctx context.Context) ([]models.DockerVolume, error) {
	// List all volumes
	volumeResult, err := d.client.VolumeList(ctx, client.VolumeListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list volumes: %w", err)
	}

	result := make([]models.DockerVolume, 0, len(volumeResult.Items))

	// Get system disk usage info to include volume sizes
	diskUsage, err := d.client.DiskUsage(ctx, client.DiskUsageOptions{Volumes: true})
	if err != nil {
		d.logger.WithError(err).Debug("Failed to get disk usage (volume sizes unavailable)")
	}

	// Create a map of volume name to usage for quick lookup
	volumeUsage := make(map[string]int64)
	if err == nil {
		for _, vol := range diskUsage.Volumes.Items {
			if vol.UsageData != nil {
				volumeUsage[vol.Name] = vol.UsageData.Size
			}
		}
	}

	for _, vol := range volumeResult.Items {
		// Parse created timestamp
		var createdAt *time.Time
		if vol.CreatedAt != "" {
			// Docker returns RFC3339Nano format
			if t, err := time.Parse(time.RFC3339Nano, vol.CreatedAt); err == nil {
				createdAt = &t
			}
		}

		// Get volume size if available
		var sizeBytes *int64
		if size, found := volumeUsage[vol.Name]; found {
			sizeBytes = &size
		}

		// Get driver renderer if available (e.g., "overlay2" for overlay2 driver)
		renderer := ""
		if vol.Options != nil {
			if t, ok := vol.Options["type"]; ok {
				renderer = t
			}
		}

		volumeData := models.DockerVolume{
			VolumeID:   vol.Name, // Docker volumes are identified by name
			Name:       vol.Name,
			Driver:     vol.Driver,
			Mountpoint: vol.Mountpoint,
			Renderer:   renderer,
			Scope:      vol.Scope,
			Labels:     vol.Labels,
			Options:    vol.Options,
			CreatedAt:  createdAt,
			SizeBytes:  sizeBytes,
			RefCount:   0, // Will be set below if available
		}

		// Get reference count from usage data if available
		if vol.UsageData != nil && vol.UsageData.RefCount >= 0 {
			volumeData.RefCount = int(vol.UsageData.RefCount)
		}

		result = append(result, volumeData)
	}

	return result, nil
}
