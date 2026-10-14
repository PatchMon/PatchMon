package docker

import (
	"context"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"patchmon-agent/internal/integrations/docker/registry"
	"patchmon-agent/pkg/models"

	"github.com/moby/moby/api/types/image"
	registrytypes "github.com/moby/moby/api/types/registry"
)

type fakeResolver struct {
	mu       sync.Mutex
	calls    map[registry.Ref]int
	answers  map[registry.Ref]remoteOutcome
	inFlight map[string]int
	maxHost  int
	maxAll   int
	active   int
}

func (f *fakeResolver) ManifestDigest(_ context.Context, ref registry.Ref) (string, error) {
	f.mu.Lock()
	f.calls[ref]++
	f.inFlight[ref.Host]++
	f.active++
	f.maxHost = max(f.maxHost, f.inFlight[ref.Host])
	f.maxAll = max(f.maxAll, f.active)
	out := f.answers[ref]
	f.mu.Unlock()

	time.Sleep(5 * time.Millisecond)

	f.mu.Lock()
	f.inFlight[ref.Host]--
	f.active--
	f.mu.Unlock()
	return out.digest, out.err
}

func newFakeResolver(answers map[registry.Ref]remoteOutcome) *fakeResolver {
	return &fakeResolver{calls: map[registry.Ref]int{}, answers: answers, inFlight: map[string]int{}}
}

