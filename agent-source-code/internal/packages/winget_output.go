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

const wingetNoApplicationsFound = 0x8A150014

type wingetExitClass int

const (
	wingetExitOK wingetExitClass = iota
	wingetExitNoApps
	wingetExitPartial
	wingetExitFailed
)

func classifyWingetExit(code int, stdout string) (string, wingetExitClass) {
	switch {
	case code == 0:
		return stripTerminalControls(stdout), wingetExitOK
	case uint32(code) == wingetNoApplicationsFound:
		return "", wingetExitNoApps
	case hasWingetTable(stdout):
		return stripTerminalControls(stdout), wingetExitPartial
	}
	return "", wingetExitFailed
}

// Looks at raw output: stripTerminalControls drops dash-only lines as spinner frames.
func hasWingetTable(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if i := strings.LastIndex(line, "\r"); i >= 0 {
			line = line[i+1:]
		}
		t := strings.TrimSpace(line)
		if len(t) >= 10 && strings.Trim(t, "-") == "" {
			return true
		}
	}
	return false
}
