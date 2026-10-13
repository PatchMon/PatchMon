package store

import (
	"context"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/db"
	"github.com/PatchMon/PatchMon/server-source-code/internal/models"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtime"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Image update check statuses reported by the agent, plus the read-side only values.
const (
	ImageStatusUpdateAvailable = "update_available"
	ImageStatusUnknown         = "unknown"
	ImageStatusUpToDate        = "up_to_date"
	ImageStatusLocalBuild      = "local_build"
	ImageStatusPinned          = "pinned"
	ImageStatusSkipped         = "skipped"
	ImageStatusNotChecked      = "not_checked"
	ImageStatusRecreateNeeded  = "recreate_needed"
)

// DockerImageCheckResult is one validated result from the agent's image update check.
type DockerImageCheckResult struct {
	Repository   string
	Tag          string
	ImageID      string
	Status       string
	LocalDigest  string
	RemoteDigest string
	Reason       string
	Registry     string
}

// DockerImageUpdateTransition is an image that has just moved into update_available on a host.
type DockerImageUpdateTransition struct {
	ImageUUID    string
	Repository   string
	Tag          string
	RemoteDigest string
}

// DockerImageCheckIngestResult summarises one image-check upload.
type DockerImageCheckIngestResult struct {
	Received   int
	Unresolved int
	NewUpdates []DockerImageUpdateTransition
}

// DockerHostAPIID pairs a host with its agent API ID.
type DockerHostAPIID struct {
	HostID string
	ApiID  string
}

// ReceiveImageChecks replaces the host's image-check results with results.
func (s *DockerStore) ReceiveImageChecks(ctx context.Context, hostID string, checkedAt time.Time, results []DockerImageCheckResult) (*DockerImageCheckIngestResult, error) {
	for i := range results {
		r := &results[i]
		r.Repository = sanitizeText(r.Repository)
		r.Tag = sanitizeText(r.Tag)
		r.ImageID = sanitizeText(r.ImageID)
		r.Registry = sanitizeText(r.Registry)
	}
	d := s.db.DB(ctx)
	tx, err := d.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback(context.WithoutCancel(ctx))
	}()
	res, err := receiveImageChecks(ctx, d.Queries.WithTx(tx), hostID, checkedAt, time.Now().UTC(), results)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return res, nil
}

