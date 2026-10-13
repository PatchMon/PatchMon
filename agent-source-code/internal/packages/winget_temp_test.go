package packages

import (
	"reflect"
	"testing"
)

func TestWithTempDirReplacesTempAndTmp(t *testing.T) {
	got := withTempDir([]string{`TEMP=C:\Windows\Temp`, "A=1", `tmp=C:\Windows\Temp`, `Temp=x`}, `C:\Windows\SystemTemp`)
	want := []string{"A=1", `TEMP=C:\Windows\SystemTemp`, `TMP=C:\Windows\SystemTemp`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEnvValue(t *testing.T) {
	env := withTempDir([]string{"A=1", `temp=C:\old`}, `C:\Windows\SystemTemp`)
	if got := envValue(env, "TEMP"); got != `C:\Windows\SystemTemp` {
		t.Errorf("TEMP = %q", got)
	}
	if got := envValue(env, "MISSING"); got != "" {
		t.Errorf("missing = %q", got)
	}
}
