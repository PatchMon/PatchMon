package compliance

import (
	"os"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"

	"patchmon-agent/pkg/models"
)

const (
	maxAgeRule     = "xccdf_org.ssgproject.content_rule_accounts_maximum_age_login_defs"
	emptyPassRule  = "xccdf_org.ssgproject.content_rule_sshd_disable_empty_passwords"
	maxAgeValueRef = "xccdf_org.ssgproject.content_value_var_accounts_maximum_age_login_defs"
)

func loadResultsExcerpt(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/oscap-results-excerpt.xml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	return string(data)
}

func newTestScanner() *OpenSCAPScanner {
	logger := logrus.New()
	logger.SetOutput(os.Stderr)
	logger.SetLevel(logrus.PanicLevel)
	return &OpenSCAPScanner{logger: logger}
}

func TestRemediationResolvesProfileValue(t *testing.T) {
	content := loadResultsExcerpt(t)
	meta := newTestScanner().extractRuleMetadata(content, extractXCCDFValues(content))

	script := meta[maxAgeRule].Remediation
	if !strings.Contains(script, "var_accounts_maximum_age_login_defs='365'\n") {
		t.Fatalf("profile value not substituted:\n%s", script)
	}
	if strings.Contains(script, "<") && strings.Contains(script, "sub idref") {
		t.Fatalf("placeholder markup left in script:\n%s", script)
	}
}