func optionalText(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func receiveImageChecks(ctx context.Context, q *db.Queries, hostID string, checkedAt, now time.Time, results []DockerImageCheckResult) (*DockerImageCheckIngestResult, error) {
	out := &DockerImageCheckIngestResult{}

	repos := make([]string, len(results))
	tags := make([]string, len(results))
	ids := make([]string, len(results))
	for i, r := range results {
		repos[i], tags[i], ids[i] = r.Repository, r.Tag, r.ImageID
	}
	resolved := map[string]string{}
	if len(results) > 0 {
		rows, err := q.ResolveDockerImageIDs(ctx, db.ResolveDockerImageIDsParams{HostID: hostID, Repositories: repos, Tags: tags, ImageIds: ids})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			resolved[dockerImageKey(row.Repository, row.Tag, row.ImageID)] = row.ID
		}
	}

	prevRows, err := q.ListDockerImageChecksByHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	prev := make(map[string]db.ListDockerImageChecksByHostRow, len(prevRows))
	for _, p := range prevRows {
		prev[p.ImageID] = p
	}

	checkedPg := pgtime.From(checkedAt)
	nowPg := pgtime.From(now)
	kept := make([]string, 0, len(results))
	seen := make(map[string]bool, len(results))
	for _, r := range results {
		imageUUID, ok := resolved[dockerImageKey(r.Repository, r.Tag, r.ImageID)]
		if !ok {
			out.Unresolved++
			continue
		}
		if seen[imageUUID] {
			continue
		}
		seen[imageUUID] = true

		old, hadOld := prev[imageUUID]
		wasUpdate := hadOld && old.FirstSeenUpdateAt.Valid &&
			(old.Status == ImageStatusUpdateAvailable || old.Status == ImageStatusUnknown)
		firstSeen := pgtype.Timestamp{}
		switch {
		case r.Status == ImageStatusUpdateAvailable && wasUpdate:
			firstSeen = old.FirstSeenUpdateAt
		case r.Status == ImageStatusUpdateAvailable:
			firstSeen = checkedPg
			out.NewUpdates = append(out.NewUpdates, DockerImageUpdateTransition{
				ImageUUID:    imageUUID,
				Repository:   r.Repository,
				Tag:          r.Tag,
				RemoteDigest: r.RemoteDigest,
			})
		case r.Status == ImageStatusUnknown && wasUpdate:
			firstSeen = old.FirstSeenUpdateAt
		}

		if err := q.UpsertDockerImageCheck(ctx, db.UpsertDockerImageCheckParams{
			ID:                uuid.New().String(),
			HostID:            hostID,
			ImageID:           imageUUID,
			Status:            r.Status,
			LocalDigest:       optionalText(r.LocalDigest),
			RemoteDigest:      optionalText(r.RemoteDigest),
			Registry:          optionalText(r.Registry),
			Reason:            optionalText(r.Reason),
			CheckedAt:         checkedPg,
			FirstSeenUpdateAt: firstSeen,
			UpdatedAt:         nowPg,
		}); err != nil {
			return nil, err
		}
		kept = append(kept, imageUUID)
	}
	out.Received = len(kept)

	if err := q.DeleteStaleDockerImageChecks(ctx, db.DeleteStaleDockerImageChecksParams{
		HostID:   hostID,
		ImageIds: kept,
	}); err != nil {
		return nil, err
	}
	return out, nil
}

// ImageCheckCounts is how many hosts reported each status for one image.
type ImageCheckCounts struct {
	UpdateAvailable int
	Unknown         int
	UpToDate        int
	LocalBuild      int
	Pinned          int
	Skipped         int
}

// AggregateUpdateStatus must keep the same precedence as the update_status filter in queries/docker.sql.
func AggregateUpdateStatus(c ImageCheckCounts) string {
	switch {
	case c.UpdateAvailable > 0:
		return ImageStatusUpdateAvailable
	case c.Unknown > 0:
		return ImageStatusUnknown
	case c.UpToDate > 0:
		return ImageStatusUpToDate
	case c.LocalBuild > 0:
		return ImageStatusLocalBuild
	case c.Pinned > 0:
		return ImageStatusPinned
	case c.Skipped > 0:
		return ImageStatusSkipped
	}
	return ImageStatusNotChecked
}

// ContainerUpdateStatus derives a container's status: pinned, then recreate_needed, then its host's check.
func ContainerUpdateStatus(pinned, imageStale bool, hostCheckStatus string) string {
	if pinned {
		return ImageStatusPinned
	}
	if imageStale {
		return ImageStatusRecreateNeeded
	}
	if hostCheckStatus == "" {
		return ImageStatusNotChecked
	}
	return hostCheckStatus
}

