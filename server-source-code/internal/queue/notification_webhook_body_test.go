package queue

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
)

// Real-world token shapes: Mattermost IDs are 26 chars, Rocket.Chat is a 17-char
// id plus a longer token.
const (
	mattermostToken   = "873nng3nmpbpxrfz8wtjzge8oa"
	rocketChatID      = "abcdefghij1234567"
	rocketChatToken   = "aBcDeF1234567890aBcDeF1234567890aBcDeF1234567890"
	reportSubject     = "Weekly patch summary"
	reportHTML        = "<p>12 hosts need updates</p>"
	reportCSV         = "host,updates\nweb-01,4\n"
	signingSecretForT = "s3cr3t"
)

func testPayload() notifications.NotificationDeliverPayload {
	return notifications.NotificationDeliverPayload{
		ChannelType:   "webhook",
		EventType:     "host_down",
		Severity:      "critical",
		Title:         "Host is down",
		Message:       "web-01 has stopped reporting",
		ReferenceType: "host",
		ReferenceID:   "abc-123",
		Metadata: map[string]interface{}{
			"host_name": "web-01",
			"app_link":  "https://patchmon.example/hosts/abc-123",
		},
	}
}

type capture struct {
	body    []byte
	sigHdr  string
	decoded map[string]interface{}
}

