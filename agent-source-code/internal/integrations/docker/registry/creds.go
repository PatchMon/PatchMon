package registry

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	dockerHubConfigKey = "https://index.docker.io/v1/"
	helperTimeout      = 10 * time.Second
	maxConfigSize      = 4 << 20
	maxHelperOutput    = 64 << 10
)

var helperNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Credential holds registry secrets. It never formats its contents.
type Credential struct {
	Username      string
	Password      string
	IdentityToken string
}

// String keeps secrets out of %v and %s.
func (Credential) String() string { return "[redacted]" }

// GoString keeps secrets out of %#v.
func (Credential) GoString() string { return "[redacted]" }

type authEntry struct {
	Auth          string `json:"auth"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	IdentityToken string `json:"identitytoken"`
}

// DockerConfig is the credential part of a Docker CLI config.json.
type DockerConfig struct {
	auths       map[string]authEntry
	credHelpers map[string]string
	credsStore  string

	mu    sync.Mutex
	cache map[string]cachedCred
}

type cachedCred struct {
	cred Credential
	ok   bool
}

// DefaultDockerConfigPath returns $DOCKER_CONFIG/config.json, else the default for the agent user.
func DefaultDockerConfigPath() string {
	if dir := os.Getenv("DOCKER_CONFIG"); dir != "" {
		return filepath.Join(dir, "config.json")
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, ".docker", "config.json")
		}
	}
	return "/root/.docker/config.json"
}

// LoadDockerConfig reads path. A missing file yields an empty config.
func LoadDockerConfig(path string) (*DockerConfig, error) {
	cfg := &DockerConfig{auths: map[string]authEntry{}, credHelpers: map[string]string{}}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("open docker config: %w", err)
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(io.LimitReader(f, maxConfigSize))
	if err != nil {
		return cfg, fmt.Errorf("read docker config: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return cfg, nil
	}
	var file struct {
		Auths       map[string]authEntry `json:"auths"`
		CredHelpers map[string]string    `json:"credHelpers"`
		CredsStore  string               `json:"credsStore"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return cfg, errors.New("parse docker config: invalid JSON")
	}
	for k, v := range file.Auths {
		cfg.auths[normaliseConfigKey(k)] = v
	}
	for k, v := range file.CredHelpers {
		cfg.credHelpers[normaliseConfigKey(k)] = v
	}
	cfg.credsStore = file.CredsStore
	return cfg, nil
}

func normaliseConfigKey(k string) string {
	k = strings.TrimSpace(k)
	if k == dockerHubConfigKey {
		return DockerHubHost
	}
	k = strings.TrimPrefix(k, "https://")
	k = strings.TrimPrefix(k, "http://")
	k, _, _ = strings.Cut(k, "/")
	switch k {
	case "docker.io", "index.docker.io":
		return DockerHubHost
	}
	return k
}

// Lookup returns the credential for a registry API host.
func (c *DockerConfig) Lookup(ctx context.Context, host string) (Credential, bool) {
	if c == nil {
		return Credential{}, false
	}
	key := normaliseConfigKey(host)
	c.mu.Lock()
	defer c.mu.Unlock()
	if hit, ok := c.cache[key]; ok {
		return hit.cred, hit.ok
	}
	cred, ok := c.lookup(ctx, key)
	if c.cache == nil {
		c.cache = make(map[string]cachedCred)
	}
	c.cache[key] = cachedCred{cred: cred, ok: ok}
	return cred, ok
}

func (c *DockerConfig) lookup(ctx context.Context, key string) (Credential, bool) {
	serverURL := key
	if key == DockerHubHost {
		serverURL = dockerHubConfigKey
	}
	if helper, ok := c.credHelpers[key]; ok {
		return runHelper(ctx, helper, serverURL)
	}
	if e, ok := c.auths[key]; ok {
		if cred, ok := e.credential(); ok {
			return cred, true
		}
	}
	if c.credsStore != "" {
		return runHelper(ctx, c.credsStore, serverURL)
	}
	return Credential{}, false
}

func (e authEntry) credential() (Credential, bool) {
	cred := Credential{Username: e.Username, Password: e.Password, IdentityToken: e.IdentityToken}
	if e.Auth != "" {
		decoded, err := base64.StdEncoding.DecodeString(e.Auth)
		if err != nil {
			return Credential{}, false
		}
		user, pass, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return Credential{}, false
		}
		cred.Username, cred.Password = user, pass
	}
	if cred.IdentityToken == "" && (cred.Username == "" || cred.Password == "") {
		return Credential{}, false
	}
	return cred, true
}

func runHelper(ctx context.Context, name, serverURL string) (Credential, bool) {
	if !helperNameRe.MatchString(name) {
		return Credential{}, false
	}
	hctx, cancel := context.WithTimeout(ctx, helperTimeout)
	defer cancel()
	cmd := exec.CommandContext(hctx, "docker-credential-"+name, "get")
	cmd.Stdin = strings.NewReader(serverURL)
	var stdout bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxHelperOutput}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return Credential{}, false
	}
	var out struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.Secret == "" {
		return Credential{}, false
	}
	if out.Username == "<token>" {
		return Credential{IdentityToken: out.Secret}, true
	}
	return Credential{Username: out.Username, Password: out.Secret}, true
}

type limitedWriter struct {
	w io.Writer
	n int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	chunk := p
	if int64(len(chunk)) > l.n {
		chunk = chunk[:l.n]
	}
	n, err := l.w.Write(chunk)
	l.n -= int64(n)
	if err != nil {
		return n, err
	}
	return len(p), nil
}
