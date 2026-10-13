package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/PatchMon/PatchMon/server-source-code/internal/database"
	"github.com/PatchMon/PatchMon/server-source-code/internal/pgtest"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestAggregateUpdateStatus(t *testing.T) {
	cases := []struct {
		name string
		in   ImageCheckCounts
		want string
	}{
		{"never checked", ImageCheckCounts{}, ImageStatusNotChecked},
		{"any update wins", ImageCheckCounts{UpdateAvailable: 1, Unknown: 3, UpToDate: 5}, ImageStatusUpdateAvailable},
		{"unknown beats up to date", ImageCheckCounts{Unknown: 1, UpToDate: 4}, ImageStatusUnknown},
		{"all up to date", ImageCheckCounts{UpToDate: 2, LocalBuild: 1}, ImageStatusUpToDate},
		{"local build only", ImageCheckCounts{LocalBuild: 2}, ImageStatusLocalBuild},
		{"pinned only", ImageCheckCounts{Pinned: 1, Skipped: 1}, ImageStatusPinned},
		{"skipped only", ImageCheckCounts{Skipped: 1}, ImageStatusSkipped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AggregateUpdateStatus(tc.in); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestContainerUpdateStatus(t *testing.T) {
	cases := []struct {
		pinned bool
		stale  bool
		status string
		want   string
	}{
		{true, true, ImageStatusUpdateAvailable, ImageStatusPinned},
		{true, false, "", ImageStatusPinned},
		{false, true, ImageStatusUpToDate, ImageStatusRecreateNeeded},
		{false, true, ImageStatusUpdateAvailable, ImageStatusRecreateNeeded},
		{false, true, "", ImageStatusRecreateNeeded},
		{false, false, ImageStatusUpdateAvailable, ImageStatusUpdateAvailable},
		{false, false, ImageStatusUnknown, ImageStatusUnknown},
		{false, false, "", ImageStatusNotChecked},
	}
	for _, tc := range cases {
		if got := ContainerUpdateStatus(tc.pinned, tc.stale, tc.status); got != tc.want {
			t.Errorf("ContainerUpdateStatus(%v, %v, %q) = %q, want %q", tc.pinned, tc.stale, tc.status, got, tc.want)
		}
	}
}

type dockerPG struct {
	t   *testing.T
	ctx context.Context
	d   *database.DB
	s   *DockerStore
}

func newDockerPG(t *testing.T) *dockerPG {
	t.Helper()
	ctx := context.Background()
	d := pgtest.Open(t)
	return &dockerPG{t: t, ctx: ctx, d: d, s: NewDockerStore(d)}
}

func (p *dockerPG) exec(sql string, args ...interface{}) {
	p.t.Helper()
	if _, err := p.d.Exec(p.ctx, sql, args...); err != nil {
		p.t.Fatal(err)
	}
}

func (p *dockerPG) strings(sql string, args ...interface{}) []string {
	p.t.Helper()
	rows, err := p.d.Raw(p.ctx, sql, args...)
	if err != nil {
		p.t.Fatal(err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		p.t.Fatal(err)
	}
	return out
}

func (p *dockerPG) hosts(n int) (string, []string) {
	p.t.Helper()
	run := uuid.NewString()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("h%d-%s", i+1, run)
		p.exec(`INSERT INTO hosts (id, friendly_name, os_type, os_version, updated_at, api_id, api_key)
			VALUES ($1, $1, 'linux', '1', now(), $1, $1)`, ids[i])
	}
	repoPrefix := "imgcheck-" + run
	p.t.Cleanup(func() {
		p.exec(`DELETE FROM hosts WHERE id = ANY($1)`, ids)
		p.exec(`DELETE FROM docker_images WHERE repository LIKE $1`, repoPrefix+"%")
	})
	return repoPrefix, ids
}

func (p *dockerPG) receive(hostID string, payload DockerReceivePayload) {
	p.t.Helper()
	if _, err := p.s.ReceiveDockerData(p.ctx, hostID, &payload); err != nil {
		p.t.Fatal(err)
	}
}

func (p *dockerPG) imageUUID(repo, tag, imageID string) string {
	p.t.Helper()
	ids := p.strings(`SELECT id FROM docker_images WHERE repository = $1 AND tag = $2 AND image_id = $3`, repo, tag, imageID)
	if len(ids) != 1 {
		p.t.Fatalf("image %s:%s@%s: got %d rows", repo, tag, imageID, len(ids))
	}
	return ids[0]
}

func digestOf(c byte) string { return "sha256:" + strings.Repeat(string(c), 64) }

func TestDockerIngestKeepsImageMetadataForInUseImages(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(1)
	h := hosts[0]
	created := time.Date(2023, 6, 13, 10, 0, 0, 0, time.UTC)
	containerCreated := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)

	p.receive(h, DockerReceivePayload{
		Containers: []DockerReceiveContainer{{ContainerID: "c1", Name: "web", ImageRepository: repo, ImageTag: "1.25.0", ImageID: "img1", ImageSource: "docker-hub", Status: "running", CreatedAt: &containerCreated}},
		Collected:  []string{"containers"},
	})

	p.receive(h, DockerReceivePayload{
		Containers: []DockerReceiveContainer{{ContainerID: "c1", Name: "web", ImageRepository: repo, ImageTag: "1.25.0", ImageID: "img1", ImageSource: "docker-hub", Status: "running", CreatedAt: &containerCreated, RestartCount: 3, ImageStale: true}},
		Images:     []DockerReceiveImage{{Repository: repo, Tag: "1.25.0", ImageID: "img1", Source: "local", SizeBytes: 1234, CreatedAt: &created, Digest: digestOf('a')}},
		Collected:  []string{"containers", "images"},
	})

	check := func(step string) {
		t.Helper()
		var digest string
		var size int64
		var gotCreated time.Time
		var source string
		row := p.d.RawQueryRow(p.ctx, `SELECT digest, size_bytes, created_at, source FROM docker_images WHERE repository = $1`, repo)
		if err := row.Scan(&digest, &size, &gotCreated, &source); err != nil {
			t.Fatalf("%s: %v", step, err)
		}
		if digest != digestOf('a') || size != 1234 || !gotCreated.Equal(created) || source != "local" {
			t.Fatalf("%s: got digest=%q size=%d created=%v source=%q", step, digest, size, gotCreated, source)
		}
	}
	check("after images upload")

	p.receive(h, DockerReceivePayload{
		Containers: []DockerReceiveContainer{{ContainerID: "c1", Name: "web", ImageRepository: repo, ImageTag: "1.25.0", ImageID: "img1", ImageSource: "docker-hub", Status: "running", CreatedAt: &containerCreated}},
	})
	check("after container-only upload")

	var stale bool
	var restarts int
	if err := p.d.RawQueryRow(p.ctx, `SELECT image_stale, restart_count FROM docker_containers WHERE host_id = $1`, h).Scan(&stale, &restarts); err != nil {
		t.Fatal(err)
	}
	if stale || restarts != 0 {
		t.Fatalf("container flags not replaced: stale=%v restarts=%d", stale, restarts)
	}
	if n := len(p.strings(`SELECT id FROM docker_images WHERE repository = $1`, repo)); n != 1 {
		t.Fatalf("got %d image rows, want 1", n)
	}
}

func TestDockerHostImageLinksReplace(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(2)
	h1, h2 := hosts[0], hosts[1]
	img := func(tag string) DockerReceiveImage {
		return DockerReceiveImage{Repository: repo, Tag: tag, ImageID: "id-" + tag, Source: "docker-hub"}
	}
	links := func(hostID string) []string {
		t.Helper()
		return p.strings(`SELECT di.tag FROM docker_host_images dhi JOIN docker_images di ON di.id = dhi.image_id
			WHERE dhi.host_id = $1 ORDER BY di.tag`, hostID)
	}
	all := []string{"containers", "images", "volumes", "networks"}

	p.receive(h1, DockerReceivePayload{Images: []DockerReceiveImage{img("a"), img("b")}, Collected: all})
	p.receive(h2, DockerReceivePayload{Images: []DockerReceiveImage{img("a"), img("b")}, Collected: all})
	if got := links(h1); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("initial links = %v", got)
	}

	p.receive(h1, DockerReceivePayload{
		Images:     []DockerReceiveImage{img("a")},
		Containers: []DockerReceiveContainer{{ContainerID: "c1", Name: "c1", ImageRepository: repo, ImageTag: "c", ImageID: "id-c", Status: "running"}},
		Collected:  all,
	})
	if got := links(h1); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("after replace links = %v, want [a c]", got)
	}
	if got := links(h2); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("other host links = %v", got)
	}

	p.receive(h1, DockerReceivePayload{})
	if got := links(h1); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("old agent empty upload changed links: %v", got)
	}

	p.receive(h1, DockerReceivePayload{Collected: []string{"networks"}})
	if got := links(h1); !slices.Equal(got, []string{"a", "c"}) {
		t.Fatalf("failed image collection changed links: %v", got)
	}

	orphans := func() []string {
		t.Helper()
		return p.strings(`SELECT di.tag FROM docker_images di
			WHERE di.repository = $1
			AND NOT EXISTS (SELECT 1 FROM docker_containers dc WHERE dc.image_id = di.id)
			AND NOT EXISTS (SELECT 1 FROM docker_host_images dhi WHERE dhi.image_id = di.id) ORDER BY 1`, repo)
	}
	q := p.d.Queries
	rows, err := q.ListOrphanedImages(p.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Repository == repo {
			t.Fatalf("image %s linked to a host was listed as orphaned", r.Tag)
		}
	}
	p.receive(h2, DockerReceivePayload{Collected: all})
	if got := orphans(); !slices.Equal(got, []string{"b"}) {
		t.Fatalf("orphans after h2 cleared = %v, want [b]", got)
	}
}

