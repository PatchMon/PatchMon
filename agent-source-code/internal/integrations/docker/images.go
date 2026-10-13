package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"patchmon-agent/pkg/models"

	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/client"
)

type imageIndex struct {
	byID      map[string]image.Summary
	byRepoTag map[string]string
}

func newImageIndex(images []image.Summary) *imageIndex {
	idx := &imageIndex{
		byID:      make(map[string]image.Summary, len(images)),
		byRepoTag: make(map[string]string),
	}
	for _, img := range images {
		id := trimSHA(img.ID)
		idx.byID[id] = img
		for _, rt := range img.RepoTags {
			ref := parseImageRef(rt)
			if ref.isNamed() && ref.Tag != noneTag {
				idx.byRepoTag[repoTagKey(ref.Repository, ref.Tag)] = id
			}
		}
	}
	return idx
}

func (idx *imageIndex) lookupID(prefix string) (image.Summary, bool) {
	if idx == nil || prefix == "" {
		return image.Summary{}, false
	}
	if img, ok := idx.byID[prefix]; ok {
		return img, true
	}
	var found image.Summary
	matches := 0
	for id, img := range idx.byID {
		if strings.HasPrefix(id, prefix) {
			found = img
			matches++
		}
	}
	return found, matches == 1
}

func (idx *imageIndex) idForRepoTag(repository, tag string) string {
	if idx == nil {
		return ""
	}
	return idx.byRepoTag[repoTagKey(repository, tag)]
}

func (idx *imageIndex) sourceFor(imageID string) (string, bool) {
	if idx == nil {
		return "", false
	}
	img, ok := idx.byID[trimSHA(imageID)]
	if !ok {
		return "", false
	}
	if len(img.RepoDigests) == 0 {
		return "local", true
	}
	for _, rt := range img.RepoTags {
		if ref := parseImageRef(rt); ref.isNamed() {
			return determineImageSource(ref.Repository), true
		}
	}
	for _, rd := range img.RepoDigests {
		if ref := parseImageRef(rd); ref.isNamed() {
			return determineImageSource(ref.Repository), true
		}
	}
	return "", false
}

func (d *Integration) listImages(ctx context.Context) ([]image.Summary, error) {
	res, err := d.client.ImageList(ctx, client.ImageListOptions{All: false})
	if err != nil {
		return nil, fmt.Errorf("failed to list images: %w", err)
	}
	return res.Items, nil
}

type imageRow struct {
	Repository  string
	Tag         string
	ImageID     string
	RepoDigests []string
}

func imageRows(img image.Summary) []imageRow {
	id := trimSHA(img.ID)
	var rows []imageRow
	for _, rt := range img.RepoTags {
		if rt == "<none>:<none>" {
			continue
		}
		ref := parseImageRef(rt)
		if !ref.isNamed() {
			continue
		}
		rows = append(rows, imageRow{
			Repository:  ref.Repository,
			Tag:         ref.Tag,
			ImageID:     id,
			RepoDigests: repoDigestsFor(img.RepoDigests, ref.Repository),
		})
	}
	if len(rows) > 0 {
		return rows
	}
	seen := make(map[string]bool)
	for _, rd := range img.RepoDigests {
		ref := parseImageRef(rd)
		if !ref.isNamed() || ref.Digest == "" || seen[ref.Repository] {
			continue
		}
		seen[ref.Repository] = true
		rows = append(rows, imageRow{
			Repository:  ref.Repository,
			Tag:         noneTag,
			ImageID:     id,
			RepoDigests: repoDigestsFor(img.RepoDigests, ref.Repository),
		})
	}
	return rows
}

func buildImages(images []image.Summary) []models.DockerImage {
	result := make([]models.DockerImage, 0, len(images))
	for _, img := range images {
		var createdAt *time.Time
		if img.Created > 0 {
			t := time.Unix(img.Created, 0)
			createdAt = &t
		}
		for _, row := range imageRows(img) {
			source := "local"
			if len(img.RepoDigests) > 0 {
				source = determineImageSource(row.Repository)
			}
			digest := ""
			if len(row.RepoDigests) > 0 {
				digest = trimSHA(row.RepoDigests[0])
			}
			result = append(result, models.DockerImage{
				Repository: row.Repository,
				Tag:        row.Tag,
				ImageID:    row.ImageID,
				Source:     source,
				SizeBytes:  img.Size,
				CreatedAt:  createdAt,
				Digest:     digest,
				Labels:     img.Labels,
			})
		}
	}
	return result
}