// captureWith posts through post to a stub receiver mounted at path and returns
// what the receiver saw.
func captureWith(t *testing.T, path, signingSecret string, post func(t *testing.T, cfg string)) capture {
	t.Helper()

	type seen struct {
		body []byte
		sig  string
	}
	ch := make(chan seen, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		ch <- seen{body: b, sig: r.Header.Get("X-PatchMon-Signature")}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfgMap := map[string]string{"url": srv.URL + path}
	if signingSecret != "" {
		cfgMap["signing_secret"] = signingSecret
	}
	cfg, err := json.Marshal(cfgMap)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	post(t, string(cfg))

	var got seen
	select {
	case got = <-ch:
	default:
		t.Fatal("receiver was never called")
	}

	out := capture{body: got.body, sigHdr: got.sig}
	if err := json.Unmarshal(got.body, &out.decoded); err != nil {
		t.Fatalf("unmarshal captured body %q: %v", string(got.body), err)
	}
	return out
}

func captureNotification(t *testing.T, path string) capture {
	t.Helper()
	return captureWith(t, path, "", func(t *testing.T, cfg string) {
		t.Helper()
		h := &NotificationDeliverHandler{}
		if err := h.sendWebhook(context.Background(), cfg, testPayload()); err != nil {
			t.Fatalf("sendWebhook: %v", err)
		}
	})
}

func captureScheduledReport(t *testing.T, path string) capture {
	t.Helper()
	return captureWith(t, path, "", func(t *testing.T, cfg string) {
		t.Helper()
		err := sendScheduledWebhook(context.Background(), cfg, reportSubject, reportHTML, reportCSV)
		if err != nil {
			t.Fatalf("sendScheduledWebhook: %v", err)
		}
	})
}

func TestIsSlackCompatibleWebhookURL(t *testing.T) {
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"mattermost", "https://chat.example.com/hooks/" + mattermostToken, true},
		{"mattermost behind a subpath proxy", "https://example.com/mattermost/hooks/" + mattermostToken, true},
		{"mattermost with a trailing slash", "https://chat.example.com/hooks/" + mattermostToken + "/", true},
		{"rocket.chat", "https://chat.example.com/hooks/" + rocketChatID + "/" + rocketChatToken, true},
		{"uppercase hooks segment", "https://chat.example.com/Hooks/" + mattermostToken, true},

		{"no token after hooks", "https://chat.example.com/hooks", false},
		{"empty token after hooks", "https://chat.example.com/hooks/", false},
		{"token too short to be opaque", "https://chat.example.com/hooks/abc123", false},
		{"more than two trailing segments", "https://chat.example.com/hooks/" + mattermostToken + "/a/b", false},

		// A Zapier catch hook is /hooks/catch/<id>/<key>: three trailing segments,
		// and "catch" is a routing word, so it keeps the structured body.
		{"zapier catch hook keeps the structured body", "https://hooks.zapier.com/hooks/catch/123456/abcdef", false},
		{"zapier catch hook with trailing slash", "https://hooks.zapier.com/hooks/catch/123456/abcdef/", false},

		// Jira/Confluence automation hands out /pro/hooks/<40 char token>, which is
		// shaped exactly like a chat webhook. Atlassian answers 2xx to anything, so
		// mis-detecting it would silently empty every rule built on the structured body.
		{"atlassian automation keeps the structured body", "https://automation.atlassian.com/pro/hooks/1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d", false},
		{"atlassian legacy domain", "https://automation.codebarrel.io/pro/hooks/1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d", false},
		{"atlassian host match is case-insensitive", "https://Automation.Atlassian.COM/pro/hooks/1a2b3c4d5e6f708192a3b4c5d6e7f8091a2b3c4d", false},

		{"discord is not matched here", "https://discord.com/api/webhooks/123/tok", false},
		{"slack is not matched here", "https://hooks.slack.com/services/T00000000/B00000000/abcdefghijklmnop", false},
		{"hostname alone does not count", "https://hooks.example.com/ingest/" + mattermostToken, false},
		{"workato uses the plural webhooks", "https://www.workato.com/webhooks/rest/" + mattermostToken, false},
		{"n8n uses the singular webhook", "https://n8n.example.com/webhook/" + mattermostToken, false},
		{"unrelated generic endpoint", "https://example.com/api/notify", false},
		{"empty", "", false},

		{"query string is ignored", "https://chat.example.com/hooks/" + mattermostToken + "?channel=ops", true},
		{"fragment is ignored", "https://chat.example.com/hooks/" + mattermostToken + "#frag", true},
		{"percent-encoded separator is not a segment break", "https://chat.example.com/hooks%2F" + mattermostToken, false},
		{"unparseable url", "https://%zz/hooks/" + mattermostToken, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSlackCompatibleWebhookURL(tc.url); got != tc.want {
				t.Errorf("isSlackCompatibleWebhookURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}

// Mattermost rejects any incoming-webhook payload carrying neither a top-level
// "text" nor "attachments" with HTTP 400. See issue #851.
func TestMattermostNotificationBody(t *testing.T) {
	got := captureNotification(t, "/hooks/"+mattermostToken)

	text, ok := got.decoded["text"].(string)
	if !ok || text == "" {
		t.Fatalf("no usable top-level text key; got %v", got.decoded)
	}
	if !strings.Contains(text, "Host is down") {
		t.Errorf("text is missing the notification title; got %q", text)
	}
	if _, ok := got.decoded["event_type"]; ok {
		t.Errorf("Slack-shaped body should not carry the generic event_type key; got %v", got.decoded)
	}
}

func TestMattermostScheduledReportBody(t *testing.T) {
	got := captureScheduledReport(t, "/hooks/"+mattermostToken)

	text, ok := got.decoded["text"].(string)
	if !ok || text == "" {
		t.Fatalf("no usable top-level text key; got %v", got.decoded)
	}
	if !strings.Contains(text, reportSubject) {
		t.Errorf("text is missing the report subject; got %q", text)
	}
}

// The Slack payload must not drift: this is the shape Slack, Mattermost and
// Rocket.Chat all receive.
func TestSlackNotificationBodyShape(t *testing.T) {
	got := captureNotification(t, "/hooks/"+mattermostToken)

	if len(got.decoded) != 3 {
		t.Errorf("expected exactly text/username/icon_emoji, got %v", got.decoded)
	}
	if u := got.decoded["username"]; u != "PatchMon" {
		t.Errorf("username = %v, want PatchMon", u)
	}
	if e := got.decoded["icon_emoji"]; e != ":bell:" {
		t.Errorf("icon_emoji = %v, want :bell:", e)
	}
}

func TestSlackScheduledReportBodyShape(t *testing.T) {
	got := captureScheduledReport(t, "/hooks/"+mattermostToken)

	if len(got.decoded) != 3 {
		t.Errorf("expected exactly text/username/icon_emoji, got %v", got.decoded)
	}
	if u := got.decoded["username"]; u != "PatchMon" {
		t.Errorf("username = %v, want PatchMon", u)
	}
	if e := got.decoded["icon_emoji"]; e != ":bar_chart:" {
		t.Errorf("icon_emoji = %v, want :bar_chart:", e)
	}
}

// Discord routing and payload are untouched by the Mattermost work.
func TestDiscordBodiesUnchanged(t *testing.T) {
	if !isDiscordWebhookURL("https://discord.com/api/webhooks/123/tok") {
		t.Fatal("discord URL no longer detected")
	}
	if isSlackCompatibleWebhookURL("https://discord.com/api/webhooks/123/tok") {
		t.Fatal("discord URL must not be claimed by the Slack-compatible matcher")
	}

	b, err := discordWebhookBody(testPayload())
	if err != nil {
		t.Fatalf("discordWebhookBody: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(b, &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["username"] != "PatchMon" {
		t.Errorf("discord username = %v", body["username"])
	}
	embeds, ok := body["embeds"].([]interface{})
	if !ok || len(embeds) != 1 {
		t.Fatalf("discord body lost its embeds array; got %v", body)
	}
	embed, ok := embeds[0].(map[string]interface{})
	if !ok {
		t.Fatalf("discord embed is not an object; got %v", embeds[0])
	}
	if embed["title"] != "Host is down" {
		t.Errorf("discord embed title = %v", embed["title"])
	}
	// critical maps to red; a drift here silently miscolours every alert.
	if c, _ := embed["color"].(float64); int(c) != discordColorForSeverity("critical") {
		t.Errorf("discord embed color = %v, want %d", embed["color"], discordColorForSeverity("critical"))
	}

	rb, err := discordScheduledReportWebhookBody(reportSubject, reportHTML, reportCSV)
	if err != nil {
		t.Fatalf("discordScheduledReportWebhookBody: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal(rb, &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := report["embeds"].([]interface{}); !ok {
		t.Errorf("discord report body lost its embeds array; got %v", report)
	}
}

func TestSlackIncomingWebhookURLUnchanged(t *testing.T) {
	if !isSlackIncomingWebhookURL("https://hooks.slack.com/services/T/B/X") {
		t.Error("slack URL no longer detected")
	}
}

// Any other receiver keeps the PatchMon-shaped body and additionally gets a
// top-level text, so Slack-compatible endpoints on an unexpected path still work.
func TestGenericNotificationBodyKeepsShapeAndAddsText(t *testing.T) {
	got := captureNotification(t, "/generic/receiver")

	for _, k := range []string{"event_type", "severity", "title", "message", "reference", "metadata"} {
		if _, ok := got.decoded[k]; !ok {
			t.Errorf("generic body lost the %q key; got %v", k, got.decoded)
		}
	}
	if text, ok := got.decoded["text"].(string); !ok || text == "" {
		t.Errorf("generic body has no usable top-level text key; got %v", got.decoded)
	}
	if got.decoded["app_link"] != "https://patchmon.example/hosts/abc-123" {
		t.Errorf("generic body lost the promoted app_link; got %v", got.decoded["app_link"])
	}
}

func TestGenericScheduledReportBodyKeepsShapeAndAddsText(t *testing.T) {
	got := captureScheduledReport(t, "/generic/receiver")

	for _, k := range []string{"kind", "subject", "html", "csv"} {
		if _, ok := got.decoded[k]; !ok {
			t.Errorf("generic body lost the %q key; got %v", k, got.decoded)
		}
	}
	if got.decoded["kind"] != "scheduled_report" {
		t.Errorf("kind = %v, want scheduled_report", got.decoded["kind"])
	}
	if got.decoded["csv"] != reportCSV {
		t.Errorf("generic body must still carry the csv in full; got %v", got.decoded["csv"])
	}
	if text, ok := got.decoded["text"].(string); !ok || text == "" {
		t.Errorf("generic body has no usable top-level text key; got %v", got.decoded)
	}
}

// The scheduled-report generic body already carries html and csv in full, so its
// fallback text stays bounded rather than duplicating the whole report, and it must
// not truncate into a code fence.
func TestGenericScheduledReportTextIsBoundedAndWellFormed(t *testing.T) {
	longHTML := "<p>" + strings.Repeat("host web-01 needs updates. ", 2000) + "</p>"
	longCSV := strings.Repeat("web-01,4\n", 2000)

	got := captureWith(t, "/generic/receiver", "", func(t *testing.T, cfg string) {
		t.Helper()
		if err := sendScheduledWebhook(context.Background(), cfg, reportSubject, longHTML, longCSV); err != nil {
			t.Fatalf("sendScheduledWebhook: %v", err)
		}
	})

	text, _ := got.decoded["text"].(string)
	if n := utf8.RuneCountInString(text); n > genericFallbackTextMaxRunes {
		t.Errorf("fallback text is %d runes, want <= %d", n, genericFallbackTextMaxRunes)
	}
	if !strings.Contains(text, reportSubject) {
		t.Errorf("fallback text lost the subject; got %q", text)
	}
	if n := strings.Count(text, "```"); n%2 != 0 {
		t.Errorf("fallback text has an unterminated code fence (%d fence markers); got %q", n, text)
	}
	if strings.Contains(text, "host,updates") {
		t.Errorf("fallback text should not duplicate the csv already in the body; got %q", text)
	}
}

// Pins the Slack text rendering so a future refactor of the builders cannot drift
// the payload Slack, Mattermost and Rocket.Chat all receive.
func TestSlackNotificationTextGolden(t *testing.T) {
	const want = "🔴 *Host is down*\n\nweb-01 has stopped reporting\n\n" +
		"*Host:* web-01\n*Severity:* 🔴 CRITICAL\n" +
		"\n<https://patchmon.example/hosts/abc-123|🔗 View in PatchMon>\n"

	if got := slackIncomingWebhookText(testPayload()); got != want {
		t.Errorf("slack text drifted:\n got %q\nwant %q", got, want)
	}
}

// The signature must cover the exact bytes sent, whichever body shape was chosen.
func TestSigningSecretCoversTheSentBody(t *testing.T) {
	for _, path := range []string{"/hooks/" + mattermostToken, "/generic/receiver"} {
		t.Run(path, func(t *testing.T) {
			got := captureWith(t, path, signingSecretForT, func(t *testing.T, cfg string) {
				t.Helper()
				h := &NotificationDeliverHandler{}
				if err := h.sendWebhook(context.Background(), cfg, testPayload()); err != nil {
					t.Fatalf("sendWebhook: %v", err)
				}
			})

			mac := hmac.New(sha256.New, []byte(signingSecretForT))
			mac.Write(got.body)
			want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
			if got.sigHdr != want {
				t.Errorf("signature = %q, want %q", got.sigHdr, want)
			}
		})
	}
}