func annotateImages(ctx context.Context, q *db.Queries, imgs []models.DockerImage) error {
	for i := range imgs {
		imgs[i].UpdateStatus = ImageStatusNotChecked
	}
	if len(imgs) == 0 {
		return nil
	}
	ids := make([]string, len(imgs))
	for i := range imgs {
		ids[i] = imgs[i].ID
	}
	aggs, err := q.GetImageCheckAggregatesByImageIDs(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[string]db.GetImageCheckAggregatesByImageIDsRow, len(aggs))
	for _, a := range aggs {
		byID[a.ImageID] = a
	}
	stale, err := q.GetStaleContainerCountsByImageIDs(ctx, ids)
	if err != nil {
		return err
	}
	staleByID := make(map[string]int, len(stale))
	for _, r := range stale {
		if r.ImageID != nil {
			staleByID[*r.ImageID] = int(r.Cnt)
		}
	}
	for i := range imgs {
		img := &imgs[i]
		img.ContainersNeedingRecreate = staleByID[img.ID]
		a, ok := byID[img.ID]
		if !ok {
			continue
		}
		img.UpdateStatus = AggregateUpdateStatus(ImageCheckCounts{
			UpdateAvailable: int(a.HostsUpdateAvailable),
			Unknown:         int(a.HostsUnknown),
			UpToDate:        int(a.HostsUpToDate),
			LocalBuild:      int(a.HostsLocalBuild),
			Pinned:          int(a.HostsPinned),
			Skipped:         int(a.HostsSkipped),
		})
		img.HasUpdates = img.UpdateStatus == ImageStatusUpdateAvailable
		img.UpdateCheck = &models.DockerImageUpdateCheck{
			CheckedAt:            pgTime(a.CheckedAt),
			HostsChecked:         int(a.HostsChecked),
			HostsUpdateAvailable: int(a.HostsUpdateAvailable),
			RemoteDigest:         a.RemoteDigest,
			Reason:               a.Reason,
		}
	}
	return nil
}

func annotateImagesForHost(ctx context.Context, q *db.Queries, hostID string, imgs []models.DockerImage, containers []models.DockerContainer) error {
	checks, err := q.GetImageChecksByHostID(ctx, hostID)
	if err != nil {
		return err
	}
	byID := make(map[string]db.GetImageChecksByHostIDRow, len(checks))
	for _, c := range checks {
		byID[c.ImageID] = c
	}
	staleByID := map[string]int{}
	for _, c := range containers {
		if c.ImageStale && c.ImageID != nil {
			staleByID[*c.ImageID]++
		}
	}
	for i := range imgs {
		img := &imgs[i]
		img.UpdateStatus = ImageStatusNotChecked
		img.UpdateCheck = nil
		img.ContainersNeedingRecreate = staleByID[img.ID]
		c, ok := byID[img.ID]
		if !ok {
			img.HasUpdates = false
			continue
		}
		img.UpdateStatus = c.Status
		img.HasUpdates = c.Status == ImageStatusUpdateAvailable
		available := 0
		if img.HasUpdates {
			available = 1
		}
		img.UpdateCheck = &models.DockerImageUpdateCheck{
			CheckedAt:            pgTime(c.CheckedAt),
			HostsChecked:         1,
			HostsUpdateAvailable: available,
			RemoteDigest:         c.RemoteDigest,
			Reason:               c.Reason,
		}
	}
	return nil
}

func annotateContainers(ctx context.Context, q *db.Queries, containers []models.DockerContainer) error {
	var ids []string
	seen := map[string]bool{}
	for _, c := range containers {
		if c.ImageID != nil && !seen[*c.ImageID] {
			seen[*c.ImageID] = true
			ids = append(ids, *c.ImageID)
		}
	}
	statuses := map[string]string{}
	if len(ids) > 0 {
		rows, err := q.GetImageCheckStatusesByImageIDs(ctx, ids)
		if err != nil {
			return err
		}
		for _, r := range rows {
			statuses[r.HostID+"|"+r.ImageID] = r.Status
		}
	}
	for i := range containers {
		c := &containers[i]
		status := ""
		if c.ImageID != nil {
			status = statuses[c.HostID+"|"+*c.ImageID]
		}
		c.UpdateStatus = ContainerUpdateStatus(c.ImageDigest != nil && *c.ImageDigest != "", c.ImageStale, status)
	}
	return nil
}

// ListDockerHostAPIIDs returns every host in this context that has Docker inventory.
func (s *DockerStore) ListDockerHostAPIIDs(ctx context.Context) ([]DockerHostAPIID, error) {
	rows, err := s.db.DB(ctx).Queries.ListDockerHostAPIIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DockerHostAPIID, len(rows))
	for i, r := range rows {
		out[i] = DockerHostAPIID{HostID: r.ID, ApiID: r.ApiID}
	}
	return out, nil
}