func TestDockerImageChecksIngest(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(2)
	h1, h2 := hosts[0], hosts[1]
	all := []string{"containers", "images", "volumes", "networks"}
	images := []DockerReceiveImage{
		{Repository: repo, Tag: "a", ImageID: "id-a", Digest: digestOf('a')},
		{Repository: repo, Tag: "b", ImageID: "id-b", Digest: digestOf('b')},
	}
	p.receive(h1, DockerReceivePayload{Images: images, Collected: all,
		Containers: []DockerReceiveContainer{
			{ContainerID: "c-a", Name: "ca", ImageRepository: repo, ImageTag: "a", ImageID: "id-a", Status: "running"},
			{ContainerID: "c-b", Name: "cb", ImageRepository: repo, ImageTag: "b", ImageID: "id-b", Status: "running", ImageStale: true},
		}})
	p.receive(h2, DockerReceivePayload{Images: images, Collected: all})
	idA := p.imageUUID(repo, "a", "id-a")

	result := func(tag, status string) DockerImageCheckResult {
		r := DockerImageCheckResult{Repository: repo, Tag: tag, ImageID: "id-" + tag, Status: status, LocalDigest: digestOf(tag[0]), Registry: "registry-1.docker.io"}
		if status == ImageStatusUpdateAvailable {
			r.RemoteDigest = digestOf('f')
		}
		return r
	}
	ingest := func(hostID string, at time.Time, rs ...DockerImageCheckResult) *DockerImageCheckIngestResult {
		t.Helper()
		out, err := p.s.ReceiveImageChecks(p.ctx, hostID, at, rs)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	firstSeen := func(hostID, imageID string) *time.Time {
		t.Helper()
		var ts *time.Time
		if err := p.d.RawQueryRow(p.ctx, `SELECT first_seen_update_at FROM docker_image_checks WHERE host_id = $1 AND image_id = $2`, hostID, imageID).Scan(&ts); err != nil {
			t.Fatal(err)
		}
		return ts
	}

	t0 := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	out := ingest(h1, t0, result("a", ImageStatusUpdateAvailable), result("b", ImageStatusUpToDate),
		DockerImageCheckResult{Repository: repo, Tag: "missing", ImageID: "nope", Status: ImageStatusUnknown, Reason: "unreachable"})
	if out.Received != 2 || out.Unresolved != 1 || len(out.NewUpdates) != 1 || out.NewUpdates[0].ImageUUID != idA {
		t.Fatalf("first ingest = %+v", out)
	}
	if fs := firstSeen(h1, idA); fs == nil || !fs.Equal(t0) {
		t.Fatalf("first_seen after transition = %v, want %v", fs, t0)
	}

	t1 := t0.Add(6 * time.Hour)
	out = ingest(h1, t1, result("a", ImageStatusUpdateAvailable), result("b", ImageStatusUpToDate))
	if len(out.NewUpdates) != 0 {
		t.Fatalf("repeat ingest re-alerted: %+v", out.NewUpdates)
	}
	if fs := firstSeen(h1, idA); fs == nil || !fs.Equal(t0) {
		t.Fatalf("first_seen moved on repeat: %v", fs)
	}

	out = ingest(h1, t1.Add(time.Hour), result("a", ImageStatusUpToDate))
	if len(out.NewUpdates) != 0 {
		t.Fatalf("leaving update_available alerted: %+v", out.NewUpdates)
	}
	if fs := firstSeen(h1, idA); fs != nil {
		t.Fatalf("first_seen not cleared: %v", fs)
	}
	if got := p.strings(`SELECT image_id FROM docker_image_checks WHERE host_id = $1`, h1); !slices.Equal(got, []string{idA}) {
		t.Fatalf("replace semantics: rows = %v, want only image a", got)
	}

	t2 := t1.Add(2 * time.Hour)
	out = ingest(h1, t2, result("a", ImageStatusUpdateAvailable))
	if len(out.NewUpdates) != 1 {
		t.Fatalf("re-entering update_available did not alert: %+v", out)
	}
	if fs := firstSeen(h1, idA); fs == nil || !fs.Equal(t2) {
		t.Fatalf("first_seen after re-entry = %v, want %v", fs, t2)
	}

	ingest(h2, t2, result("a", ImageStatusUnknown), result("b", ImageStatusUpToDate))

	imgs, total, err := p.s.ListImages(p.ctx, ImageListParams{Search: repo, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("ListImages total = %d", total)
	}
	byTag := map[string]ImageWithCounts{}
	for _, img := range imgs {
		byTag[img.Tag] = img
	}
	a, b := byTag["a"], byTag["b"]
	if a.UpdateStatus != ImageStatusUpdateAvailable || !a.HasUpdates || a.UpdateCheck == nil ||
		a.UpdateCheck.HostsChecked != 2 || a.UpdateCheck.HostsUpdateAvailable != 1 {
		t.Fatalf("image a = %+v check=%+v", a.DockerImage, a.UpdateCheck)
	}
	if b.UpdateStatus != ImageStatusUpToDate || b.HasUpdates || b.ContainersNeedingRecreate != 1 {
		t.Fatalf("image b = %+v", b.DockerImage)
	}

	for status, want := range map[string][]string{
		ImageStatusUpdateAvailable: {"a"},
		ImageStatusUpToDate:        {"b"},
		ImageStatusRecreateNeeded:  {"b"},
		ImageStatusNotChecked:      nil,
		ImageStatusUnknown:         nil,
	} {
		got, n, err := p.s.ListImages(p.ctx, ImageListParams{Search: repo, UpdateStatus: status, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		var tags []string
		for _, img := range got {
			tags = append(tags, img.Tag)
		}
		if !slices.Equal(tags, want) || n != len(want) {
			t.Errorf("filter %s = %v (count %d), want %v", status, tags, n, want)
		}
	}

	detail, err := p.s.GetHostDockerDetail(p.ctx, h1)
	if err != nil {
		t.Fatal(err)
	}
	statusByTag := map[string]string{}
	for _, img := range detail.Images {
		statusByTag[img.Tag] = img.UpdateStatus
	}
	if statusByTag["a"] != ImageStatusUpdateAvailable || statusByTag["b"] != ImageStatusNotChecked {
		t.Fatalf("host images = %v", statusByTag)
	}
	containerStatus := map[string]string{}
	for _, c := range detail.Containers {
		containerStatus[c.ContainerID] = c.UpdateStatus
	}
	if containerStatus["c-a"] != ImageStatusUpdateAvailable || containerStatus["c-b"] != ImageStatusRecreateNeeded {
		t.Fatalf("container statuses = %v", containerStatus)
	}

	h2detail, err := p.s.GetHostDockerDetail(p.ctx, h2)
	if err != nil {
		t.Fatal(err)
	}
	if len(h2detail.Images) != 2 {
		t.Fatalf("host with unused images lists %d images, want 2", len(h2detail.Images))
	}

	imgDetail, err := p.s.GetImage(p.ctx, idA)
	if err != nil {
		t.Fatal(err)
	}
	if imgDetail.TotalHosts != 2 || len(imgDetail.Image.DockerImageChecks) != 2 || len(imgDetail.Image.DockerContainers) != 1 {
		t.Fatalf("image detail hosts=%d checks=%d containers=%d", imgDetail.TotalHosts, len(imgDetail.Image.DockerImageChecks), len(imgDetail.Image.DockerContainers))
	}

	dash, err := p.s.GetDashboard(p.ctx)
	if err != nil {
		t.Fatal(err)
	}
	stats := dash["stats"].(map[string]interface{})
	if stats["lastImageCheckAt"].(*time.Time) == nil || stats["availableUpdates"].(int32) < 1 || stats["containersNeedingRecreate"].(int32) < 1 {
		t.Fatalf("dashboard stats = %+v", stats)
	}

	if _, err := p.s.ReceiveImageChecks(p.ctx, h1, t2, nil); err != nil {
		t.Fatal(err)
	}
	if got := p.strings(`SELECT image_id FROM docker_image_checks WHERE host_id = $1`, h1); len(got) != 0 {
		t.Fatalf("empty result set left rows: %v", got)
	}
	if got := p.strings(`SELECT image_id FROM docker_image_checks WHERE host_id = $1 ORDER BY 1`, h2); len(got) != 2 {
		t.Fatalf("other host rows touched: %v", got)
	}
}

func TestDockerOrphanImagesHiddenFromListsAndCounts(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(1)
	p.receive(hosts[0], DockerReceivePayload{
		Images:    []DockerReceiveImage{{Repository: repo, Tag: "live", ImageID: "id-live", Source: "orphantest-" + repo}},
		Collected: []string{"images"},
	})
	orphanID := uuid.NewString()
	p.exec(`INSERT INTO docker_images (id, repository, tag, image_id, source, created_at, updated_at)
		VALUES ($1, $2, 'orphan', 'id-orphan', $3, now(), now())`, orphanID, repo, "orphantest-"+repo)

	imgs, total, err := p.s.ListImages(p.ctx, ImageListParams{Search: repo, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(imgs) != 1 || imgs[0].Tag != "live" {
		t.Fatalf("ListImages total=%d images=%+v, want only the live image", total, imgs)
	}
	for _, status := range []string{ImageStatusNotChecked, ImageStatusUpToDate} {
		_, n, err := p.s.ListImages(p.ctx, ImageListParams{Search: repo, UpdateStatus: status, Limit: 50})
		if err != nil {
			t.Fatal(err)
		}
		if want := map[string]int{ImageStatusNotChecked: 1, ImageStatusUpToDate: 0}[status]; n != want {
			t.Errorf("filter %s count = %d, want %d", status, n, want)
		}
	}

	stats, err := p.d.Queries.GetDockerDashboardStats(p.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var linked int32
	if err := p.d.RawQueryRow(p.ctx, `SELECT COUNT(*)::int FROM docker_images di
		WHERE EXISTS (SELECT 1 FROM docker_containers dc WHERE dc.image_id = di.id)
		OR EXISTS (SELECT 1 FROM docker_host_images dhi WHERE dhi.image_id = di.id)`).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if stats.Column3 != linked {
		t.Fatalf("dashboard totalImages = %d, want %d linked images", stats.Column3, linked)
	}

	bySource, err := p.d.Queries.GetImagesBySource(p.ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range bySource {
		if r.Source == "orphantest-"+repo && r.Count != 1 {
			t.Fatalf("imagesBySource counts orphan: %d", r.Count)
		}
	}

	results, err := p.d.Queries.GlobalSearch(p.ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.ID == orphanID {
			t.Fatal("global search returned the orphan image")
		}
	}

	if _, err := p.s.GetImage(p.ctx, orphanID); err != nil {
		t.Fatalf("image detail by id should still return the orphan: %v", err)
	}
}

func TestDockerImageChecksOnlyResolveImagesOnPostingHost(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(2)
	hostA, hostB := hosts[0], hosts[1]
	p.receive(hostA, DockerReceivePayload{Images: []DockerReceiveImage{{Repository: repo, Tag: "shared", ImageID: "id-shared"}}, Collected: []string{"images"}})
	p.receive(hostB, DockerReceivePayload{Images: []DockerReceiveImage{
		{Repository: repo, Tag: "shared", ImageID: "id-shared"},
		{Repository: repo, Tag: "b-only", ImageID: "id-b"},
	}, Collected: []string{"images"}})

	out, err := p.s.ReceiveImageChecks(p.ctx, hostA, time.Now().UTC(), []DockerImageCheckResult{
		{Repository: repo, Tag: "shared", ImageID: "id-shared", Status: ImageStatusUpToDate},
		{Repository: repo, Tag: "b-only", ImageID: "id-b", Status: ImageStatusUpdateAvailable, RemoteDigest: digestOf('f')},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Received != 1 || out.Unresolved != 1 || len(out.NewUpdates) != 0 {
		t.Fatalf("ingest = %+v, want 1 stored, 1 unresolved, no alerts", out)
	}
	if got := p.strings(`SELECT image_id FROM docker_image_checks WHERE image_id = $1`, p.imageUUID(repo, "b-only", "id-b")); len(got) != 0 {
		t.Fatalf("host A wrote a check for an image only host B has: %v", got)
	}
}

func TestDockerImageCheckUnknownKeepsUpdateState(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(1)
	h := hosts[0]
	p.receive(h, DockerReceivePayload{Images: []DockerReceiveImage{{Repository: repo, Tag: "a", ImageID: "id-a"}}, Collected: []string{"images"}})
	id := p.imageUUID(repo, "a", "id-a")

	t0 := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	step := func(n int, status, reason string, wantAlerts int, wantFirstSeen *time.Time) {
		t.Helper()
		r := DockerImageCheckResult{Repository: repo, Tag: "a", ImageID: "id-a", Status: status, Reason: reason}
		out, err := p.s.ReceiveImageChecks(p.ctx, h, t0.Add(time.Duration(n)*time.Hour), []DockerImageCheckResult{r})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.NewUpdates) != wantAlerts {
			t.Fatalf("step %d (%s): %d alerts, want %d", n, status, len(out.NewUpdates), wantAlerts)
		}
		var got *time.Time
		var gotStatus string
		if err := p.d.RawQueryRow(p.ctx, `SELECT status, first_seen_update_at FROM docker_image_checks WHERE host_id = $1 AND image_id = $2`, h, id).Scan(&gotStatus, &got); err != nil {
			t.Fatal(err)
		}
		if gotStatus != status {
			t.Fatalf("step %d: stored status %q, want %q", n, gotStatus, status)
		}
		if (got == nil) != (wantFirstSeen == nil) || (got != nil && !got.Equal(*wantFirstSeen)) {
			t.Fatalf("step %d (%s): first_seen = %v, want %v", n, status, got, wantFirstSeen)
		}
	}
	first := t0
	step(0, ImageStatusUpdateAvailable, "", 1, &first)
	step(1, ImageStatusUnknown, "rate_limited", 0, &first)
	step(2, ImageStatusUpdateAvailable, "", 0, &first)
	step(3, ImageStatusUnknown, "unreachable", 0, &first)
	step(4, ImageStatusUpToDate, "", 0, nil)
	step(5, ImageStatusUnknown, "unreachable", 0, nil)
	again := t0.Add(6 * time.Hour)
	step(6, ImageStatusUpdateAvailable, "", 1, &again)
}

func TestDockerImageChecksPinnedContainerImage(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(1)
	h := hosts[0]
	p.receive(h, DockerReceivePayload{
		Containers: []DockerReceiveContainer{{ContainerID: "c-pin", Name: "pin", ImageRepository: repo, ImageTag: "<none>", ImageID: "id-pin", Status: "running"}},
		Collected:  []string{"containers", "images"},
	})
	out, err := p.s.ReceiveImageChecks(p.ctx, h, time.Now().UTC(), []DockerImageCheckResult{
		{Repository: repo, Tag: "<none>", ImageID: "id-pin", Status: ImageStatusPinned},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Received != 1 || out.Unresolved != 0 {
		t.Fatalf("pinned result did not resolve: %+v", out)
	}
	detail, err := p.s.GetHostDockerDetail(p.ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Containers) != 1 || detail.Containers[0].UpdateStatus != ImageStatusPinned {
		t.Fatalf("containers = %+v", detail.Containers)
	}
}

func TestDockerIngestOverlappingImagesConcurrently(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(2)
	img := func(prefix string, i int) DockerReceiveImage {
		tag := fmt.Sprintf("%s%02d", prefix, i)
		return DockerReceiveImage{Repository: repo, Tag: tag, ImageID: "id-" + tag}
	}
	var aImages, bImages []DockerReceiveImage
	var aContainers []DockerReceiveContainer
	for i := 0; i < 40; i++ {
		a, b := img("a", i), img("b", i)
		aImages = append(aImages, a)
		bImages = append(bImages, b)
		aContainers = append(aContainers, DockerReceiveContainer{ContainerID: "c" + a.Tag, Name: a.Tag, ImageRepository: repo, ImageTag: a.Tag, ImageID: a.ImageID, Status: "running"})
	}
	reversed := func(in []DockerReceiveImage) []DockerReceiveImage {
		out := slices.Clone(in)
		slices.Reverse(out)
		return out
	}
	payloads := [][2]DockerReceivePayload{
		{
			{Images: aImages, Containers: aContainers, Collected: []string{"images", "containers"}},
			{Images: reversed(aImages), Containers: aContainers, Collected: []string{"images", "containers"}},
		},
		{
			{Images: bImages, Containers: aContainers, Collected: []string{"images", "containers"}},
			{Images: append(reversed(bImages), reversed(aImages)...), Collected: []string{"images"}},
		},
	}

	for _, pair := range payloads {
		for round := 0; round < 5; round++ {
			errs := make(chan error, 2)
			for i, payload := range pair {
				go func(hostID string, payload DockerReceivePayload) {
					payload.Images = slices.Clone(payload.Images)
					payload.Containers = slices.Clone(payload.Containers)
					_, err := p.s.ReceiveDockerData(p.ctx, hostID, &payload)
					errs <- err
				}(hosts[i], payload)
			}
			for i := 0; i < 2; i++ {
				if err := <-errs; err != nil {
					t.Fatalf("round %d: %v", round, err)
				}
			}
		}
	}
}

func TestDockerDigestPinnedContainerSharesTaggedImage(t *testing.T) {
	p := newDockerPG(t)
	repo, hosts := p.hosts(1)
	h := hosts[0]
	pin := digestOf('d')
	p.receive(h, DockerReceivePayload{
		Images: []DockerReceiveImage{{Repository: repo, Tag: "stable", ImageID: "id-s", Digest: pin}},
		Containers: []DockerReceiveContainer{
			{ContainerID: "c-pinned", Name: "pinned", ImageRepository: repo, ImageTag: "stable", ImageID: "id-s", ImageDigest: pin, Status: "running"},
			{ContainerID: "c-tagged", Name: "tagged", ImageRepository: repo, ImageTag: "stable", ImageID: "id-s", Status: "running"},
		},
		Collected: []string{"containers", "images"},
	})
	if _, err := p.s.ReceiveImageChecks(p.ctx, h, time.Now().UTC(), []DockerImageCheckResult{
		{Repository: repo, Tag: "stable", ImageID: "id-s", Status: ImageStatusUpdateAvailable, RemoteDigest: digestOf('e')},
	}); err != nil {
		t.Fatal(err)
	}
	detail, err := p.s.GetHostDockerDetail(p.ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range detail.Containers {
		got[c.ContainerID] = c.UpdateStatus
		if c.ContainerID == "c-pinned" && (c.ImageDigest == nil || *c.ImageDigest != pin) {
			t.Fatalf("image_digest not persisted: %v", c.ImageDigest)
		}
	}
	if got["c-pinned"] != ImageStatusPinned || got["c-tagged"] != ImageStatusUpdateAvailable {
		t.Fatalf("container statuses = %v", got)
	}
	if len(detail.Images) != 1 || detail.Images[0].UpdateStatus != ImageStatusUpdateAvailable {
		t.Fatalf("shared tagged image = %+v", detail.Images)
	}

	p.receive(h, DockerReceivePayload{
		Containers: []DockerReceiveContainer{{ContainerID: "c-pinned", Name: "pinned", ImageRepository: repo, ImageTag: "stable", ImageID: "id-s", Status: "running"}},
		Collected:  []string{"containers"},
	})
	var digest *string
	if err := p.d.RawQueryRow(p.ctx, `SELECT image_digest FROM docker_containers WHERE host_id = $1 AND container_id = 'c-pinned'`, h).Scan(&digest); err != nil {
		t.Fatal(err)
	}
	if digest != nil {
		t.Fatalf("image_digest not cleared on recreate: %v", *digest)
	}
}
