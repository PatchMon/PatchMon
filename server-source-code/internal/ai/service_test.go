package ai

import "testing"

// TestGetProvidersIncludesOrcaRouter guards that the OrcaRouter named provider
// stays exposed through the providers endpoint used by the AI settings page.
func TestGetProvidersIncludesOrcaRouter(t *testing.T) {
	s := &Service{}
	got := s.GetProviders()
	for _, p := range got {
		if p.ID == "orcarouter" {
			if p.DefaultModel != "orcarouter/fusion" {
				t.Fatalf("orcarouter default model = %q, want %q", p.DefaultModel, "orcarouter/fusion")
			}
			if len(p.Models) == 0 {
				t.Fatal("orcarouter provider has no models")
			}
			return
		}
	}
	t.Fatal("orcarouter provider not found in GetProviders()")
}
