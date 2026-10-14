package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
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

func TestSanitizeSVGBoundsEntityExpansion(t *testing.T) {
	head := `<!DOCTYPE svg [<!ENTITY a "` + strings.Repeat("x", 256) + `">]><svg xmlns="http://www.w3.org/2000/svg"><text>`
	in := append([]byte(head), bytes.Repeat([]byte("&a;"), (3750000-len(head))/3)...)
	in = append(in, `</text></svg>`...)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := sanitizeSVG(in)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("entity expansion bomb accepted")
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 64<<20 {
		t.Fatalf("rejecting a %d byte entity bomb allocated %d MB", len(in), alloc>>20)
	}

	var decls strings.Builder
	for i := 0; i <= maxSVGEntities; i++ {
		decls.WriteString(`<!ENTITY e` + strconv.Itoa(i) + ` "v">`)
	}
	if _, err := sanitizeSVG([]byte(`<!DOCTYPE svg [` + decls.String() + `]>` + svgOpen + `</svg>`)); err == nil {
		t.Error("too many entity declarations accepted")
	}
	small := `<!DOCTYPE svg [<!ENTITY n "http://www.w3.org/2000/svg">]><svg xmlns="&n;"><text>&n;</text></svg>`
	if _, err := sanitizeSVG([]byte(small)); err != nil {
		t.Errorf("small entity use rejected: %v", err)
	}
}

func TestSanitizeSVGDepthCap(t *testing.T) {
	t.Parallel()
	deep := svgOpen + strings.Repeat("<g>", maxSVGDepth) + strings.Repeat("</g>", maxSVGDepth) + `</svg>`
	if _, err := sanitizeSVG([]byte(deep)); err == nil {
		t.Error("nesting beyond the depth cap accepted")
	}
	ok := svgOpen + strings.Repeat("<g>", 50) + strings.Repeat("</g>", 50) + `</svg>`
	if _, err := sanitizeSVG([]byte(ok)); err != nil {
		t.Errorf("ordinary nesting rejected: %v", err)
	}
}