func TestImageChecksStatuses(t *testing.T) {
	images := []image.Summary{
		{ID: "sha256:" + hexA, RepoTags: []string{"nginx:1.25.0", "nginx:old"}, RepoDigests: []string{"nginx@sha256:" + hexB}},
		{ID: "sha256:" + hexB, RepoTags: []string{"ghcr.io/x/y:1"}, RepoDigests: []string{"ghcr.io/x/y@sha256:" + hexC}},
		{ID: "sha256:" + hexC, RepoTags: []string{"local/myapp:dev"}},
		{ID: "sha256:" + hexC, RepoDigests: []string{"busybox@sha256:" + hexA}},
		{ID: "sha256:" + hexB, RepoDigests: []string{"redis@sha256:" + hexB}},
		{ID: "sha256:" + hexC, RepoTags: []string{"retag/nginx:x"}, RepoDigests: []string{"nginx@sha256:" + hexB}},
		{ID: "sha256:" + hexA, RepoTags: []string{"quay.io/a/b:2"}, RepoDigests: []string{"quay.io/a/b@sha256:" + hexA}},
	}
	hub := registry.Ref{Host: registry.DockerHubHost, Repository: "library/nginx", Tag: "1.25.0"}
	hubOld := registry.Ref{Host: registry.DockerHubHost, Repository: "library/nginx", Tag: "old"}
	ghcr := registry.Ref{Host: "ghcr.io", Repository: "x/y", Tag: "1"}
	quay := registry.Ref{Host: "quay.io", Repository: "a/b", Tag: "2"}
	res := newFakeResolver(map[registry.Ref]remoteOutcome{
		hub:    {digest: "sha256:" + hexB},
		hubOld: {digest: "sha256:" + hexA},
		ghcr:   {err: registry.ErrNotFound},
		quay:   {err: registry.ErrAuthFailed},
	})

	pinned := make(pinnedRefs)
	pinned.add(parseImageRef("busybox@sha256:" + hexA))
	pinned.add(parseImageRef("redis@sha256:" + hexA))
	got := runImageChecks(context.Background(), planImageChecks(images, pinned), res)
	want := []struct{ repo, tag, status, reason, remote, registry string }{
		{"nginx", "1.25.0", models.ImageCheckUpToDate, "", "sha256:" + hexB, registry.DockerHubHost},
		{"nginx", "old", models.ImageCheckUpdateAvailable, "", "sha256:" + hexA, registry.DockerHubHost},
		{"ghcr.io/x/y", "1", models.ImageCheckUnknown, models.ImageCheckReasonTagNotFound, "", "ghcr.io"},
		{"local/myapp", "dev", models.ImageCheckLocalBuild, "", "", ""},
		{"busybox", noneTag, models.ImageCheckPinned, "", "", ""},
		{"redis", noneTag, models.ImageCheckSkipped, "", "", ""},
		{"retag/nginx", "x", models.ImageCheckUnknown, models.ImageCheckReasonNoDigest, "", ""},
		{"quay.io/a/b", "2", models.ImageCheckUnknown, models.ImageCheckReasonAuthFailed, "", "quay.io"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results: %+v", len(got), got)
	}
	for i, w := range want {
		g := got[i]
		if g.Repository != w.repo || g.Tag != w.tag || g.Status != w.status || g.Reason != w.reason || g.RemoteDigest != w.remote || g.Registry != w.registry {
			t.Errorf("result %d = %+v, want %+v", i, g, w)
		}
	}
	if got[0].LocalDigest != "sha256:"+hexB || got[0].ImageID != hexA {
		t.Errorf("local digest/image id = %q / %q", got[0].LocalDigest, got[0].ImageID)
	}
}

func TestPinnedContainersAlwaysGetAResult(t *testing.T) {
	images := []image.Summary{
		{ID: "sha256:" + hexA, RepoTags: []string{"nginx:latest"}, RepoDigests: []string{"nginx@sha256:" + hexB}},
		{ID: "sha256:" + hexC, RepoDigests: []string{"redis@sha256:" + hexC}},
		{ID: "sha256:" + hexB, RepoTags: []string{"alpine:3.18"}, RepoDigests: []string{"alpine@sha256:" + hexA}},
	}
	pinned := make(pinnedRefs)
	for _, raw := range []string{
		"nginx@sha256:" + hexB,
		"redis:7.2@sha256:" + hexC,
		"alpine:3.18@sha256:" + hexA,
	} {
		pinned.add(parseImageRef(raw))
	}
	res := newFakeResolver(map[registry.Ref]remoteOutcome{
		{Host: registry.DockerHubHost, Repository: "library/nginx", Tag: "latest"}: {digest: "sha256:" + hexC},
		{Host: registry.DockerHubHost, Repository: "library/alpine", Tag: "3.18"}:  {digest: "sha256:" + hexA},
	})
	got := runImageChecks(context.Background(), planImageChecks(images, pinned), res)

	byKey := make(map[string]models.DockerImageCheckResult)
	for _, r := range got {
		byKey[r.Repository+"|"+r.Tag+"|"+r.ImageID] = r
	}
	want := map[string]string{
		"nginx|latest|" + hexA: models.ImageCheckUpdateAvailable,
		"nginx|<none>|" + hexA: models.ImageCheckPinned,
		"redis|<none>|" + hexC: models.ImageCheckPinned,
		"redis|7.2|" + hexC:    models.ImageCheckPinned,
		"alpine|3.18|" + hexB:  models.ImageCheckUpToDate,
	}
	for key, status := range want {
		r, ok := byKey[key]
		if !ok {
			t.Errorf("no result for %s", key)
			continue
		}
		if r.Status != status {
			t.Errorf("%s status = %s, want %s", key, r.Status, status)
		}
	}
	if r := byKey["nginx|<none>|"+hexA]; r.LocalDigest != "sha256:"+hexB {
		t.Errorf("pinned row local digest = %q", r.LocalDigest)
	}
	if len(got) != len(want) {
		t.Errorf("got %d results, want %d: %+v", len(got), len(want), got)
	}
}

func TestImageChecksDedupeConcurrencyAndRateLimit(t *testing.T) {
	var images []image.Summary
	answers := map[registry.Ref]remoteOutcome{}
	hosts := []string{"r1.example.com", "r2.example.com", "r3.example.com", "r4.example.com", "r5.example.com", "r6.example.com"}
	for _, h := range hosts {
		for _, tag := range []string{"a", "b", "c"} {
			images = append(images, image.Summary{
				ID:          "sha256:" + hexA,
				RepoTags:    []string{h + "/app:" + tag},
				RepoDigests: []string{h + "/app@sha256:" + hexA},
			})
			answers[registry.Ref{Host: h, Repository: "app", Tag: tag}] = remoteOutcome{digest: "sha256:" + hexA}
		}
	}
	images = append(images, image.Summary{ID: "sha256:" + hexB, RepoTags: []string{"r1.example.com/app:a"}, RepoDigests: []string{"r1.example.com/app@sha256:" + hexA}})
	answers[registry.Ref{Host: "r2.example.com", Repository: "app", Tag: "a"}] = remoteOutcome{err: registry.ErrRateLimited}

	res := newFakeResolver(answers)
	got := runImageChecks(context.Background(), planImageChecks(images, nil), res)

	if n := res.calls[registry.Ref{Host: "r1.example.com", Repository: "app", Tag: "a"}]; n != 1 {
		t.Errorf("duplicate ref resolved %d times", n)
	}
	if res.maxHost > 1 {
		t.Errorf("max concurrent requests to one registry = %d", res.maxHost)
	}
	if res.maxAll > imageCheckConcurrency {
		t.Errorf("max concurrent requests overall = %d", res.maxAll)
	}
	if n := res.calls[registry.Ref{Host: "r2.example.com", Repository: "app", Tag: "b"}] + res.calls[registry.Ref{Host: "r2.example.com", Repository: "app", Tag: "c"}]; n != 0 {
		t.Errorf("registry was queried %d more times after a 429", n)
	}
	for _, r := range got {
		if r.Registry == "r2.example.com" {
			if r.Status != models.ImageCheckUnknown || r.Reason != models.ImageCheckReasonRateLimited {
				t.Errorf("r2 result = %+v", r)
			}
		} else if r.Status != models.ImageCheckUpToDate {
			t.Errorf("result = %+v", r)
		}
	}
	if len(got) != len(images) {
		t.Errorf("got %d results for %d images", len(got), len(images))
	}
}

func TestInsecureMatcher(t *testing.T) {
	cfg := &registrytypes.ServiceConfig{
		InsecureRegistryCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8"), netip.MustParsePrefix("10.5.0.0/16")},
		IndexConfigs: map[string]*registrytypes.IndexInfo{
			"docker.io":         {Name: "docker.io", Secure: true},
			"harbor.local:8443": {Name: "harbor.local:8443", Secure: false},
			"secure.local":      {Name: "secure.local", Secure: true},
		},
	}
	resolve := func(name string) []netip.Addr {
		switch name {
		case "localhost":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}
		case "build.internal":
			return []netip.Addr{netip.MustParseAddr("10.5.3.4")}
		}
		return nil
	}
	m := insecureMatcher(cfg, resolve)
	tests := map[string]bool{
		"localhost:5000":         true,
		"127.0.0.1:5000":         true,
		"harbor.local:8443":      true,
		"secure.local":           false,
		"build.internal:5000":    true,
		"ghcr.io":                false,
		registry.DockerHubHost:   false,
		"[::ffff:10.5.0.1]:5000": true,
	}
	for host, want := range tests {
		if got := m(host); got != want {
			t.Errorf("insecure(%q) = %v, want %v", host, got, want)
		}
	}
	mixed := insecureMatcher(cfg, func(name string) []netip.Addr {
		switch name {
		case "split.example.com":
			return []netip.Addr{netip.MustParseAddr("10.5.0.9"), netip.MustParseAddr("203.0.113.7")}
		case "split6.example.com":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("2001:db8::1")}
		case "allin.example.com":
			return []netip.Addr{netip.MustParseAddr("10.5.0.9"), netip.MustParseAddr("127.0.0.2")}
		}
		return nil
	})
	for host, want := range map[string]bool{
		"split.example.com:5000":  false,
		"split6.example.com:5000": false,
		"allin.example.com:5000":  true,
		"unresolvable.example":    false,
		"harbor.local:8443":       true,
		"10.5.0.9:5000":           true,
		"203.0.113.7:5000":        false,
	} {
		if got := mixed(host); got != want {
			t.Errorf("insecure(%q) = %v, want %v", host, got, want)
		}
	}
	loopCfg := &registrytypes.ServiceConfig{InsecureRegistryCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}}
	loop := insecureMatcher(loopCfg, func(name string) []netip.Addr {
		switch name {
		case "localhost":
			return []netip.Addr{netip.IPv6Loopback(), netip.MustParseAddr("127.0.0.1")}
		case "mixed.example.com":
			return []netip.Addr{netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("203.0.113.5")}
		case "v6loop.example.com":
			return []netip.Addr{netip.IPv6Loopback()}
		}
		return nil
	})
	for host, want := range map[string]bool{
		"localhost:5000":          true,
		"localhost:5001":          true,
		"mixed.example.com:5000":  false,
		"v6loop.example.com:5000": true,
		"[::1]:5000":              true,
	} {
		if got := loop(host); got != want {
			t.Errorf("loopback insecure(%q) = %v, want %v", host, got, want)
		}
	}
	if !insecureMatcher(loopCfg, func(string) []netip.Addr { return nil })("localhost:5000") {
		t.Error("localhost must count as loopback even when it does not resolve")
	}
	noLoop := &registrytypes.ServiceConfig{InsecureRegistryCIDRs: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	if insecureMatcher(noLoop, func(string) []netip.Addr { return nil })("localhost:5000") {
		t.Error("localhost must not be insecure when no CIDR covers loopback")
	}
	if insecureMatcher(nil, resolve)("localhost:5000") {
		t.Error("nil config must never mark a registry insecure")
	}
}

