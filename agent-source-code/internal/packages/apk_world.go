package packages

import (
	"fmt"
	"os"
	"strings"
)

// APKWorldFile is where apk keeps the packages and constraints the admin asked for.
const APKWorldFile = "/etc/apk/world"

// APKWorldConstraint is one versioned entry from /etc/apk/world, such as busybox<1.37.0-r10.
type APKWorldConstraint struct {
	Raw     string
	Name    string
	Op      string
	Version string
}

// APKHold describes an upgradable package that apk will not upgrade because of world.
type APKHold struct {
	Name       string
	Installed  string
	Available  string
	Constraint string
}

// ReadAPKWorld returns the versioned constraints in /etc/apk/world. A missing file has none.
func ReadAPKWorld() ([]APKWorldConstraint, error) {
	data, err := readAPKWorldFile()
	return ParseAPKWorld(data), err
}

func readAPKWorldFile() (string, error) {
	data, err := os.ReadFile(APKWorldFile)
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(data), err
}

// ParseAPKWorldNames returns every package world names in any form: plain, tagged,
// versioned or as a conflict (!name).
func ParseAPKWorldNames(content string) map[string]bool {
	names := map[string]bool{}
	for _, entry := range strings.Fields(content) {
		entry = strings.TrimPrefix(entry, "!")
		if i := strings.IndexAny(entry, "@<>=~"); i >= 0 {
			entry = entry[:i]
		}
		if entry != "" {
			names[entry] = true
		}
	}
	return names
}

// ParseAPKWorld returns the entries that pin a version. Plain names, conflicts (!name)
// and checksum pins are not version holds and are left out.
func ParseAPKWorld(content string) []APKWorldConstraint {
	var out []APKWorldConstraint
	for _, entry := range strings.Fields(content) {
		if strings.HasPrefix(entry, "!") {
			continue
		}
		i := strings.IndexAny(entry, "<>=~")
		if i <= 0 {
			continue
		}
		j := i
		for j < len(entry) && strings.ContainsRune("<>=~", rune(entry[j])) {
			j++
		}
		name, op, version := entry[:i], entry[i:j], entry[j:]
		if at := strings.IndexByte(name, '@'); at >= 0 {
			name = name[:at]
		}
		if name == "" || !apkVersionValid(version) || !validAPKWorldOp(op) {
			continue
		}
		out = append(out, APKWorldConstraint{Raw: entry, Name: name, Op: op, Version: version})
	}
	return out
}

func validAPKWorldOp(op string) bool {
	switch op {
	case "=", "<", "<=", ">", ">=", "~", "=~", ">~", "<~":
		return true
	}
	return false
}

// Allows reports whether version satisfies the constraint, using cmp to order versions.
func (c APKWorldConstraint) Allows(version string, cmp func(a, b string) int) bool {
	if !apkVersionValid(version) {
		return false
	}
	if strings.Contains(c.Op, "~") && apkFuzzyMatch(version, c.Version) {
		return true
	}
	r := cmp(version, c.Version)
	switch strings.Trim(c.Op, "~") {
	case "=", "":
		return r == 0
	case "<":
		return r < 0
	case "<=":
		return r <= 0
	case ">":
		return r > 0
	case ">=":
		return r >= 0
	}
	return false
}

func apkFuzzyMatch(version, prefix string) bool {
	if !strings.HasPrefix(version, prefix) {
		return false
	}
	rest := version[len(prefix):]
	return rest == "" || rest[0] < '0' || rest[0] > '9'
}

// String renders a hold for run output, for example
// "busybox=1.37.0-r29 (1.37.0-r30 available)".
func (h APKHold) String() string {
	if h.Available == "" {
		return h.Constraint
	}
	return fmt.Sprintf("%s (%s available)", h.Constraint, h.Available)
}

// A conflict is a constraint the installed version already breaks: apk refuses every
// transaction until it is fixed, whereas it skips a hold quietly.
func apkWorldHolds(world []APKWorldConstraint, installed map[string]string, available map[string]string, cmp func(a, b string) int) (holds, conflicts []APKHold) {
	for _, c := range world {
		cur, ok := installed[c.Name]
		if !ok {
			continue
		}
		h := APKHold{Name: c.Name, Installed: cur, Available: available[c.Name], Constraint: c.Raw}
		switch {
		case !c.Allows(cur, cmp):
			conflicts = append(conflicts, h)
		case h.Available != "" && !c.Allows(h.Available, cmp):
			holds = append(holds, h)
		}
	}
	return holds, conflicts
}
