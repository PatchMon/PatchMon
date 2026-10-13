// Package docker provides Docker container integration functionality
package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const containerInspectTimeout = 10 * time.Second

type containerUsage struct {
	networks map[string]int
	volumes  map[string]int
}

// collectContainers collects all Docker containers (running and stopped)
func (d *Integration) collectContainers(ctx context.Context, idx *imageIndex) ([]models.DockerContainer, *containerUsage, error) {
	containerResult, err := d.client.ContainerList(ctx, client.ContainerListOptions{
		All: true,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list containers: %w", err)
	}

	usage := &containerUsage{networks: make(map[string]int), volumes: make(map[string]int)}
	result := make([]models.DockerContainer, 0, len(containerResult.Items))

	for _, c := range containerResult.Items {
		var inspect *container.InspectResponse
		ictx, cancel := context.WithTimeout(ctx, containerInspectTimeout)
		res, ierr := d.client.ContainerInspect(ictx, c.ID, client.ContainerInspectOptions{})
		cancel()
		if ierr != nil {
			d.logger.WithError(ierr).WithField("container_id", shortID(c.ID)).Debug("Container inspect failed, using list data")
		} else {
			inspect = &res.Container
			usage.add(inspect)
		}
		result = append(result, buildContainer(c, inspect, idx))
	}

	return result, usage, nil
}

func (u *containerUsage) add(in *container.InspectResponse) {
	if in.NetworkSettings != nil {
		for name, ep := range in.NetworkSettings.Networks {
			key := name
			if ep != nil && ep.NetworkID != "" {
				key = ep.NetworkID
			}
			u.networks[key]++
		}
	}
	for _, m := range in.Mounts {
		if m.Type == mount.TypeVolume && m.Name != "" {
			u.volumes[m.Name]++
		}
	}
}

func buildContainer(c container.Summary, in *container.InspectResponse, idx *imageIndex) models.DockerContainer {
	name := ""
	if len(c.Names) > 0 {
		name = strings.TrimPrefix(c.Names[0], "/")
	}

	var createdAt *time.Time
	if c.Created > 0 {
		t := time.Unix(c.Created, 0)
		createdAt = &t
	}

	imageID := trimSHA(c.ImageID)
	rawRef := c.Image
	if in != nil {
		if in.Config != nil && in.Config.Image != "" {
			rawRef = in.Config.Image
		}
		if in.Image != "" {
			imageID = trimSHA(in.Image)
		}
	}

	ref := resolveContainerRef(rawRef, imageID, idx)

	source, known := idx.sourceFor(imageID)
	if !known {
		source = determineImageSource(ref.Repository)
	}

	out := models.DockerContainer{
		ContainerID:     trimSHA(c.ID),
		Name:            name,
		ImageName:       ref.Repository,
		ImageTag:        ref.Tag,
		ImageRepository: ref.Repository,
		ImageSource:     source,
		ImageID:         imageID,
		ImageDigest:     ref.Digest,
		Status:          normalizeStatus(c.Status, string(c.State)),
		State:           string(c.State),
		Ports:           convertPorts(c.Ports),
		CreatedAt:       createdAt,
		Labels:          c.Labels,
		NetworkMode:     c.HostConfig.NetworkMode,
	}

	if ref.Digest == "" && ref.Tag != noneTag {
		if localID := idx.idForRepoTag(ref.Repository, ref.Tag); localID != "" && imageID != "" && localID != imageID {
			out.ImageStale = true
		}
	}

	if in != nil {
		out.RestartCount = in.RestartCount
		if in.State != nil {
			out.StartedAt = parseDockerTime(in.State.StartedAt)
		}
	}
	return out
}

func resolveContainerRef(raw, imageID string, idx *imageIndex) imageRef {
	ref := parseImageRef(raw)
	if ref.ID != "" {
		img, ok := idx.lookupID(ref.ID)
		if !ok && !ref.IDOnly {
			ref.ID = ""
			return ref
		}
		if !ok {
			img, ok = idx.lookupID(imageID)
		}
		resolved := imageRef{Repository: noneTag, Tag: noneTag}
		if ok && len(img.RepoTags) == 1 {
			if r := parseImageRef(img.RepoTags[0]); r.isNamed() {
				resolved.Repository, resolved.Tag = r.Repository, r.Tag
			}
		}
		return resolved
	}
	if !ref.isNamed() {
		return imageRef{Repository: noneTag, Tag: noneTag}
	}
	return ref
}

func parseDockerTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil || t.IsZero() || t.Year() <= 1 {
		return nil
	}
	return &t
}

func shortID(id string) string {
	id = trimSHA(id)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
