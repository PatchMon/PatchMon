package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDockerDataCollectedWireName(t *testing.T) {
	b, err := json.Marshal(DockerData{Collected: []string{"containers"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"collected":["containers"]`) {
		t.Fatalf("got %s", b)
	}
}
