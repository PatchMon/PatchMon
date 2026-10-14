package handler

import (
	"reflect"
	"testing"
)

func TestSetRunPackageIDsOnlyMapsInventoryNames(t *testing.T) {
	name := "libstdc++6"
	linux := map[string]interface{}{
		"package_name":      &name,
		"package_names":     []string{"libstdc++6", "openssl"},
		"packages_affected": []string{"libstdc++6", "openssl", "libssl3t64"},
	}
	windows := map[string]interface{}{
		"package_name":      (*string)(nil),
		"packages_affected": []string{"2026-10 Security Update (KB5127648) (26300.9711)", "Mozilla Firefox (x64 en-GB)"},
	}
	empty := map[string]interface{}{}
	ids := map[string]string{"libstdc++6": "p1", "openssl": "p2", "Mozilla Firefox (x64 en-GB)": "p3"}

	setRunPackageIDs([]map[string]interface{}{linux, windows, empty}, ids)

	if got, want := linux["package_ids"], map[string]string{"libstdc++6": "p1", "openssl": "p2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("linux package_ids = %v, want %v", got, want)
	}
	if got, want := windows["package_ids"], map[string]string{"Mozilla Firefox (x64 en-GB)": "p3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("windows package_ids = %v, want %v", got, want)
	}
	if got, want := empty["package_ids"], map[string]string{}; !reflect.DeepEqual(got, want) {
		t.Errorf("empty package_ids = %v, want %v", got, want)
	}
}

func TestUniqueRunPackageNames(t *testing.T) {
	name := "libstdc++6"
	runs := []map[string]interface{}{
		{"package_name": &name, "package_names": []string{"libstdc++6", "openssl"}, "packages_affected": []string{"libstdc++6", "openssl", "libssl3t64"}},
		{"packages_affected": []string{"openssl", "Mozilla Firefox (x64 en-GB)"}},
		{},
	}
	got := uniqueRunPackageNames(runs)
	want := []string{"libstdc++6", "openssl", "libssl3t64", "Mozilla Firefox (x64 en-GB)"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
