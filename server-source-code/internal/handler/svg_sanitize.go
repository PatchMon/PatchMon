package handler

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
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

var (
	svgEntityDecl = regexp.MustCompile(`<!ENTITY\s+([A-Za-z_][\w.-]*)\s+"([^"%&<]{0,256})"\s*>`)
	cssImport     = regexp.MustCompile(`(?i)@import\s+[^;]+;?`)
	errInvalidSVG = errors.New("invalid SVG")
)

// sanitizeSVG re-serialises an SVG keeping only allow-listed SVG elements and safe attributes.
// Anything that does not parse as well-formed XML is rejected rather than repaired.
func sanitizeSVG(in []byte) ([]byte, error) {
	d := xml.NewDecoder(bytes.NewReader(in))
	d.Strict = true
	d.Entity = map[string]string{}

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
				text = cssImport.ReplaceAll(text, nil)
			}
			_ = xml.EscapeText(&out, text)
		case xml.Directive:
			for _, m := range svgEntityDecl.FindAllSubmatch(t, -1) {
				d.Entity[string(m[1])] = string(m[2])
			}
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
		case local == "style":
			return cssImport.ReplaceAllString(a.Value, ""), true
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
		return a.Value, true
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
