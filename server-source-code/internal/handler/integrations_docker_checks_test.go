package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validImageCheck() dockerImageCheckResultReq {
	return dockerImageCheckResultReq{
		Repository:   "nginx",
		Tag:          "1.25.0",
		ImageID:      strings.Repeat("1", 64),
		Status:       "update_available",
		LocalDigest:  "sha256:" + strings.Repeat("a", 64),
		RemoteDigest: "sha256:" + strings.Repeat("b", 64),
		Registry:     "registry-1.docker.io",
	}
}

func TestValidateDockerImageChecks(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*dockerImageCheckResultReq)
		wantErr bool
	}{
		{"valid", func(*dockerImageCheckResultReq) {}, false},
		{"unknown with reason and no digests", func(r *dockerImageCheckResultReq) {
			r.Status, r.Reason, r.LocalDigest, r.RemoteDigest = "unknown", "rate_limited", "", ""
		}, false},
		{"bad status", func(r *dockerImageCheckResultReq) { r.Status = "outdated" }, true},
		{"not_checked is read-side only", func(r *dockerImageCheckResultReq) { r.Status = "not_checked" }, true},
		{"bad reason", func(r *dockerImageCheckResultReq) { r.Reason = "because" }, true},
		{"uppercase digest", func(r *dockerImageCheckResultReq) { r.RemoteDigest = "sha256:" + strings.Repeat("B", 64) }, true},
		{"short digest", func(r *dockerImageCheckResultReq) { r.LocalDigest = "sha256:abc" }, true},
		{"digest without prefix", func(r *dockerImageCheckResultReq) { r.LocalDigest = strings.Repeat("a", 64) }, true},
		{"empty repository", func(r *dockerImageCheckResultReq) { r.Repository = "" }, true},
		{"long repository", func(r *dockerImageCheckResultReq) { r.Repository = strings.Repeat("r", 513) }, true},
		{"long tag", func(r *dockerImageCheckResultReq) { r.Tag = strings.Repeat("t", 129) }, true},
		{"long registry", func(r *dockerImageCheckResultReq) { r.Registry = strings.Repeat("g", 256) }, true},
		{"empty image id", func(r *dockerImageCheckResultReq) { r.ImageID = "" }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := validImageCheck()
			tc.mutate(&r)
			p := dockerImageChecksReq{Results: []dockerImageCheckResultReq{r}}
			if err := validateDockerImageChecks(&p); (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateDockerImageChecksLimits(t *testing.T) {
	p := dockerImageChecksReq{Results: make([]dockerImageCheckResultReq, maxImageCheckResults+1)}
	for i := range p.Results {
		p.Results[i] = validImageCheck()
	}
	if err := validateDockerImageChecks(&p); err == nil {
		t.Fatal("accepted more than the maximum number of results")
	}
	p.Results = p.Results[:maxImageCheckResults]
	if err := validateDockerImageChecks(&p); err != nil {
		t.Fatalf("rejected exactly the maximum: %v", err)
	}
	p.AgentVersion = strings.Repeat("v", 65)
	if err := validateDockerImageChecks(&p); err == nil {
		t.Fatal("accepted an over-long agent_version")
	}
}

func TestValidateDockerImageChecksStripsImageIDPrefix(t *testing.T) {
	r := validImageCheck()
	r.ImageID = "sha256:" + strings.Repeat("1", 64)
	p := dockerImageChecksReq{Results: []dockerImageCheckResultReq{r}}
	if err := validateDockerImageChecks(&p); err != nil {
		t.Fatal(err)
	}
	if p.Results[0].ImageID != strings.Repeat("1", 64) {
		t.Fatalf("image_id = %q", p.Results[0].ImageID)
	}
}

func TestImageCheckTime(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	if got := imageCheckTime(time.Time{}, now); !got.Equal(now) {
		t.Errorf("zero checked_at = %v", got)
	}
	if got := imageCheckTime(now.Add(time.Hour), now); !got.Equal(now) {
		t.Errorf("future checked_at = %v", got)
	}
	past := now.Add(-time.Hour)
	if got := imageCheckTime(past, now); !got.Equal(past) {
		t.Errorf("past checked_at = %v", got)
	}
}

func TestDockerImageChecksPayloadDecodes(t *testing.T) {
	body := `{"checked_at":"2026-10-13T12:00:00Z","agent_version":"2.1.4","results":[{"repository":"nginx","tag":"1.25.0","image_id":"abc","status":"unknown","local_digest":"","remote_digest":"","reason":"tag_not_found","registry":"registry-1.docker.io"}]}`
	var p dockerImageChecksReq
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if err := validateDockerImageChecks(&p); err != nil {
		t.Fatal(err)
	}
	if p.Results[0].Reason != "tag_not_found" || p.CheckedAt.IsZero() {
		t.Fatalf("decoded %+v", p)
	}
}

func TestConvertDockerPayloadCarriesContainerState(t *testing.T) {
	body := `{"containers":[{"container_id":"c1","image_stale":true,"image_digest":"sha256:abc","restart_count":4,"started_at":"2026-10-13T11:00:00Z"}],"updates":[{"repository":"x"}]}`
	var p dockerPayloadReq
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	c := convertDockerPayloadToStore(&p).Containers[0]
	if !c.ImageStale || c.RestartCount != 4 || c.StartedAt == nil || c.ImageDigest != "sha256:abc" {
		t.Fatalf("container = %+v", c)
	}
}

func TestImageUpdateEventText(t *testing.T) {
	title, msg := imageUpdateEventText("web-1", []string{"nginx:1.25.0"}, 1)
	if title != "Docker image update available: nginx:1.25.0" || msg != `A newer image is available for "nginx:1.25.0" on host "web-1".` {
		t.Errorf("single: %q / %q", title, msg)
	}
	title, msg = imageUpdateEventText("web-1", []string{"nginx:1.25.0", "redis:7"}, 2)
	if title != "2 Docker image updates available on web-1" || msg != `Newer images are available for 2 images on host "web-1": nginx:1.25.0, redis:7.` {
		t.Errorf("two: %q / %q", title, msg)
	}
	refs := make([]string, maxImageUpdateRefs)
	for i := range refs {
		refs[i] = "img:" + strings.Repeat("x", i+1)
	}
	_, msg = imageUpdateEventText("web-1", refs, 13)
	if !strings.HasSuffix(msg, " and 3 more.") || strings.ContainsRune(msg, '—') {
		t.Errorf("truncated: %q", msg)
	}
}
