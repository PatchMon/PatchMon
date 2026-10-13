//go:build windows

package packages

import "golang.org/x/sys/windows/registry"

func pendingFileRenameValues() [][]string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager`, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer func() { _ = k.Close() }()
	var out [][]string
	for _, name := range []string{"PendingFileRenameOperations", "PendingFileRenameOperations2"} {
		if v, _, err := k.GetStringsValue(name); err == nil {
			out = append(out, v)
		}
	}
	return out
}
