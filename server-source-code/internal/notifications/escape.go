package notifications

import "strings"

// TemplateEscape escapes a string for safe interpolation into the HTML email
// bodies built in this package.
//
// It escapes quotes as well as the three text-context characters, because the
// email builders interpolate through this helper into HTML ATTRIBUTE contexts,
// not just text ones: report_render.go puts values inside href="..." and
// src="..." (host and package links, the branding logo URL, the dashboard
// button), and notification_worker.go does the same for the "View in PatchMon"
// link. Escaping only & < > would leave a value containing a double quote free
// to close the attribute early and inject a new one, for example
// onmouseover=..., which is an XSS vector in any mail client that renders
// attributes. The branding logo URL is operator-configurable, so that path is
// reachable by a privileged user against everyone who reads the email.
//
// Escaping quotes is harmless in text context: a mail client renders &quot;
// and &#39; back to the literal characters.
//
// Ampersand MUST be replaced first, otherwise it would double-escape the
// entities emitted by the later replacements.
func TemplateEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}
