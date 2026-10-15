package packages

import "strings"

// withTempDir points TEMP and TMP at dir. winget as SYSTEM otherwise stages installers in
// C:\Windows\Temp, where a local user can pre-create folders and swap an installer.
func withTempDir(environ []string, dir string) []string {
	out := make([]string, 0, len(environ)+2)
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if strings.EqualFold(k, "TEMP") || strings.EqualFold(k, "TMP") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, "TEMP="+dir, "TMP="+dir)
}

func envValue(environ []string, key string) string {
	for _, kv := range environ {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}
