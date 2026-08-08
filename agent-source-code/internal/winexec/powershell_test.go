package winexec

import (
	"strings"
	"testing"
)

func TestScriptSetsBomlessUTF8Output(t *testing.T) {
	got := Script("Get-Service\n")
	if !strings.HasPrefix(got, "try { [Console]::OutputEncoding") {
		t.Fatalf("preamble must come first, got: %q", got)
	}
	// [System.Text.Encoding]::UTF8 writes a BOM, which breaks json.Unmarshal
	// on the collectors that parse ConvertTo-Json output.
	if strings.Contains(got, "[System.Text.Encoding]::UTF8") {
		t.Fatal("preamble uses the BOM-emitting encoding")
	}
	if !strings.Contains(got, "UTF8Encoding $false") {
		t.Fatalf("preamble does not disable the BOM: %q", got)
	}
	if !strings.HasSuffix(got, "Get-Service\n") {
		t.Fatalf("script body not preserved: %q", got)
	}
}

func TestTrimBOM(t *testing.T) {
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`[{"Name":"vim"}]`)...)
	if got := string(TrimBOM(withBOM)); got != `[{"Name":"vim"}]` {
		t.Fatalf("BOM not trimmed: %q", got)
	}
	clean := []byte(`[{"Name":"vim"}]`)
	if got := string(TrimBOM(clean)); got != string(clean) {
		t.Fatalf("clean input altered: %q", got)
	}
	if got := TrimBOM(nil); len(got) != 0 {
		t.Fatalf("nil input produced %q", got)
	}
}
