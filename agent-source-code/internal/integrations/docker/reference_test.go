package docker

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
)

const (
	hexA = "17868f7231bc0c4ff5d42f0a6e0fbbfc5d3ef2e1e1f8f9ca7d6b5a4a3a2a1a0f"
	hexB = "b997b9ab1bcbd44e4c2eb3ffbd3a8b9a8c8a6b4d0c8e3f2a1b0c9d8e7f6a5b4c"
	hexC = "aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11aa11"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		in   string
		want imageRef
	}{
		{"nginx", imageRef{Repository: "nginx", Tag: "latest"}},
		{"nginx:1.25.0", imageRef{Repository: "nginx", Tag: "1.25.0"}},
		{"docker.io/library/nginx:1.25", imageRef{Repository: "nginx", Tag: "1.25"}},
		{"library/nginx", imageRef{Repository: "nginx", Tag: "latest"}},
		{"portainer/portainer-ce:lts", imageRef{Repository: "portainer/portainer-ce", Tag: "lts"}},
		{"ghcr.io/x/y:1", imageRef{Repository: "ghcr.io/x/y", Tag: "1"}},
		{"ghcr.io/home-assistant/home-assistant:2024.1.0", imageRef{Repository: "ghcr.io/home-assistant/home-assistant", Tag: "2024.1.0"}},
		{"host:5000/a/b:tag", imageRef{Repository: "host:5000/a/b", Tag: "tag"}},
		{"localhost:5000/team/app", imageRef{Repository: "localhost:5000/team/app", Tag: "latest"}},
		{"busybox@sha256:" + hexB, imageRef{Repository: "busybox", Tag: noneTag, Digest: "sha256:" + hexB}},
		{"nginx:1.25@sha256:" + hexB, imageRef{Repository: "nginx", Tag: "1.25", Digest: "sha256:" + hexB}},
		{"sha256:" + hexA, imageRef{ID: hexA, IDOnly: true}},
		{hexA, imageRef{ID: hexA, IDOnly: true}},
		{"sha256:17868f7231bc", imageRef{ID: "17868f7231bc", IDOnly: true}},
		{"17868f7231bc", imageRef{Repository: "17868f7231bc", Tag: "latest", ID: "17868f7231bc"}},
		{"Not A Ref", imageRef{}},
		{"", imageRef{}},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := parseImageRef(tt.in); got != tt.want {
				t.Errorf("parseImageRef(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}

func testIndex() *imageIndex {
	return newImageIndex([]image.Summary{
		{
			ID:          "sha256:" + hexA,
			RepoTags:    []string{"nginx:1.25.0"},
			RepoDigests: []string{"nginx@sha256:" + hexB},
		},
		{
			ID:          "sha256:" + hexC,
			RepoTags:    []string{"localhost:5000/team/app:stable"},
			RepoDigests: []string{"localhost:5000/team/app@sha256:" + hexC},
		},
		{
			ID:       "sha256:" + hexB,
			RepoTags: []string{"local/myapp:dev"},
		},
	})
}

func TestResolveContainerRef(t *testing.T) {
	idx := testIndex()
	tests := []struct {
		name, raw, imageID string
		repo, tag          string
	}{
		{"named", "nginx:1.25.0", hexA, "nginx", "1.25.0"},
		{"full id resolves to single tag", "sha256:" + hexA, hexA, "nginx", "1.25.0"},
		{"bare short id", hexA[:12], hexA, "nginx", "1.25.0"},
		{"unknown id falls back to container image", "sha256:" + strings.Repeat("0", 64), hexC, "localhost:5000/team/app", "stable"},
		{"unknown id and image", "sha256:" + strings.Repeat("0", 64), strings.Repeat("1", 64), noneTag, noneTag},
		{"hex that is really a repo name", "abcdefabcdef", hexA, "abcdefabcdef", "latest"},
		{"unparsable", "UPPER", hexA, noneTag, noneTag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveContainerRef(tt.raw, tt.imageID, idx)
			if got.Repository != tt.repo || got.Tag != tt.tag {
				t.Errorf("got %s:%s, want %s:%s", got.Repository, got.Tag, tt.repo, tt.tag)
			}
		})
	}
}

func TestBuildContainerStaleAndInspect(t *testing.T) {
	idx := testIndex()
	old := strings.Repeat("d", 64)
	started := "2026-10-13T11:59:30.5+01:00"

	summary := container.Summary{
		ID:      strings.Repeat("e", 64),
		Names:   []string{"/app"},
		Image:   "sha256:" + old,
		ImageID: "sha256:" + old,
		State:   container.StateRunning,
	}
	inspect := &container.InspectResponse{
		Image:        "sha256:" + old,
		RestartCount: 3,
		State:        &container.State{StartedAt: started},
		Config:       &container.Config{Image: "localhost:5000/team/app:stable"},
	}

	got := buildContainer(summary, inspect, idx)
	if got.ImageRepository != "localhost:5000/team/app" || got.ImageName != got.ImageRepository || got.ImageTag != "stable" {
		t.Fatalf("image ref = %s / %s:%s", got.ImageName, got.ImageRepository, got.ImageTag)
	}
	if !got.ImageStale {
		t.Error("container on a superseded image should be stale")
	}
	if got.RestartCount != 3 {
		t.Errorf("restart_count = %d", got.RestartCount)
	}
	want, _ := time.Parse(time.RFC3339Nano, started)
	if got.StartedAt == nil || !got.StartedAt.Equal(want) {
		t.Errorf("started_at = %v, want %v", got.StartedAt, want)
	}
	if got.ImageSource != "private" {
		t.Errorf("source = %q, want private", got.ImageSource)
	}

	inspect.Image = "sha256:" + hexC
	if buildContainer(summary, inspect, idx).ImageStale {
		t.Error("container on the current image must not be stale")
	}

	inspect.Config.Image = "localhost:5000/team/app@sha256:" + hexC
	pinned := buildContainer(summary, inspect, idx)
	if pinned.ImageStale || pinned.ImageDigest != "sha256:"+hexC || pinned.ImageTag != noneTag {
		t.Errorf("digest pinned container = %+v", pinned)
	}

	inspect.State.StartedAt = "0001-01-01T00:00:00Z"
	inspect.Config.Image = "local/myapp:dev"
	inspect.Image = "sha256:" + hexB
	local := buildContainer(summary, inspect, idx)
	if local.StartedAt != nil {
		t.Error("zero started_at must be omitted")
	}
	if local.ImageSource != "local" {
		t.Errorf("locally built image source = %q", local.ImageSource)
	}

	if fallback := buildContainer(summary, nil, nil); fallback.ImageRepository != noneTag || fallback.ImageID != old {
		t.Errorf("list-only fallback = %+v", fallback)
	}
}

func TestImageRows(t *testing.T) {
	img := image.Summary{
		ID:       "sha256:" + hexA,
		RepoTags: []string{"nginx:1.25.0", "myrepo/nginx:mine", "<none>:<none>"},
		RepoDigests: []string{
			"myrepo/nginx@sha256:" + hexC,
			"nginx@sha256:" + hexB,
		},
	}
	want := []imageRow{
		{Repository: "nginx", Tag: "1.25.0", ImageID: hexA, RepoDigests: []string{"sha256:" + hexB}},
		{Repository: "myrepo/nginx", Tag: "mine", ImageID: hexA, RepoDigests: []string{"sha256:" + hexC}},
	}
	if got := imageRows(img); !reflect.DeepEqual(got, want) {
		t.Errorf("tagged rows = %+v", got)
	}

	digestOnly := image.Summary{ID: "sha256:" + hexA, RepoDigests: []string{"busybox@sha256:" + hexB}}
	want = []imageRow{{Repository: "busybox", Tag: noneTag, ImageID: hexA, RepoDigests: []string{"sha256:" + hexB}}}
	if got := imageRows(digestOnly); !reflect.DeepEqual(got, want) {
		t.Errorf("digest-only rows = %+v", got)
	}

	if got := imageRows(image.Summary{ID: "sha256:" + hexA, RepoTags: []string{"<none>:<none>"}}); len(got) != 0 {
		t.Errorf("dangling image rows = %+v", got)
	}
}

func TestBuildImagesSourceAndDigest(t *testing.T) {
	got := buildImages([]image.Summary{
		{ID: "sha256:" + hexA, RepoTags: []string{"ghcr.io/x/y:1"}, RepoDigests: []string{"ghcr.io/x/y@sha256:" + hexB}},
		{ID: "sha256:" + hexB, RepoTags: []string{"local/myapp:dev"}},
	})
	if len(got) != 2 {
		t.Fatalf("got %d images", len(got))
	}
	if got[0].Repository != "ghcr.io/x/y" || got[0].Source != "github" || got[0].Digest != hexB {
		t.Errorf("ghcr image = %+v", got[0])
	}
	if got[1].Source != "local" || got[1].Digest != "" {
		t.Errorf("local image = %+v", got[1])
	}
}

func TestConvertPorts(t *testing.T) {
	v4 := netip.MustParseAddr("0.0.0.0")
	v6 := netip.MustParseAddr("::")
	lo := netip.MustParseAddr("127.0.0.1")
	tests := []struct {
		name  string
		ports []container.PortSummary
		want  map[string]string
	}{
		{"v6 then v4 prefers v4", []container.PortSummary{
			{IP: v6, PrivatePort: 80, PublicPort: 8088, Type: "tcp"},
			{IP: v4, PrivatePort: 80, PublicPort: 8088, Type: "tcp"},
		}, map[string]string{"80/tcp": "0.0.0.0:8088"}},
		{"v4 then v6 keeps v4", []container.PortSummary{
			{IP: v4, PrivatePort: 80, PublicPort: 8088, Type: "tcp"},
			{IP: v6, PrivatePort: 80, PublicPort: 8088, Type: "tcp"},
		}, map[string]string{"80/tcp": "0.0.0.0:8088"}},
		{"v6 only", []container.PortSummary{
			{IP: v6, PrivatePort: 53, PublicPort: 5353, Type: "udp"},
		}, map[string]string{"53/udp": "[::]:5353"}},
		{"loopback and unpublished", []container.PortSummary{
			{IP: lo, PrivatePort: 6379, PublicPort: 6379, Type: "tcp"},
			{PrivatePort: 9000, Type: "tcp"},
		}, map[string]string{"6379/tcp": "127.0.0.1:6379"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := convertPorts(tt.ports); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
