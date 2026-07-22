package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

const userAgent = "patchmon-cli/0.1"

type config struct {
	Server      string `json:"server"`
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	Username    string `json:"username,omitempty"`
	Insecure    bool   `json:"insecure,omitempty"`
}
type loginResponse struct {
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
	ExpiresAt   string `json:"expires_at"`
	RequiresTFA bool   `json:"requiresTfa"`
	Username    string `json:"username"`
	User        struct {
		Username string `json:"username"`
	} `json:"user"`
}
type host struct {
	ID              string       `json:"id"`
	FriendlyName    string       `json:"friendly_name"`
	Hostname        *string      `json:"hostname"`
	IP              *string      `json:"ip"`
	OSType          string       `json:"os_type"`
	OSVersion       string       `json:"os_version"`
	Status          string       `json:"status"`
	EffectiveStatus string       `json:"effectiveStatus"`
	LastUpdate      string       `json:"last_update"`
	APIID           string       `json:"api_id"`
	Groups          []hostGroup  `json:"groups,omitempty"`
	AllowedAccounts []sshAccount `json:"allowed_accounts,omitempty"`
}
type hostGroup struct {
	Name string `json:"name"`
}
type sshAccount struct {
	LinuxUsername string `json:"linux_username"`
	AllowSudo     bool   `json:"allow_sudo"`
}
type apiClient struct {
	cfg  config
	http *http.Client
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "patchmon:", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "login", "init":
		return runLogin(args[1:])
	case "logout":
		return runLogout()
	case "instances", "hosts":
		return runInstances(args[1:])
	case "ssh":
		return runSSH(args[1:])
	case "tunnel":
		return runTunnel(args[1:])
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, "PatchMon CLI\n\nUsage:\n  patchmon login --server https://patchmon.example.com\n  patchmon instances list [--output table|json]\n  patchmon ssh user@instance\n  patchmon tunnel instance 22\n  patchmon logout")
}
func configPath() (string, error) {
	d, err := os.UserConfigDir()
	return filepath.Join(d, "patchmon", "config.json"), err
}
func loadConfig() (config, error) {
	p, err := configPath()
	if err != nil {
		return config{}, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return config{}, errors.New("not logged in; run 'patchmon login --server URL'")
	}
	if err != nil {
		return config{}, err
	}
	var cfg config
	if json.Unmarshal(b, &cfg) != nil || cfg.Server == "" || cfg.AccessToken == "" {
		return config{}, errors.New("invalid configuration; run 'patchmon login' again")
	}
	return cfg, nil
}
func saveConfig(cfg config) error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".config-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
func runLogout() error {
	p, err := configPath()
	if err == nil {
		err = os.Remove(p)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("Logged out")
	return nil
}
func runLogin(args []string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	server := fs.String("server", "", "PatchMon server URL")
	username := fs.String("username", "", "PatchMon username or email")
	insecure := fs.Bool("insecure", false, "skip TLS verification")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *server == "" {
		if old, err := loadConfig(); err == nil {
			*server = old.Server
		}
	}
	if *server == "" {
		return errors.New("--server is required")
	}
	normalized, err := normalizeServer(*server, *insecure)
	if err != nil {
		return err
	}
	if *username == "" {
		fmt.Print("Username or email: ")
		if _, err := fmt.Scanln(username); err != nil {
			return errors.New("username is required")
		}
	}
	password, err := readSecret("Password: ")
	if err != nil {
		return err
	}
	client := newAPIClient(config{Server: normalized, Insecure: *insecure})
	var out loginResponse
	err = client.post(context.Background(), "/api/v1/auth/login", map[string]string{"username": *username, "password": password}, &out, false)
	password = ""
	if err != nil {
		return err
	}
	if out.RequiresTFA {
		code, err := readSecret("Authentication code: ")
		if err != nil {
			return err
		}
		err = client.post(context.Background(), "/api/v1/auth/verify-tfa", map[string]interface{}{"username": out.Username, "token": strings.TrimSpace(code), "remember_me": false}, &out, false)
		code = ""
		if err != nil {
			return err
		}
	}
	token := out.AccessToken
	if token == "" {
		token = out.Token
	}
	if token == "" {
		return errors.New("server did not return an access token")
	}
	name := out.User.Username
	if name == "" {
		name = *username
	}
	if err := saveConfig(config{Server: normalized, AccessToken: token, ExpiresAt: out.ExpiresAt, Username: name, Insecure: *insecure}); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", normalized, name)
	return nil
}
func normalizeServer(raw string, insecure bool) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("invalid server URL")
	}
	if u.Scheme != "https" && !(insecure && u.Scheme == "http") {
		return "", errors.New("server must use HTTPS (or --insecure for HTTP development instances)")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("server URL must not contain a query or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
func newAPIClient(cfg config) *apiClient {
	t := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.Insecure {
		t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &apiClient{cfg: cfg, http: &http.Client{Transport: t, Timeout: 30 * time.Second}}
}
func (c *apiClient) request(ctx context.Context, method, path string, body, out interface{}, auth bool) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.Server+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("contact server: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&e)
		msg := e.Error
		if msg == "" {
			msg = e.Message
		}
		if msg == "" {
			msg = resp.Status
		}
		if resp.StatusCode == http.StatusUnauthorized {
			msg += "; run 'patchmon login' again"
		}
		return errors.New(msg)
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<20)).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
func (c *apiClient) post(ctx context.Context, path string, body, out interface{}, auth bool) error {
	return c.request(ctx, http.MethodPost, path, body, out, auth)
}
func (c *apiClient) hosts(ctx context.Context) ([]host, error) {
	var out []host
	err := c.request(ctx, http.MethodGet, "/api/v1/dashboard/hosts", nil, &out, true)
	return out, err
}
func (c *apiClient) sshAccounts(ctx context.Context, hostID string) ([]sshAccount, error) {
	var out []sshAccount
	err := c.request(ctx, http.MethodGet, "/api/v1/ssh/accounts/"+url.PathEscape(hostID), nil, &out, true)
	return out, err
}
func runInstances(args []string) error {
	if len(args) > 0 && args[0] == "list" {
		args = args[1:]
	}
	fs := flag.NewFlagSet("instances list", flag.ContinueOnError)
	output := fs.String("output", "table", "table or json")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	client := newAPIClient(cfg)
	hosts, err := client.hosts(context.Background())
	if err != nil {
		return err
	}
	for i := range hosts {
		accounts, accountErr := client.sshAccounts(context.Background(), hosts[i].ID)
		if accountErr == nil {
			hosts[i].AllowedAccounts = accounts
		}
	}
	sort.Slice(hosts, func(i, j int) bool { return displayName(hosts[i]) < displayName(hosts[j]) })
	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(hosts)
	}
	if *output != "table" {
		return errors.New("--output must be table or json")
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tHOSTNAME\tIP\tOS\tSTATUS\tGROUPS\tSSH ACCOUNTS\tLAST UPDATE")
	for _, h := range hosts {
		status := h.EffectiveStatus
		if status == "" {
			status = h.Status
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s %s\t%s\t%s\t%s\t%s\n", displayName(h), value(h.Hostname), value(h.IP), h.OSType, h.OSVersion, status, groupNames(h.Groups), accountNames(h.AllowedAccounts), h.LastUpdate)
	}
	return w.Flush()
}
func groupNames(groups []hostGroup) string {
	if len(groups) == 0 {
		return "-"
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return strings.Join(names, ",")
}
func accountNames(accounts []sshAccount) string {
	if len(accounts) == 0 {
		return "-"
	}
	names := make([]string, 0, len(accounts))
	for _, account := range accounts {
		names = append(names, account.LinuxUsername)
	}
	return strings.Join(names, ",")
}
func displayName(h host) string {
	if h.FriendlyName != "" {
		return h.FriendlyName
	}
	if h.Hostname != nil && *h.Hostname != "" {
		return *h.Hostname
	}
	return h.ID
}
func value(s *string) string {
	if s == nil || *s == "" {
		return "-"
	}
	return *s
}
func resolveHost(hosts []host, query string) (host, error) {
	var found []host
	for _, h := range hosts {
		if h.ID == query || h.APIID == query || strings.EqualFold(h.FriendlyName, query) || (h.Hostname != nil && strings.EqualFold(*h.Hostname, query)) {
			found = append(found, h)
		}
	}
	if len(found) == 0 {
		return host{}, fmt.Errorf("instance %q not found", query)
	}
	if len(found) > 1 {
		return host{}, fmt.Errorf("instance %q is ambiguous; use its ID", query)
	}
	return found[0], nil
}
func runSSH(args []string) error {
	fs := flag.NewFlagSet("ssh", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: patchmon ssh user@instance")
	}
	user, target, ok := strings.Cut(fs.Arg(0), "@")
	if !ok || user == "" || target == "" {
		return errors.New("a Linux account is required: patchmon ssh user@instance")
	}
	if user == "root" {
		return errors.New("root login is disabled by default")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	client := newAPIClient(cfg)
	hosts, err := client.hosts(context.Background())
	if err != nil {
		return err
	}
	host, err := resolveHost(hosts, target)
	if err != nil {
		return err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate ephemeral SSH key: %w", err)
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("encode ephemeral SSH key: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("encode ephemeral private key: %w", err)
	}
	for i := range privateKey {
		privateKey[i] = 0
	}
	var certificate struct {
		Certificate string `json:"certificate"`
		CAPublicKey string `json:"caPublicKey"`
		BastionHost string `json:"bastionHost"`
		BastionPort string `json:"bastionPort"`
		ExpiresAt   string `json:"expiresAt"`
		Recorded    bool   `json:"recorded"`
	}
	request := map[string]string{"hostId": host.ID, "linuxUsername": user, "publicKey": string(ssh.MarshalAuthorizedKey(sshPublicKey))}
	if err := client.post(context.Background(), "/api/v1/auth/ssh-certificate", request, &certificate, true); err != nil {
		return err
	}
	if certificate.Certificate == "" || certificate.BastionHost == "" || certificate.BastionPort == "" {
		return errors.New("server returned an incomplete SSH certificate")
	}
	tempDir, err := os.MkdirTemp("", "patchmon-ssh-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	keyPath := filepath.Join(tempDir, "id_ed25519")
	certPath := filepath.Join(tempDir, "id_ed25519-cert.pub")
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	for i := range privateDER {
		privateDER[i] = 0
	}
	if err := os.WriteFile(keyPath, privatePEM, 0600); err != nil {
		return err
	}
	for i := range privatePEM {
		privatePEM[i] = 0
	}
	if err := os.WriteFile(certPath, []byte(certificate.Certificate), 0600); err != nil {
		return err
	}
	if certificate.Recorded {
		fmt.Fprintf(os.Stderr, "PatchMon: this interactive session is recorded (certificate expires %s).\n", certificate.ExpiresAt)
	}
	command := exec.Command("ssh", "-p", certificate.BastionPort, "-i", keyPath, "-o", "CertificateFile="+certPath, "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new", user+"@"+certificate.BastionHost)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("OpenSSH failed: %w", err)
	}
	return nil
}

func runTunnel(args []string) error {
	fs := flag.NewFlagSet("tunnel", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New("usage: patchmon tunnel instance 22")
	}
	if fs.NArg() == 2 && fs.Arg(1) != "22" {
		return errors.New("the first tunnel version only permits SSH port 22")
	}
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	client := newAPIClient(cfg)
	hosts, err := client.hosts(context.Background())
	if err != nil {
		return err
	}
	host, err := resolveHost(hosts, fs.Arg(0))
	if err != nil {
		return err
	}
	var ticket struct {
		Ticket string `json:"ticket"`
	}
	if err := client.post(context.Background(), "/api/v1/auth/ssh-ticket", map[string]string{"hostId": host.ID}, &ticket, true); err != nil {
		return err
	}
	wsURL, err := websocketEndpointURL(cfg.Server, "ssh-tunnel", host.ID, ticket.Ticket)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 20 * time.Second}
	if cfg.Insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	conn, response, err := dialer.Dial(wsURL, http.Header{"User-Agent": []string{userAgent}})
	if err != nil {
		if response != nil {
			return fmt.Errorf("open SSH tunnel: %s", response.Status)
		}
		return fmt.Errorf("open SSH tunnel: %w", err)
	}
	defer conn.Close()
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, readErr := os.Stdin.Read(buffer)
			if n > 0 {
				if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:n]); err != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	}()
	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return nil
			}
			return err
		}
		if messageType != websocket.BinaryMessage {
			continue
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return err
		}
	}
}
func websocketURL(server, hostID, ticket string) (string, error) {
	return websocketEndpointURL(server, "ssh-terminal", hostID, ticket)
}

func websocketEndpointURL(server, endpoint, hostID, ticket string) (string, error) {
	u, err := url.Parse(server)
	if err != nil {
		return "", err
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/api/v1/" + endpoint + "/" + hostID
	q := u.Query()
	q.Set("ticket", ticket)
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func terminalName() string {
	if s := os.Getenv("TERM"); s != "" {
		return s
	}
	return "xterm-256color"
}
func readSecret(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	return string(b), err
}