func TestRemediationKeepsShellStructure(t *testing.T) {
	content := loadResultsExcerpt(t)
	meta := newTestScanner().extractRuleMetadata(content, extractXCCDFValues(content))

	script := meta[emptyPassRule].Remediation
	for _, want := range []string{
		"# Remediation is applicable only in certain platforms\nif dpkg-query",
		"mkdir -p /etc/ssh/sshd_config.d\nchmod 0700 /etc/ssh/sshd_config.d\n",
		`printf '%s\n' "PermitEmptyPasswords no" > "/etc/ssh/sshd_config.d/01-complianceascode-reinforce-os-defaults.conf"`,
		"\nelse\n",
		"\nfi",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Count(script, "\n") < 15 {
		t.Errorf("line breaks were collapsed:\n%s", script)
	}
}

func TestExtractXCCDFValuesPrefersSetValue(t *testing.T) {
	content := loadResultsExcerpt(t)
	if got := extractXCCDFValues(content)[maxAgeValueRef]; got != "365" {
		t.Fatalf("set-value should win over the default, got %q", got)
	}

	withoutTestResult := content[:strings.Index(content, "<TestResult")]
	if got := extractXCCDFValues(withoutTestResult)[maxAgeValueRef]; got != "60" {
		t.Fatalf("default value should be used without a set-value, got %q", got)
	}
}

func TestCleanRemediationText(t *testing.T) {
	values := map[string]string{"v_umask": "027", "v_banner": "a &amp; b"}
	tests := []struct {
		name, in, want string
	}{
		{
			name: "namespaced self-closing sub",
			in:   `umask_value='<xccdf-1.2:sub xmlns:xccdf-1.2="http://checklists.nist.gov/xccdf/1.2" idref="v_umask" use="legacy"/>'`,
			want: "umask_value='027'",
		},
		{
			name: "unprefixed sub with closing tag",
			in:   `x='<sub idref="v_umask"></sub>'`,
			want: "x='027'",
		},
		{
			name: "escaped value decoded once",
			in:   `b='<sub idref="v_banner"/>'`,
			want: "b='a & b'",
		},
		{
			name: "entities and newlines",
			in:   "\n  if [ -f x ] &amp;&amp; true; then\n    cat &lt;&lt;&lt; \"$y\" &gt;&gt; f\n  fi\n",
			want: "if [ -f x ] && true; then\n    cat <<< \"$y\" >> f\n  fi",
		},
		{
			name: "missing value is visible, not blank",
			in:   `x='<sub idref="xccdf_org.ssgproject.content_value_var_missing"/>'`,
			want: "x='UNRESOLVED:var_missing'",
		},
		{
			name: "control and bidi characters stripped",
			in:   "echo a&#27;[2J\u202eb\u2066c\td",
			want: "echo a[2Jbc\td",
		},
		{
			name: "crlf normalised",
			in:   "a\r\nb",
			want: "a\nb",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanRemediationText(tt.in, values); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMatchDiscoveredProfile(t *testing.T) {
	out := strings.Join([]string{
		"xccdf_org.ssgproject.content_profile_cis_level1_server:CIS Ubuntu 24.04 Level 1 Server Benchmark",
		"xccdf_org.ssgproject.content_profile_cis_level1_workstation:CIS Ubuntu 24.04 Level 1 Workstation Benchmark",
		"xccdf_org.ssgproject.content_profile_standard:Standard System Security Profile",
		"",
	}, "\n")
	shorten := newTestScanner().createShortID
	tests := map[string]string{
		"cis_level1_server":      "xccdf_org.ssgproject.content_profile_cis_level1_server",
		"cis_level1_workstation": "xccdf_org.ssgproject.content_profile_cis_level1_workstation",
		"standard":               "xccdf_org.ssgproject.content_profile_standard",
		"cis_level2_server":      "",
		"level1":                 "",
	}
	for shortID, want := range tests {
		if got := matchDiscoveredProfile(out, shortID, shorten); got != want {
			t.Errorf("%s: got %q, want %q", shortID, got, want)
		}
	}
}

func TestContentVersionForDerivatives(t *testing.T) {
	tests := []struct {
		name, id, version, idLike, ubuntuCodename, debianCodename, want string
	}{
		{name: "zorin 18 on noble", id: "zorin", version: "18", idLike: "ubuntu debian", ubuntuCodename: "noble", want: "24.04"},
		{name: "linux mint 22 on noble", id: "linuxmint", version: "22", idLike: "ubuntu debian", ubuntuCodename: "noble", want: "24.04"},
		{name: "lmde 6 on bookworm", id: "linuxmint", version: "6", idLike: "debian", debianCodename: "bookworm", want: "12"},
		{name: "ubuntu keeps its own version", id: "ubuntu", version: "22.04", ubuntuCodename: "jammy", want: "22.04"},
		{name: "unknown codename keeps VERSION_ID", id: "zorin", version: "19", idLike: "ubuntu debian", ubuntuCodename: "futurename", want: "19"},
		{name: "derivative without codename keeps VERSION_ID", id: "kali", version: "2025.1", idLike: "debian", want: "2025.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScanner()
			s.osInfo = models.ComplianceOSInfo{Name: tt.id, Version: tt.version}
			s.idLike = tt.idLike
			s.ubuntuCodename = tt.ubuntuCodename
			s.debianCodename = tt.debianCodename
			if got := s.contentVersion(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZorinPicksNobleContent(t *testing.T) {
	s := newTestScanner()
	s.osInfo = models.ComplianceOSInfo{Name: "zorin", Version: "18"}
	s.idLike = "ubuntu debian"
	s.ubuntuCodename = "noble"

	available := []string{"ssg-ubuntu1804-ds.xml", "ssg-ubuntu2204-ds.xml", "ssg-ubuntu2404-ds.xml"}
	if got := s.pickSSGFile(available); got != "ssg-ubuntu2404-ds.xml" {
		t.Fatalf("server sync picked %q", got)
	}
	if s.contentFileMatchesOSVersion("ssg-ubuntu1804-ds.xml") {
		t.Fatal("18.04 content must be reported as a mismatch on a noble base")
	}
	if !s.contentFileMatchesOSVersion("ssg-ubuntu2404-ds.xml") {
		t.Fatal("24.04 content must match a noble base")
	}
}

func TestUCSPicksBookwormContent(t *testing.T) {
	const osRelease = `PRETTY_NAME="Univention Corporate Server 5.2"
NAME="Univention Corporate Server"
VERSION="5.2"
ID="ucs"
VERSION_ID="5.2"
ID_LIKE="debian"
VERSION_CODENAME=bookworm
`
	s := newTestScanner()
	s.osInfo = s.parseOSRelease(strings.NewReader(osRelease))
	if s.osInfo.Family != "debian" {
		t.Fatalf("family %q, want debian", s.osInfo.Family)
	}
	available := []string{"ssg-debian11-ds.xml", "ssg-debian12-ds.xml", "ssg-debian13-ds.xml"}
	if got := s.pickSSGFile(available); got != "ssg-debian12-ds.xml" {
		t.Fatalf("server sync picked %q", got)
	}
	if got := s.getProfileID("level1_server"); got != "xccdf_org.ssgproject.content_profile_cis_level1_server" {
		t.Fatalf("profile %q", got)
	}
}

func TestDebianCodenameWinsOverVersionCodename(t *testing.T) {
	s := newTestScanner()
	s.osInfo = s.parseOSRelease(strings.NewReader("ID=linuxmint\nVERSION_ID=6\nID_LIKE=debian\nVERSION_CODENAME=faye\nDEBIAN_CODENAME=bookworm\n"))
	if got := s.contentVersion(); got != "12" {
		t.Fatalf("contentVersion %q, want 12", got)
	}
}

func TestOSReleaseContentSelection(t *testing.T) {
	tests := []struct {
		name, osRelease, wantName, wantVersion string
	}{
		{"ubuntu", "ID=ubuntu\nVERSION_ID=\"24.04\"\nID_LIKE=debian\nVERSION_CODENAME=noble\n", "ubuntu", "24.04"},
		{"debian", "ID=debian\nVERSION_ID=\"12\"\nVERSION_CODENAME=bookworm\n", "debian", "12"},
		{"mint", "ID=linuxmint\nVERSION_ID=\"22\"\nID_LIKE=\"ubuntu debian\"\nVERSION_CODENAME=wilma\nUBUNTU_CODENAME=noble\n", "ubuntu", "24.04"},
		{"kali", "ID=kali\nVERSION_ID=\"2025.1\"\nID_LIKE=debian\nVERSION_CODENAME=kali-rolling\n", "debian", "2025.1"},
		{"ucs 5.2", "ID=\"ucs\"\nVERSION_ID=\"5.2\"\nID_LIKE=\"debian\"\nVERSION_CODENAME=bookworm\n", "debian", "12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestScanner()
			s.osInfo = s.parseOSRelease(strings.NewReader(tt.osRelease))
			if got := s.getContentOSName(); got != tt.wantName {
				t.Fatalf("content OS %q, want %q", got, tt.wantName)
			}
			if got := s.contentVersion(); got != tt.wantVersion {
				t.Fatalf("content version %q, want %q", got, tt.wantVersion)
			}
		})
	}
}
