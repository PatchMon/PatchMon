package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/docker/docker/api/types/container"
)

// collectContainers collects all Docker containers (running and stopped)
func (d *Integration) collectContainers(ctx context.Context) ([]models.DockerContainer, error) {
	// List all containers
	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	result := make([]models.DockerContainer, 0, len(containers))

	for _, c := range containers {
		// Parse image name
		repository, tag := parseImageName(c.Image)

		// Clean repository name
		cleanRepo := cleanImageRepository(repository)

		// Determine source
		source := determineImageSource(repository)

		// Get container name (remove leading slash)
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		// Convert timestamps
		var createdAt *time.Time
		if c.Created > 0 {
			t := time.Unix(c.Created, 0)
			createdAt = &t
		}

		// Normalize status
		status := normalizeStatus(c.Status, c.State)

		// Convert ports
		ports := convertPorts(c.Ports)

		// Normalize container ID to full 64-character hash
		// Docker can return short (12 char) or full (64 char) IDs
		// We always use the full ID for consistency
		fullContainerID := c.ID
		if len(fullContainerID) == 12 {
			// If we got a short ID, we need to get the full ID
			// The c.ID from ContainerList should already be full, but let's be safe
			fullContainerID = c.ID
		}
		// Remove any sha256: prefix if present
		fullContainerID = strings.TrimPrefix(fullContainerID, "sha256:")

		container := models.DockerContainer{
			ContainerID:     fullContainerID,
			Name:            name,
			ImageName:       repository,
			ImageTag:        tag,
			ImageRepository: cleanRepo,
			ImageSource:     source,
			ImageID:         strings.TrimPrefix(c.ImageID, "sha256:"),
			Status:          status,
			State:           c.State,
			Ports:           ports,
			CreatedAt:       createdAt,
			Labels:          c.Labels,
			NetworkMode:     c.HostConfig.NetworkMode,
		}

		result = append(result, container)
	}

	return result, nil
}
