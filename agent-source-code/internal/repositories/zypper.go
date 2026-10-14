package repositories

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"patchmon-agent/internal/constants"
	"patchmon-agent/pkg/models"

	"github.com/sirupsen/logrus"
)

// ZypperManager handles zypper repository information collection.
type ZypperManager struct {
	logger *logrus.Logger
}

// NewZypperManager creates a new zypper repository manager.
func NewZypperManager(logger *logrus.Logger) *ZypperManager {
	return &ZypperManager{logger: logger}
}

type zypperRepoList struct {
	Repos []struct {
		Alias   string `xml:"alias,attr"`
		Name    string `xml:"name,attr"`
		Enabled string `xml:"enabled,attr"`
		URL     string `xml:"url"`
	} `xml:"repo-list>repo"`
}

// GetRepositories lists the enabled zypper repositories with variables already expanded.
func (z *ZypperManager) GetRepositories() ([]models.Repository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "zypper", "--non-interactive", "--no-refresh", "--xmlout", "repos")
	cmd.Env = append(os.Environ(), "LC_MESSAGES=C", "ZYPP_LOCK_TIMEOUT=30")
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("zypper repos: %w", err)
	}
	return parseZypperRepos(out)
}

func parseZypperRepos(out []byte) ([]models.Repository, error) {
	start := bytes.Index(out, []byte("<stream"))
	if start < 0 {
		return nil, fmt.Errorf("zypper repos printed no XML stream")
	}
	var list zypperRepoList
	if err := xml.Unmarshal(out[start:], &list); err != nil {
		return nil, fmt.Errorf("parsing zypper repos XML: %w", err)
	}
	repos := make([]models.Repository, 0, len(list.Repos))
	for _, r := range list.Repos {
		if r.Enabled != "1" {
			continue
		}
		u := publicRepoURL(strings.TrimSpace(r.URL))
		if u == "" {
			continue
		}
		name := r.Name
		if name == "" {
			name = r.Alias
		}
		repos = append(repos, models.Repository{
			Name:         r.Alias,
			URL:          u,
			Distribution: name,
			RepoType:     constants.RepoTypeRPM,
			IsEnabled:    true,
			IsSecure:     strings.HasPrefix(u, "https://"),
		})
	}
	return repos, nil
}

// publicRepoURL drops credentials and the query string, which on SUSE Customer Center
// repositories carries the registration token.
func publicRepoURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ftp") {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
