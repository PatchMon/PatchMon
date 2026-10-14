package handler

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/store"
	"github.com/go-chi/chi/v5"
)

const svgOpen = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100" height="40">`

func TestSanitizeSVGStripsActiveContent(t *testing.T) {
	t.Parallel()
	payloads := map[string]string{
		"script end tag with space":    `<script>alert(1)</script >`,
		"script end tag with newline":  "<script>alert(1)</script\n>",
		"script end tag with tab":      "<script>alert(1)</script\t>",
		"prefixed script":              `<x:script xmlns:x="http://www.w3.org/2000/svg">alert(1)</x:script>`,
		"prefixed svg namespace":       `<svg:script xmlns:svg="http://www.w3.org/2000/svg">alert(1)</svg:script>`,
		"xhtml iframe":                 `<h:iframe xmlns:h="http://www.w3.org/1999/xhtml" src="javascript:alert(1)"/>`,
		"default ns switched to xhtml": `<g xmlns="http://www.w3.org/1999/xhtml"><img src="x" onerror="alert(1)"/></g>`,
		"onload attribute":             `<rect width="1" height="1" onload="alert(1)"/>`,
		"upper-case handler":           `<rect width="1" height="1" ONLOAD="alert(1)"/>`,
		"entity-encoded javascript":    `<a href="&#106;avascript:alert(1)"><text>x</text></a>`,
		"xlink javascript":             `<a xlink:href=" javascript:alert(1)"><text>x</text></a>`,
		"foreignObject":                `<foreignObject><div xmlns="http://www.w3.org/1999/xhtml"><script>alert(1)</script></div></foreignObject>`,
		"animate sets href":            `<a><set attributeName="href" to="javascript:alert(1)"/><text>x</text></a>`,
		"external use":                 `<use href="https://evil.example/x.svg#a"/>`,
		"external image":               `<image href="https://evil.example/x.png"/>`,
		"css import":                   `<style>@import url(https://evil.example/x.css); .a{fill:red}</style>`,
	}
	for name, p := range payloads {
		out, err := sanitizeSVG([]byte(svgOpen + p + `</svg>`))
		if err != nil {
			t.Errorf("%s: unexpected error %v", name, err)
			continue
		}
		lower := strings.ToLower(string(out))
		for _, bad := range []string{"script", "javascript", "onload", "onerror", "iframe", "foreignobject", "<set", "evil.example", "1999/xhtml"} {
			if strings.Contains(lower, bad) {
				t.Errorf("%s: output still contains %q: %s", name, bad, out)
			}
		}
		if err := xml.Unmarshal(out, new(struct{})); err != nil {
			t.Errorf("%s: output is not well-formed: %v", name, err)
		}
	}
}

func TestSanitizeSVGRejectsMalformed(t *testing.T) {
	t.Parallel()
	for name, in := range map[string]string{
		"unclosed script":   svgOpen + `<script>alert(1)`,
		"mismatched end":    svgOpen + `<g></rect></svg>`,
		"html root":         `<html><svg></svg></html>`,
		"undeclared ent":    svgOpen + `&xxe;</svg>`,
		"two roots":         svgOpen + `</svg><svg xmlns="http://www.w3.org/2000/svg"></svg>`,
		"external entity":   `<!DOCTYPE svg [<!ENTITY x SYSTEM "file:///etc/passwd">]>` + svgOpen + `&x;</svg>`,
		"not xml":           `<svg <script>`,
		"prefixed svg root": `<s:svg xmlns:s="http://www.w3.org/2000/svg"></s:svg>`,
	} {
		if out, err := sanitizeSVG([]byte(in)); err == nil {
			t.Errorf("%s: expected rejection, got %s", name, out)
		}
	}
}

func TestSanitizeSVGKeepsLogoContent(t *testing.T) {
	t.Parallel()
	in := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg [<!ENTITY ns_svg "http://www.w3.org/2000/svg"><!ENTITY ns_x "http://ns.adobe.com/Extensibility/1.0/">]>
<svg xmlns="&ns_svg;" xmlns:x="&ns_x;" xmlns:xlink="http://www.w3.org/1999/xlink" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" viewBox="0 0 100 40" inkscape:version="1.3">
<!-- exported -->
<title>Brand</title>
<x:pgf>junk</x:pgf>
<defs><linearGradient id="g"><stop offset="0" stop-color="#f00"/></linearGradient>
<style><![CDATA[.t>tspan{font:12px sans-serif}]]></style></defs>
<rect width="100" height="40" fill="url(#g)" style="opacity:.9"/>
<use xlink:href="#g"/>
<image href="data:image/png;base64,iVBORw0KGgo="/>
<text class="t" x="10" y="25" xml:space="preserve">Brand &amp; Co</text>
</svg>`
	out, err := sanitizeSVG([]byte(in))
	if err != nil {
		t.Fatalf("legitimate logo rejected: %v", err)
	}
	s := string(out)
	for _, want := range []string{`xmlns="http://www.w3.org/2000/svg"`, `xmlns:xlink=`, `<linearGradient id="g">`, `xlink:href="#g"`, `data:image/png;base64`, `Brand &amp; Co`, `.t&gt;tspan`, `xml:space="preserve"`, `style="opacity:.9"`, `<title>Brand</title>`} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in %s", want, s)
		}
	}
	for _, gone := range []string{"inkscape", "pgf", "junk", "adobe", "<!--"} {
		if strings.Contains(s, gone) {
			t.Errorf("unexpected %q in %s", gone, s)
		}
	}
}

func TestGetLogoServesSVGWithRestrictiveHeaders(t *testing.T) {
	d := testDatabase(t)
	h := &SettingsHandler{settings: store.NewSettingsStore(d)}
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="40"><!-- pad pad pad pad pad pad pad pad pad pad pad pad pad pad --><script>alert(1)</script ><rect width="1" height="1"/></svg>`

	up := httptest.NewRequest(http.MethodPost, "/settings/logos/upload", strings.NewReader(`{"logoType":"light","fileContent":"`+base64.StdEncoding.EncodeToString([]byte(svg))+`"}`))
	rec := httptest.NewRecorder()
	h.UploadLogo(rec, up)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/logos/light", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("type", "light")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec = httptest.NewRecorder()
	h.GetLogo(rec, req)
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusOK || strings.Contains(string(body), "script") {
		t.Fatalf("get: %d %s", rec.Code, body)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "sandbox") || !strings.Contains(got, "default-src 'none'") {
		t.Errorf("CSP = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
}
