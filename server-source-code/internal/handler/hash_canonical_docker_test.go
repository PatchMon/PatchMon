package handler

import (
	"testing"
	"time"
)

func TestDockerHashContainerStateFields(t *testing.T) {
	started := time.Date(2026, 10, 13, 11, 59, 30, 0, time.UTC)
	restarted := started.Add(time.Minute)
	sameInstantOtherZone := started.In(time.FixedZone("BST", 3600)).Add(123 * time.Nanosecond)
	var zero time.Time

	hash := func(c DockerWireContainer) string {
		t.Helper()
		c.ContainerID = "c1"
		h, err := CanonicalDockerHash(DockerHashInput{Containers: []DockerWireContainer{c}})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}

	base := hash(DockerWireContainer{StartedAt: &started})
	if hash(DockerWireContainer{StartedAt: &restarted}) == base {
		t.Error("a restart did not change the hash")
	}
	if hash(DockerWireContainer{StartedAt: &started, ImageStale: true}) == base {
		t.Error("image_stale did not change the hash")
	}
	if hash(DockerWireContainer{StartedAt: &sameInstantOtherZone}) != base {
		t.Error("zone or sub-second precision changed the hash")
	}
	if hash(DockerWireContainer{}) != hash(DockerWireContainer{StartedAt: &zero}) {
		t.Error("nil and zero started_at hash differently")
	}
}

func TestDockerHashMatchesPreUpgradeAgents(t *testing.T) {
	in := DockerHashInput{
		Containers: []DockerWireContainer{
			{ContainerID: "c2", Name: "web", ImageRepository: "nginx", ImageTag: "1.25", ImageID: "sha256:img2", Status: "running", State: "Up 2 hours"},
			{ContainerID: "c1", Name: "redis", ImageRepository: "redis", ImageTag: "7", ImageID: "sha256:img1", Status: "running", State: "Up 5 minutes"},
		},
		Images:   []DockerWireImage{{Repository: "nginx", Tag: "1.25", ImageID: "sha256:img2", Digest: "sha256:abc", SizeBytes: 12345678}},
		Volumes:  []DockerWireVolume{{VolumeID: "vol1", Name: "data", Driver: "local", Scope: "local"}},
		Networks: []DockerWireNetwork{{NetworkID: "net1", Name: "bridge", Driver: "bridge", Scope: "local"}},
		Updates:  []DockerWireImageUpdate{},
	}
	got, err := CanonicalDockerHash(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := "8fd8bc9bc047aaeab2e561f0181fb2d3e36b36a1a7bb1040d80d5ae8b3634234"; got != want {
		t.Fatalf("old-shape docker hash = %s, want %s (origin/main)", got, want)
	}
}
