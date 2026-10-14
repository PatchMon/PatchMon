package packages

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"time"

	"patchmon-agent/internal/repositories"

	"github.com/sirupsen/logrus"
)

const (
	apkSecDBBaseURL  = "https://secdb.alpinelinux.org"
	apkSecDBTTL      = 6 * time.Hour
	apkSecDBTimeout  = 20 * time.Second
	apkSecDBMaxBytes = 32 << 20
)

var apkSecDBCacheDir = "/var/cache/patchmon/apk-secdb"

// secdb is keyed by origin (source package) and lists the version each fix landed in.
type apkSecDB struct {
	logger   *logrus.Logger
	client   *http.Client
	baseURL  string
	cacheDir string
	now      func() time.Time
}

func newAPKSecDB(logger *logrus.Logger) *apkSecDB {
	return &apkSecDB{
		logger: logger,
		client: &http.Client{
			Timeout: apkSecDBTimeout,
			Transport: &http.Transport{
				Proxy:               http.ProxyFromEnvironment,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		baseURL:  apkSecDBBaseURL,
		cacheDir: apkSecDBCacheDir,
		now:      time.Now,
	}
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
			s.logger.WithError(err).WithField("repo", branch+"/"+repo).Warn("Alpine secdb unavailable, security updates in it are not flagged")
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

func (s *apkSecDB) load(ctx context.Context, branch, repo string) ([]byte, error) {
	cache := filepath.Join(s.cacheDir, branch+"-"+repo+".json")
	if info, err := os.Stat(cache); err == nil && s.now().Sub(info.ModTime()) < apkSecDBTTL {
		if data, err := os.ReadFile(cache); err == nil {
			return data, nil
		}
	}
	data, fetchErr := s.fetch(ctx, fmt.Sprintf("%s/%s/%s.json", s.baseURL, branch, repo))
	if fetchErr == nil {
		if err := writeFileAtomic(cache, data); err != nil {
			s.logger.WithError(err).Debug("Could not cache Alpine secdb")
		}
		return data, nil
	}
	if data, err := os.ReadFile(cache); err == nil {
		s.logger.WithError(fetchErr).Debug("Using stale Alpine secdb cache")
		return data, nil
	}
	return nil, fetchErr
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