func TestSanitizeSVGCleansCSSAndBase(t *testing.T) {
	t.Parallel()
	in := svgOpen + `<style>@\69mport url(https://evil.example/a.css); @IMPORT "https://evil.example/b.css"; .a{background:u\72l("https://evil.example/c.png")} .b{background-image:image-set("https://evil.example/d.png" 1x)} .c{fill:url(#g)}</style>` +
		`<g xml:base="https://evil.example/" xml:space="preserve"><rect width="1" height="1" fill="url(https://evil.example/e#x)" filter="url('//evil.example/f#x')" style="stroke:url(&quot;https://evil.example/g&quot;)"/><rect width="1" height="1" fill="url(#g)"/></g></svg>`
	out, err := sanitizeSVG([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ToLower(string(out))
	for _, bad := range []string{"evil.example", "@import", "xml:base", "image-set"} {
		if strings.Contains(s, bad) {
			t.Errorf("output still contains %q: %s", bad, out)
		}
	}
	for _, want := range []string{`fill="url(#g)"`, `.c{fill:url(#g)}`, `xml:space="preserve"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
}

func TestSanitizeSVGDropsDoubleEscapedCSS(t *testing.T) {
	t.Parallel()
	in := svgOpen + `<style>@\5c 69mport "https://evil.example/a.css"; .a{fill:red}</style>` +
		`<rect width="1" height="1" style="fill:u\5c 72l(https://evil.example/b)"/><rect width="1" height="1" style="fill:#0a0"/></svg>`
	out, err := sanitizeSVG([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"evil.example", "mport", `\`} {
		if strings.Contains(s, bad) {
			t.Errorf("output still contains %q: %s", bad, s)
		}
	}
	if !strings.Contains(s, `style="fill:#0a0"`) {
		t.Errorf("plain style was dropped: %s", s)
	}
}

func TestSanitizeSVGGhsaGgw5Corpus(t *testing.T) {
	t.Parallel()
	const ns = `xmlns="http://www.w3.org/2000/svg"`
	const xl = `xmlns:xlink="http://www.w3.org/1999/xlink"`
	cases := []struct {
		name, in string
		banned   []string
	}{
		{"advisory script bypass", `<script>alert(1) <<script />/script>`, []string{"<script", "alert"}},
		{"advisory script in svg", `<svg ` + ns + `><script>alert(1) <<script />/script></svg>`, []string{"<script", "alert"}},
		{"svg onload", `<svg ` + ns + ` onload="alert(1)"></svg>`, []string{"onload", "alert"}},
		{"svg slash onload", `<svg/onload=alert(1)>`, []string{"onload", "alert"}},
		{"anchor javascript href", `<svg ` + ns + `><a href="javascript:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"javascript", "alert"}},
		{"xlink javascript href", `<svg ` + ns + ` ` + xl + `><a xlink:href="javascript:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"javascript", "alert"}},
		{"tab inside scheme", `<svg ` + ns + `><a href="java&#9;script:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"script", "alert"}},
		{"foreign object", `<svg ` + ns + `><foreignObject><p>x</p></foreignObject></svg>`, []string{"foreignobject", "<p"}},
		{"use external ref", `<svg ` + ns + ` ` + xl + `><use xlink:href="https://evil.example/x.svg#a"/></svg>`, []string{"evil.example"}},
		{"non image data href", `<svg ` + ns + `><a href="data:text/html;base64,PHN2Zz48L3N2Zz4="><rect width="1" height="1"/></a></svg>`, []string{"data:"}},
		{"svg data uri on navigable href", `<svg ` + ns + `><a href="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="><rect width="1" height="1"/></a></svg>`, []string{"data:"}},
		{"svg data uri on xlink href", `<svg ` + ns + ` ` + xl + `><image xlink:href="data:image/svg+xml;base64,PHN2Zz48L3N2Zz4="/></svg>`, []string{"data:"}},
		{"xml stylesheet pi", `<?xml-stylesheet href="https://evil.example/x.css"?><svg ` + ns + `></svg>`, []string{"<?", "evil.example"}},
		{"doctype entity", `<!DOCTYPE svg [<!ENTITY x "y">]><svg ` + ns + `></svg>`, []string{"<!", "entity"}},
		{"not an svg", `<html><body>hi</body></html>`, []string{"<html", "hi"}},
		{"smil animate href", `<svg ` + ns + `><a><animate attributeName="href" to="javascript:alert(1)"/><text>x</text></a></svg>`, []string{"animate", "javascript"}},
		{"smil set onload", `<svg ` + ns + `><set attributeName="onload" to="alert(1)"/></svg>`, []string{"<set", "alert"}},
		{"smil animateTransform", `<svg ` + ns + `><rect width="1" height="1"><animateTransform attributeName="transform" type="rotate" from="0" to="360" dur="1s"/></rect></svg>`, []string{"animate"}},
		{"smil animateMotion", `<svg ` + ns + `><rect width="1" height="1"><animateMotion path="M0 0 L1 1" dur="1s"/></rect></svg>`, []string{"animate"}},
		{"prefixed svg script", `<svg ` + ns + `><x:script xmlns:x="http://www.w3.org/2000/svg">alert(1)</x:script></svg>`, []string{"script", "alert"}},
		{"default ns remapped on child", `<svg ` + ns + `><g xmlns="http://www.w3.org/1999/xhtml"><script>alert(1)</script></g></svg>`, []string{"script", "alert", "xhtml"}},
		{"mixed case scheme", `<svg ` + ns + `><a href="JaVaScRiPt:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"javascript", "alert"}},
		{"leading control chars in scheme", `<svg ` + ns + `><a href="&#x1;&#xC; javascript:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"javascript", "alert"}},
		{"newline inside scheme", `<svg ` + ns + `><a href="java&#10;script:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"script", "alert"}},
		{"hex entity scheme", `<svg ` + ns + `><a href="&#x6a;avascript:alert(1)"><rect width="1" height="1"/></a></svg>`, []string{"javascript", "alert"}},
		{"recursive entity", `<!DOCTYPE svg [<!ENTITY a "&b;"><!ENTITY b "&a;">]><svg ` + ns + `><text>&a;</text></svg>`, []string{"&a;", "&b;"}},
		{"css url javascript", `<svg ` + ns + `><style>.a{background:url(javascript:alert(1))}</style><rect class="a" style="fill:url('javascript:alert(1)')" width="1" height="1"/></svg>`, []string{"javascript", "alert"}},
		{"css expression", `<svg ` + ns + `><style>.a{width:expression(alert(1))}</style><rect style="width:expression(alert(1))" width="1" height="1"/></svg>`, []string{"expression", "alert"}},
		{"css import", `<svg ` + ns + `><style>@import url(https://evil.example/x.css);</style></svg>`, []string{"@import", "evil.example"}},
		{"html polyglot", `<svg ` + ns + `><p xmlns="http://www.w3.org/1999/xhtml"><img src="x" onerror="alert(1)"/></p><rect width="1" height="1"/></svg>`, []string{"<p", "<img", "onerror", "alert"}},
	}
	for _, c := range cases {
		out, err := sanitizeSVG([]byte(c.in))
		if err != nil {
			continue
		}
		lower := strings.ToLower(string(out))
		for _, b := range c.banned {
			if strings.Contains(lower, strings.ToLower(b)) {
				t.Errorf("%s: output still contains %q: %s", c.name, b, out)
			}
		}
	}

	legit := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24"><title>Logo</title><defs><linearGradient id="g"><stop offset="0" stop-color="#fff"/><stop offset="1" stop-color="#000"/></linearGradient></defs><rect x="0" y="0" width="24" height="24" rx="4" fill="url(#g)"/><path d="M4 4 L20 20" stroke="#333" stroke-width="2" fill="none"/><circle cx="12" cy="12" r="6" fill="#09f"/><g transform="translate(1,1)"><text x="2" y="2" style="font-size:4px">P</text></g></svg>`
	out, err := sanitizeSVG([]byte(legit))
	if err != nil {
		t.Fatalf("legit logo rejected: %v", err)
	}
	for _, want := range []string{"<svg", "<title>Logo</title>", "<linearGradient", "<stop", "<rect", "<path", "<circle", `<g transform="translate(1,1)">`, "<text", `fill="url(#g)"`, `style="font-size:4px"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("legit logo lost %q: %s", want, out)
		}
	}
}
