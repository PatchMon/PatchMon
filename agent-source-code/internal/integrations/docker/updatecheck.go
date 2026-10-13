package docker

import (
	"context"
	"errors"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"patchmon-agent/internal/integrations/docker/registry"
	"patchmon-agent/internal/logutil"
	"patchmon-agent/pkg/models"

	"github.com/moby/moby/api/types/image"
	registrytypes "github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	"github.com/sirupsen/logrus"
)

const (
	imageCheckConcurrency = 4
	resolveTimeout        = 2 * time.Second
)

// ErrDockerUnavailable is returned when the Docker daemon cannot be reached.
var ErrDockerUnavailable = errors.New("docker is not available")

type manifestResolver interface {
	ManifestDigest(ctx context.Context, ref registry.Ref) (string, error)
}

type checkTarget struct {
	result      models.DockerImageCheckResult
	ref         registry.Ref
	localDigest []string
	needsRemote bool
}

// CheckImageUpdates compares every local image's digest with what its registry tag points at now.
func (d *Integration) CheckImageUpdates(ctx context.Context, agentVersion string) (*models.DockerImageCheckPayload, error) {
	if d.client == nil && !d.IsAvailable() {
		return nil, ErrDockerUnavailable
	}
	images, err := d.listImages(ctx)
	if err != nil {
		return nil, err
	}

	var regCfg *registrytypes.ServiceConfig
	if info, err := d.client.Info(ctx, client.InfoOptions{}); err != nil {
		d.logger.WithError(err).Debug("Could not read Docker daemon registry settings")
	} else {
		regCfg = info.Info.RegistryConfig
	}

	creds, err := registry.LoadDockerConfig(registry.DefaultDockerConfigPath())
	if err != nil {
		d.logger.WithError(err).Warn("Could not read Docker registry credentials, checking anonymously")
	}

	resolver := &registry.Client{
		Credentials: creds.Lookup,
		Insecure:    insecureMatcher(regCfg, lookupHostAddrs),
		UserAgent:   "patchmon-agent/" + agentVersion,
	}

	results := runImageChecks(ctx, planImageChecks(images, d.pinnedDigests(ctx)), resolver)
	resolver.CloseIdleConnections()
	if len(results) > models.MaxImageCheckResults {
		d.logger.WithField("results", len(results)).Warn("Too many Docker images to report, truncating update check results")
		results = results[:models.MaxImageCheckResults]
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Status]++
		if r.Reason != "" {
			d.logger.WithFields(logrus.Fields{
				"repository": logutil.Sanitize(r.Repository),
				"tag":        logutil.Sanitize(r.Tag),
				"reason":     r.Reason,
			}).Debug("Docker image update check inconclusive")
		}
	}
	d.logger.WithFields(logrus.Fields{
		"images":           len(results),
		"update_available": counts[models.ImageCheckUpdateAvailable],
		"up_to_date":       counts[models.ImageCheckUpToDate],
		"unknown":          counts[models.ImageCheckUnknown],
	}).Info("Docker image update check finished")

	return &models.DockerImageCheckPayload{
		CheckedAt:    time.Now().UTC().Truncate(time.Second),
		AgentVersion: agentVersion,
		Results:      results,
	}, nil
}

type pinnedRefs map[string]map[string]bool

func (p pinnedRefs) add(ref imageRef) {
	key := ref.Repository + "@" + ref.Digest
	if p[key] == nil {
		p[key] = make(map[string]bool)
	}
	p[key][ref.Tag] = true
}

func (p pinnedRefs) has(repository string, digests []string) bool {
	for _, dg := range digests {
		if len(p[repository+"@"+dg]) > 0 {
			return true
		}
	}
	return false
}

func (d *Integration) pinnedDigests(ctx context.Context) pinnedRefs {
	pinned := make(pinnedRefs)
	list, err := d.client.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		d.logger.WithError(err).Debug("Could not list containers for digest-pinned references")
		return pinned
	}
	for _, c := range list.Items {
		ictx, cancel := context.WithTimeout(ctx, containerInspectTimeout)
		res, err := d.client.ContainerInspect(ictx, c.ID, client.ContainerInspectOptions{})
		cancel()
		raw := c.Image
		if err == nil && res.Container.Config != nil && res.Container.Config.Image != "" {
			raw = res.Container.Config.Image
		}
		if ref := parseImageRef(raw); ref.isNamed() && ref.Digest != "" {
			pinned.add(ref)
		}
	}
	return pinned
}

func planImageChecks(images []image.Summary, pinned pinnedRefs) []checkTarget {
	var targets []checkTarget
	for _, img := range images {
		emitted := make(map[string]bool)
		for _, row := range imageRows(img) {
			emitted[repoTagKey(row.Repository, row.Tag)] = true
			t := checkTarget{
				result: models.DockerImageCheckResult{
					Repository: row.Repository,
					Tag:        row.Tag,
					ImageID:    row.ImageID,
				},
				localDigest: row.RepoDigests,
			}
			if len(row.RepoDigests) > 0 {
				t.result.LocalDigest = row.RepoDigests[0]
			}
			switch {
			case len(img.RepoDigests) == 0:
				t.result.Status = models.ImageCheckLocalBuild
			case row.Tag == noneTag && pinned.has(row.Repository, row.RepoDigests):
				t.result.Status = models.ImageCheckPinned
			case row.Tag == noneTag, row.Tag == "":
				t.result.Status = models.ImageCheckSkipped
			case len(row.RepoDigests) == 0:
				t.result.Status = models.ImageCheckUnknown
				t.result.Reason = models.ImageCheckReasonNoDigest
			default:
				t.ref = registry.ParseRef(row.Repository, row.Tag)
				t.result.Registry = t.ref.Host
				t.needsRemote = true
			}
			targets = append(targets, t)
		}
		targets = append(targets, pinnedOnlyTargets(img, pinned, emitted)...)
	}
	return targets
}

