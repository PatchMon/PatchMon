package handler

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestConvertDockerPayloadCarriesCollected(t *testing.T) {
	var p dockerPayloadReq
	body := `{"containers":[],"images":[],"updates":[],"collected":["containers","images","volumes","networks"]}`
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	got := convertDockerPayloadToStore(&p).Collected
	if want := []string{"containers", "images", "volumes", "networks"}; !slices.Equal(got, want) {
		t.Fatalf("Collected = %v, want %v", got, want)
	}

	var legacy dockerPayloadReq
	if err := json.Unmarshal([]byte(`{"containers":[],"images":[],"updates":[]}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if got := convertDockerPayloadToStore(&legacy).Collected; len(got) != 0 {
		t.Fatalf("legacy payload Collected = %v, want empty", got)
	}
}