func TestDebouncerCoalesces(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	d := NewDebouncer(30*time.Millisecond, func() {
		if calls.Add(1) == 1 {
			<-release
		}
	})
	defer d.Stop()

	for range 10 {
		d.Trigger()
	}
	waitFor(t, func() bool { return calls.Load() == 1 })

	for range 5 {
		d.Trigger()
	}
	time.Sleep(60 * time.Millisecond)
	if n := calls.Load(); n != 1 {
		t.Fatalf("ran %d times while the first call was still running", n)
	}
	close(release)
	waitFor(t, func() bool { return calls.Load() == 2 })
	time.Sleep(80 * time.Millisecond)
	if n := calls.Load(); n != 2 {
		t.Errorf("burst during a run should cause exactly one follow-up, got %d calls", n)
	}

	d.Stop()
	d.Trigger()
	time.Sleep(60 * time.Millisecond)
	if n := calls.Load(); n != 2 {
		t.Errorf("stopped debouncer ran again, %d calls", n)
	}
}

func TestSingleFlight(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	s := NewSingleFlight(func(context.Context) {
		calls.Add(1)
		close(started)
		<-release
	})

	done := make(chan bool)
	go func() { done <- s.Run(context.Background()) }()
	<-started
	if s.Run(context.Background()) {
		t.Error("second concurrent run should be refused")
	}
	close(release)
	if !<-done {
		t.Error("first run should report it ran")
	}
	if calls.Load() != 1 {
		t.Errorf("fn ran %d times", calls.Load())
	}
	if last := s.LastFinished(); last.IsZero() || time.Since(last) > time.Minute {
		t.Errorf("LastFinished = %v", last)
	}
}

func TestSingleFlightLastFinishedZeroBeforeRun(t *testing.T) {
	if !NewSingleFlight(func(context.Context) {}).LastFinished().IsZero() {
		t.Error("LastFinished should be zero before any run")
	}
}

func TestSingleFlightRunEvery(t *testing.T) {
	var calls atomic.Int32
	s := NewSingleFlight(func(context.Context) { calls.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.RunEvery(ctx, 5*time.Millisecond, func() time.Duration { return 10 * time.Millisecond })
		close(done)
	}()
	waitFor(t, func() bool { return calls.Load() >= 3 })
	cancel()
	<-done
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(2 * time.Millisecond)
	}
}
