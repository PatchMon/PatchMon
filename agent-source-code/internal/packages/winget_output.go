package packages

import (
	"regexp"
	"strings"
)

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07\x1b]*(\x07|\x1b\\)`)
	ansiESC = regexp.MustCompile(`\x1b[@-Z\\-_]`)
)

func stripTerminalControls(s string) string {
	s = ansiOSC.ReplaceAllString(s, "")
	s = ansiCSI.ReplaceAllString(s, "")
	s = ansiESC.ReplaceAllString(s, "")
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, line := range lines {
		if i := strings.LastIndex(strings.TrimRight(line, "\r"), "\r"); i >= 0 {
			line = line[i+1:]
		}
		line = strings.Map(func(r rune) rune {
			if r == '\t' {
				return r
			}
			if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
				return -1
			}
			return r
		}, line)
		if isSpinnerLine(line) {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func isSpinnerLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if strings.Trim(t, `-\|/ `) == "" {
		return true
	}
	r := []rune(t)[0]
	return r >= 0x2580 && r <= 0x259f
}
