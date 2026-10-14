package handler

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	svgNamespace   = "http://www.w3.org/2000/svg"
	xlinkNamespace = "http://www.w3.org/1999/xlink"
)

// logoCSP stops a stored SVG running script when its URL is opened directly.
const logoCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src data:; sandbox"

var svgAllowedElements = map[string]bool{
	"svg": true, "g": true, "defs": true, "symbol": true, "use": true, "title": true, "desc": true,
	"path": true, "rect": true, "circle": true, "ellipse": true, "line": true, "polyline": true, "polygon": true,
	"text": true, "tspan": true, "textPath": true, "image": true, "style": true, "switch": true, "a": true,
	"linearGradient": true, "radialGradient": true, "stop": true, "clipPath": true, "mask": true, "pattern": true, "marker": true,
	"filter": true, "feBlend": true, "feColorMatrix": true, "feComponentTransfer": true, "feComposite": true,
	"feConvolveMatrix": true, "feDiffuseLighting": true, "feDisplacementMap": true, "feDistantLight": true,
	"feDropShadow": true, "feFlood": true, "feFuncA": true, "feFuncB": true, "feFuncG": true, "feFuncR": true,
	"feGaussianBlur": true, "feImage": true, "feMerge": true, "feMergeNode": true, "feMorphology": true, "feOffset": true,
	"fePointLight": true, "feSpecularLighting": true, "feSpotLight": true, "feTile": true, "feTurbulence": true,
}

const (
	maxSVGEntities        = 16
	maxSVGEntityExpansion = 64 << 10
	maxSVGDepth           = 256
)

var (
	svgEntityDecl = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][\w.-]*)\s+"([^"%&<]{0,256})"\s*>`)
	cssEscape     = regexp.MustCompile(`(?s)\\(?:([0-9A-Fa-f]{1,6})[ \t\n\f]?|(.))`)
	cssImport     = regexp.MustCompile(`(?i)@import\b[^;]*;?`)
	cssExpression = regexp.MustCompile(`(?i)expression\s*\(`)
	cssImageSet   = regexp.MustCompile(`(?i)(-webkit-)?image-set\([^)]*\)`)
	cssURL        = regexp.MustCompile(`(?i)url\(\s*['"]?\s*([^'")\s]*)[^)]*\)`)
	errInvalidSVG = errors.New("invalid SVG")
)

// svgEntities returns the internal entities the input declares, refusing any set whose total
// expansion could exceed the input size, so references cannot multiply memory use.
func svgEntities(in []byte) (map[string]string, error) {
	decls := svgEntityDecl.FindAllSubmatch(in, maxSVGEntities+1)
	if len(decls) > maxSVGEntities {
		return nil, errInvalidSVG
	}
	budget := min(len(in), maxSVGEntityExpansion)
	entities := make(map[string]string, len(decls))
	total := 0
	for _, m := range decls {
		name := string(m[1])
		if _, seen := entities[name]; seen {
			continue
		}
		entities[name] = string(m[2])
		total += bytes.Count(in, []byte("&"+name+";")) * len(m[2])
		if total > budget {
			return nil, errInvalidSVG
		}
	}
	return entities, nil
}

func unescapeCSS(v string) string {
	return cssEscape.ReplaceAllStringFunc(v, func(m string) string {
		sub := cssEscape.FindStringSubmatch(m)
		if sub[1] == "" {
			if sub[2] == "\n" {
				return ""
			}
			return sub[2]
		}
		n, _ := strconv.ParseUint(sub[1], 16, 32)
		if n == 0 || n > utf8.MaxRune || !utf8.ValidRune(rune(n)) {
			return "\uFFFD"
		}
		return string(rune(n))
	})
}

