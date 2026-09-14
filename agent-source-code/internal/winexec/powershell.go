// Package winexec centralises how the agent shells out to Windows PowerShell.
package winexec

import "bytes"

// utf8Preamble forces stdout to UTF-8 for the rest of the script.
//
// Windows PowerShell 5.1 encodes redirected stdout using
// [Console]::OutputEncoding, which defaults to the console's OEM code page
// (CP850 in much of western Europe, CP866 in Russia). Anything outside ASCII
// then reaches the agent as bytes that are not valid UTF-8, and Go's JSON
// decoder rewrites each one as U+FFFD, so package names arrive as "Mise � jour".
//
// The BOM-less UTF8Encoding matters. [System.Text.Encoding]::UTF8, which the
// winget collectors used before, is an encoding whose GetPreamble() returns
// three bytes; UTF8Encoding($false) returns none. Whether the host actually
// writes that preamble to a redirected stream varies by runtime, so the
// collectors that parse the output as JSON take the BOM-less encoding and go
// through TrimBOM as well.
//
// The assignment is wrapped because the agent runs as a service in Session 0,
// where no console is attached. Setting the encoding on a redirected stdout is
// safe there, but a host configuration that makes it throw must degrade to
// mojibake rather than losing the collector entirely.
const utf8Preamble = "try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding $false } catch {}\n"

var bom = []byte{0xEF, 0xBB, 0xBF}

// Script prepends the UTF-8 preamble to a PowerShell script body. Every
// PowerShell invocation in the agent must go through this, whether or not the
// script currently emits non-ASCII: Windows Update titles, publisher names and
// install paths are all localised.
func Script(body string) string {
	return utf8Preamble + body
}

// TrimBOM drops a leading byte-order mark. PowerShell can still emit one
// depending on host configuration, and it is not valid JSON.
func TrimBOM(b []byte) []byte {
	return bytes.TrimPrefix(b, bom)
}