func pinnedOnlyTargets(img image.Summary, pinned pinnedRefs, emitted map[string]bool) []checkTarget {
	var out []checkTarget
	for _, rd := range img.RepoDigests {
		ref := parseImageRef(rd)
		if !ref.isNamed() || ref.Digest == "" {
			continue
		}
		tags := slices.Sorted(maps.Keys(pinned[ref.Repository+"@"+ref.Digest]))
		for _, tag := range tags {
			key := repoTagKey(ref.Repository, tag)
			if emitted[key] {
				continue
			}
			emitted[key] = true
			out = append(out, checkTarget{result: models.DockerImageCheckResult{
				Repository:  ref.Repository,
				Tag:         tag,
				ImageID:     trimSHA(img.ID),
				Status:      models.ImageCheckPinned,
				LocalDigest: ref.Digest,
			}})
		}
	}
	return out
}

type remoteOutcome struct {
	digest string
	err    error
}

func runImageChecks(ctx context.Context, targets []checkTarget, resolver manifestResolver) []models.DockerImageCheckResult {
	byHost := make(map[string][]registry.Ref)
	seen := make(map[registry.Ref]bool)
	var hosts []string
	for _, t := range targets {
		if !t.needsRemote || seen[t.ref] {
			continue
		}
		seen[t.ref] = true
		if _, ok := byHost[t.ref.Host]; !ok {
			hosts = append(hosts, t.ref.Host)
		}
		byHost[t.ref.Host] = append(byHost[t.ref.Host], t.ref)
	}

	var mu sync.Mutex
	outcomes := make(map[registry.Ref]remoteOutcome, len(seen))
	hostCh := make(chan string)
	var wg sync.WaitGroup
	for range min(imageCheckConcurrency, len(hosts)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range hostCh {
				rateLimited := false
				for _, ref := range byHost[host] {
					var out remoteOutcome
					switch {
					case rateLimited:
						out.err = registry.ErrRateLimited
					case ctx.Err() != nil:
						out.err = ctx.Err()
					default:
						out.digest, out.err = resolver.ManifestDigest(ctx, ref)
						rateLimited = errors.Is(out.err, registry.ErrRateLimited)
					}
					mu.Lock()
					outcomes[ref] = out
					mu.Unlock()
				}
			}
		}()
	}
	for _, h := range hosts {
		hostCh <- h
	}
	close(hostCh)
	wg.Wait()

	results := make([]models.DockerImageCheckResult, 0, len(targets))
	for _, t := range targets {
		r := t.result
		if t.needsRemote {
			out := outcomes[t.ref]
			switch {
			case out.err != nil:
				r.Status = models.ImageCheckUnknown
				r.Reason = reasonFor(out.err)
			case slices.Contains(t.localDigest, out.digest):
				r.Status = models.ImageCheckUpToDate
				r.RemoteDigest = out.digest
			default:
				r.Status = models.ImageCheckUpdateAvailable
				r.RemoteDigest = out.digest
			}
		}
		results = append(results, r)
	}
	return results
}

func reasonFor(err error) string {
	switch {
	case errors.Is(err, registry.ErrRateLimited):
		return models.ImageCheckReasonRateLimited
	case errors.Is(err, registry.ErrAuthFailed):
		return models.ImageCheckReasonAuthFailed
	case errors.Is(err, registry.ErrNotFound):
		return models.ImageCheckReasonTagNotFound
	case errors.Is(err, registry.ErrUnsupportedAuth):
		return models.ImageCheckReasonUnsupportedAuth
	case errors.Is(err, registry.ErrInsecureDisallowed):
		return models.ImageCheckReasonInsecureDisallowed
	case errors.Is(err, registry.ErrNoDigest):
		return models.ImageCheckReasonNoDigest
	default:
		return models.ImageCheckReasonUnreachable
	}
}

func insecureMatcher(cfg *registrytypes.ServiceConfig, resolve func(string) []netip.Addr) func(string) bool {
	if cfg == nil {
		return func(string) bool { return false }
	}
	return func(host string) bool {
		if host == registry.DockerHubHost {
			return false
		}
		if idx, ok := cfg.IndexConfigs[host]; ok && idx != nil {
			return !idx.Secure
		}
		if len(cfg.InsecureRegistryCIDRs) == 0 {
			return false
		}
		name := host
		if h, _, err := net.SplitHostPort(host); err == nil {
			name = h
		}
		loopbackInsecure := slices.ContainsFunc(cfg.InsecureRegistryCIDRs, func(p netip.Prefix) bool {
			return p.Contains(netip.MustParseAddr("127.0.0.1")) || p.Contains(netip.IPv6Loopback())
		})
		var addrs []netip.Addr
		if a, err := netip.ParseAddr(name); err == nil {
			addrs = []netip.Addr{a}
		} else if resolve != nil {
			addrs = resolve(name)
		}
		if strings.EqualFold(name, "localhost") && len(addrs) == 0 {
			return loopbackInsecure
		}
		if len(addrs) == 0 {
			return false
		}
		for _, a := range addrs {
			a = a.Unmap()
			if a.IsLoopback() && loopbackInsecure {
				continue
			}
			if !slices.ContainsFunc(cfg.InsecureRegistryCIDRs, func(p netip.Prefix) bool { return p.Contains(a) }) {
				return false
			}
		}
		return true
	}
}

func lookupHostAddrs(name string) []netip.Addr {
	ctx, cancel := context.WithTimeout(context.Background(), resolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", name)
	if err != nil {
		return nil
	}
	return addrs
}
