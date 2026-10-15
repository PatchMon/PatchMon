package packages

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"patchmon-agent/internal/repositories"

	"github.com/sirupsen/logrus"
)

const (
	apkSecDBBaseURL  = "https://secdb.alpinelinux.org"
	apkSecDBTTL      = 6 * time.Hour
	apkSecDBTimeout  = 20 * time.Second
	apkSecDBMaxBytes = 8 << 20
	apkSecDBBackoff  = time.Hour
)

var (
	apkSecDBCacheDir = "/var/cache/patchmon/apk-secdb"

	apkSecDBMu       sync.Mutex
	apkSecDBLastWarn time.Time
	apkSecDBFailedAt = map[string]time.Time{}
)

// secdb is keyed by origin (source package) and lists the version each fix landed in.
type apkSecDB struct {
	logger   *logrus.Logger
	client   *http.Client
	baseURL  string
	cacheDir string
	// cacheOwner is the uid the cache directory must belong to.
	cacheOwner int
	now        func() time.Time
}

func newAPKSecDB(logger *logrus.Logger) *apkSecDB {
	return &apkSecDB{
		logger: logger,
		client: &http.Client{
			Timeout:       apkSecDBTimeout,
			CheckRedirect: apkSecDBCheckRedirect,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		baseURL:    apkSecDBBaseURL,
		cacheDir:   apkSecDBCacheDir,
		cacheOwner: 0,
		now:        time.Now,
	}
}

func apkSecDBCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	if req.URL.Scheme != "https" || req.URL.Host != via[0].URL.Host {
		return fmt.Errorf("refusing redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}
	return nil
}

type apkSecDBFile struct {
	Packages []struct {
		Pkg struct {
			Name     string                     `json:"name"`
			Secfixes map[string]json.RawMessage `json:"secfixes"`
		} `json:"pkg"`
	} `json:"packages"`
}

// apkSecDBSources picks the branch from the untagged repositories, since a host part
// way through a release upgrade already points at the next branch.
func apkSecDBSources(lines []repositories.APKRepoLine) (branch string, repos []string) {
	for _, l := range lines {
		if l.Tag != "" {
			continue
		}
		b, comp := repositories.APKBranchAndComponent(l.URL)
		if b == "" || b == "latest-stable" {
			continue
		}
		if branch == "" {
			branch = b
		}
		if b == branch && (comp == "main" || comp == "community") && !slices.Contains(repos, comp) {
			repos = append(repos, comp)
		}
	}
	return branch, repos
}

// A source that cannot be fetched falls back to its last cached copy, however old.
func (s *apkSecDB) fixes(ctx context.Context, branch string, repos []string) map[string][]string {
	out := map[string][]string{}
	for _, repo := range repos {
		data, err := s.load(ctx, branch, repo)
		if err != nil {
			entry := s.logger.WithError(err).WithField("repo", branch+"/"+repo)
			if s.shouldWarn() {
				entry.Warn("Alpine secdb unavailable, security updates in it are not flagged")
			} else {
				entry.Debug("Alpine secdb unavailable")
			}
			continue
		}
		var f apkSecDBFile
		if err := json.Unmarshal(data, &f); err != nil {
			s.logger.WithError(err).WithField("repo", branch+"/"+repo).Warn("Alpine secdb could not be parsed")
			continue
		}
		for _, p := range f.Packages {
			for ver := range p.Pkg.Secfixes {
				if ver != "0" && apkVersionValid(ver) {
					out[p.Pkg.Name] = append(out[p.Pkg.Name], ver)
				}
			}
		}
	}
	return out
}

// shouldWarn limits the unreachable warning to once per TTL; reports run far more often.
func (s *apkSecDB) shouldWarn() bool {
	apkSecDBMu.Lock()
	defer apkSecDBMu.Unlock()
	if !apkSecDBLastWarn.IsZero() && s.now().Sub(apkSecDBLastWarn) < apkSecDBTTL {
		return false
	}
	apkSecDBLastWarn = s.now()
	return true
}

func (s *apkSecDB) load(ctx context.Context, branch, repo string) ([]byte, error) {
	cacheOK := s.cacheDirTrusted()
	cache := filepath.Join(s.cacheDir, branch+"-"+repo+".json")
	if cacheOK {
		if data, modTime, err := readCacheFile(cache); err == nil && s.now().Sub(modTime) < apkSecDBTTL {
			return data, nil
		}
	}
	url := fmt.Sprintf("%s/%s/%s.json", s.baseURL, branch, repo)
	data, fetchErr := s.fetchWithBackoff(ctx, url)
	if fetchErr == nil {
		if cacheOK {
			if err := writeFileAtomic(cache, data); err != nil {
				s.logger.WithError(err).Debug("Could not cache Alpine secdb")
			}
		}
		return data, nil
	}
	if cacheOK {
		if data, _, err := readCacheFile(cache); err == nil {
			s.logger.WithError(fetchErr).Debug("Using stale Alpine secdb cache")
			return data, nil
		}
	}
	return nil, fetchErr
}

// cacheDirTrusted refuses a cache another user could have planted or swapped.
func (s *apkSecDB) cacheDirTrusted() bool {
	if err := os.MkdirAll(s.cacheDir, 0o755); err != nil {
		return false
	}
	info, err := os.Lstat(s.cacheDir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return false
	}
	owner, ok := fileOwner(info)
	if !ok || owner != s.cacheOwner {
		s.logger.WithField("dir", s.cacheDir).Debug("Alpine secdb cache directory not trusted, not caching")
		return false
	}
	return true
}

func readCacheFile(path string) ([]byte, time.Time, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !info.Mode().IsRegular() {
		return nil, time.Time{}, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, apkSecDBMaxBytes+1))
	if err != nil {
		return nil, time.Time{}, err
	}
	if len(data) > apkSecDBMaxBytes {
		return nil, time.Time{}, fmt.Errorf("%s is larger than %d bytes", path, apkSecDBMaxBytes)
	}
	return data, info.ModTime(), nil
}

// fetchWithBackoff skips the request for an hour after a failure, so an unreachable
// secdb does not add two timeouts to every report.
func (s *apkSecDB) fetchWithBackoff(ctx context.Context, url string) ([]byte, error) {
	apkSecDBMu.Lock()
	failed, ok := apkSecDBFailedAt[url]
	apkSecDBMu.Unlock()
	if ok && s.now().Sub(failed) < apkSecDBBackoff {
		return nil, fmt.Errorf("GET %s failed at %s, next attempt after %s", url, failed.UTC().Format(time.RFC3339), failed.Add(apkSecDBBackoff).UTC().Format(time.RFC3339))
	}
	data, err := s.fetch(ctx, url)
	if err != nil && ctx.Err() != nil {
		return nil, err
	}
	apkSecDBMu.Lock()
	if err != nil {
		apkSecDBFailedAt[url] = s.now()
	} else {
		delete(apkSecDBFailedAt, url)
	}
	apkSecDBMu.Unlock()
	return data, err
}

func (s *apkSecDB) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "patchmon-agent")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, apkSecDBMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > apkSecDBMaxBytes {
		return nil, fmt.Errorf("GET %s: response larger than %d bytes", url, apkSecDBMaxBytes)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("GET %s: response is not JSON", url)
	}
	return data, nil
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".secdb-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func apkFixesSecurity(fixVersions []string, installed, available string, cmp func(a, b string) int) bool {
	if !apkVersionValid(installed) || !apkVersionValid(available) {
		return false
	}
	for _, fix := range fixVersions {
		if cmp(installed, fix) < 0 && cmp(fix, available) <= 0 {
			return true
		}
	}
	return false
}
