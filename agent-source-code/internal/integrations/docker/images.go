package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/docker/docker/api/types/image"
)

// collectImages collects all Docker images
func (d *Integration) collectImages(ctx context.Context) ([]models.DockerImage, error) {
	// List all images
	images, err := d.client.ImageList(ctx, image.ListOptions{
		All: false, // Only show non-intermediate images
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}

	result := make([]models.DockerImage, 0)

	for _, img := range images {
		// Skip images with no tags (dangling images)
		if len(img.RepoTags) == 0 {
			continue
		}

		// Process each tag
		for _, repoTag := range img.RepoTags {
			// Skip <none>:<none> images
			if repoTag == "<none>:<none>" {
				continue
			}

			// Parse image name
			repository, tag := parseImageName(repoTag)

			// Get digest first to determine if image is locally built
			digest := ""
			if len(img.RepoDigests) > 0 {
				// Extract just the hash part
				parts := strings.Split(img.RepoDigests[0], "@")
				if len(parts) > 1 {
					digest = strings.TrimPrefix(parts[1], "sha256:")
				}
			}

			// Determine source - if no digest, image is locally built
			source := determineImageSource(repository)
			if len(img.RepoDigests) == 0 || digest == "" {
				// No RepoDigests means the image was built locally and never pushed to a registry
				source = "local"
			}

			// Convert created timestamp
			var createdAt *time.Time
			if img.Created > 0 {
				t := time.Unix(img.Created, 0)
				createdAt = &t
			}

			imageData := models.DockerImage{
				Repository: repository,
				Tag:        tag,
				ImageID:    strings.TrimPrefix(img.ID, "sha256:"),
				Source:     source,
				SizeBytes:  img.Size,
				CreatedAt:  createdAt,
				Digest:     digest,
				Labels:     img.Labels,
			}

			result = append(result, imageData)
		}
	}

	return result, nil
}