// cleanCSS drops @import, image-set() and url() references to anything but a fragment or embedded image.
// A value still holding a backslash after one unescape (a double escape), or an IE expression(), is dropped whole.
func cleanCSS(v string) string {
	v = unescapeCSS(v)
	if strings.Contains(v, `\`) || cssExpression.MatchString(v) {
		return ""
	}
	v = cssImport.ReplaceAllString(v, "")
	v = cssImageSet.ReplaceAllString(v, "none")
	return cssURL.ReplaceAllStringFunc(v, func(m string) string {
		target := strings.ToLower(cssURL.FindStringSubmatch(m)[1])
		if strings.HasPrefix(target, "#") || strings.HasPrefix(target, "data:image/") {
			return m
		}
		return "none"
	})
}

// sanitizeSVG re-serialises an SVG keeping only allow-listed SVG elements and safe attributes.
// Anything that does not parse as well-formed XML is rejected rather than repaired.
func sanitizeSVG(in []byte) ([]byte, error) {
	entities, err := svgEntities(in)
	if err != nil {
		return nil, err
	}
	d := xml.NewDecoder(bytes.NewReader(in))
	d.Strict = true
	d.Entity = entities

	var out bytes.Buffer
	var stack []xml.Name
	skip := 0
	sawRoot := false
	maxOut := 2*len(in) + 4096

	for {
		tok, err := d.RawToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errInvalidSVG
		}
		switch t := tok.(type) {
		case xml.StartElement:
			stack = append(stack, t.Name)
			if len(stack) > maxSVGDepth {
				return nil, errInvalidSVG
			}
			if !sawRoot {
				if t.Name.Space != "" || t.Name.Local != "svg" {
					return nil, errInvalidSVG
				}
				sawRoot = true
			} else if len(stack) == 1 {
				return nil, errInvalidSVG
			}
			if skip > 0 || t.Name.Space != "" || !svgAllowedElements[t.Name.Local] {
				skip++
				continue
			}
			out.WriteByte('<')
			out.WriteString(t.Name.Local)
			for _, a := range t.Attr {
				if v, ok := sanitizeSVGAttr(a); ok {
					out.WriteByte(' ')
					if a.Name.Space != "" {
						out.WriteString(a.Name.Space)
						out.WriteByte(':')
					}
					out.WriteString(a.Name.Local)
					out.WriteString(`="`)
					_ = xml.EscapeText(&out, []byte(v))
					out.WriteByte('"')
				}
			}
			out.WriteByte('>')
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1] != t.Name {
				return nil, errInvalidSVG
			}
			stack = stack[:len(stack)-1]
			if skip > 0 {
				skip--
				continue
			}
			out.WriteString("</")
			out.WriteString(t.Name.Local)
			out.WriteByte('>')
		case xml.CharData:
			if skip > 0 || len(stack) == 0 {
				continue
			}
			text := []byte(t)
			if stack[len(stack)-1].Local == "style" {
				text = []byte(cleanCSS(string(text)))
			}
			_ = xml.EscapeText(&out, text)
		}
		if out.Len() > maxOut {
			return nil, errInvalidSVG
		}
	}
	if !sawRoot || len(stack) != 0 {
		return nil, errInvalidSVG
	}
	return out.Bytes(), nil
}

func sanitizeSVGAttr(a xml.Attr) (string, bool) {
	local := a.Name.Local
	switch a.Name.Space {
	case "":
		switch {
		case local == "xmlns":
			return a.Value, a.Value == svgNamespace
		case strings.HasPrefix(strings.ToLower(local), "on"):
			return "", false
		case local == "href":
			return a.Value, safeSVGHref(a.Value)
		case strings.ContainsAny(a.Value, `\(@`):
			return cleanCSS(a.Value), true
		}
		return a.Value, true
	case "xmlns":
		return a.Value, local == "xlink" && a.Value == xlinkNamespace
	case "xlink":
		if local == "href" {
			return a.Value, safeSVGHref(a.Value)
		}
		return a.Value, true
	case "xml":
		return a.Value, local != "base"
	}
	return "", false
}

// safeSVGHref allows in-document fragments and embedded raster images only.
func safeSVGHref(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	if strings.HasPrefix(v, "#") {
		return true
	}
	for _, p := range []string{"data:image/png", "data:image/jpeg", "data:image/jpg", "data:image/gif", "data:image/webp"} {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}
