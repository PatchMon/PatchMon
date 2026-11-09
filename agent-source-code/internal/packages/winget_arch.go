package packages

import (
	"debug/pe"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func wingetDetailsArgs(id string) []string {
	return []string{"list", "--id", id, "--exact", "--source", "winget", "--details", "--accept-source-agreements", "--disable-interactivity"}
}

var arpLocalIdentifier = regexp.MustCompile(`(?m)\bARP\\(Machine|User)\\(X86|X64|Arm64|Arm|Neutral)\\([^\r\n]+)`)

func machineX86ARPKey(detailsOutput string) (string, bool) {
	m := arpLocalIdentifier.FindAllStringSubmatch(detailsOutput, -1)
	if len(m) != 1 || !strings.EqualFold(m[0][1], "Machine") || !strings.EqualFold(m[0][2], "X86") {
		return "", false
	}
	key := strings.TrimSpace(m[0][3])
	return key, key != ""
}

// The 32-bit registry view alone is not enough: 64-bit Microsoft Edge and the x64 VC++
// redistributable both register there.
func x86OnlyBinaries(dir string) bool {
	if dir == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	found := false
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
			continue
		}
		machine, ok := peMachine(filepath.Join(dir, e.Name()))
		if !ok {
			continue
		}
		if machine != pe.IMAGE_FILE_MACHINE_I386 {
			return false
		}
		found = true
	}
	return found
}

func peMachine(path string) (uint16, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	var dos [64]byte
	if _, err := io.ReadFull(f, dos[:]); err != nil || dos[0] != 'M' || dos[1] != 'Z' {
		return 0, false
	}
	var hdr [6]byte
	if _, err := f.ReadAt(hdr[:], int64(binary.LittleEndian.Uint32(dos[0x3c:]))); err != nil || string(hdr[:4]) != "PE\x00\x00" {
		return 0, false
	}
	return binary.LittleEndian.Uint16(hdr[4:]), true
}
