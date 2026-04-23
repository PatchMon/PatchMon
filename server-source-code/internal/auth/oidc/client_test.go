package oidc

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func TestResolveProviderPictureFetchesMicrosoftGraphPhoto(t *testing.T) {
	t.Parallel()

	photoBody := []byte("fake-jpeg-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("expected bearer token, got %q", got)
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(photoBody)
	}))
	defer server.Close()

	originalURL := microsoftGraphPhotoURL
	microsoftGraphPhotoURL = server.URL
	t.Cleanup(func() {
		microsoftGraphPhotoURL = originalURL
	})

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())
	got, err := resolveProviderPicture(
		ctx,
		"https://login.microsoftonline.com/tenant-id/v2.0",
		&oauth2.Token{AccessToken: "access-token"},
		"c8a1d9e8-6fdd-4427-82df-9b9d269d321b",
	)
	if err != nil {
		t.Fatalf("resolveProviderPicture() error = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
		t.Fatalf("expected data URL, got %q", got)
	}
}

func TestResolveProviderPictureKeepsRenderableSources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		picture string
	}{
		{name: "https url", picture: "https://example.com/avatar.png"},
		{name: "data url", picture: "data:image/png;base64,AAAA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveProviderPicture(
				context.Background(),
				"https://login.microsoftonline.com/tenant-id/v2.0",
				&oauth2.Token{AccessToken: "unused"},
				tt.picture,
			)
			if err != nil {
				t.Fatalf("resolveProviderPicture() error = %v", err)
			}
			if got != tt.picture {
				t.Fatalf("expected picture %q, got %q", tt.picture, got)
			}
		})
	}
}

func TestResolveProviderPictureLeavesNonMicrosoftProvidersUnchanged(t *testing.T) {
	t.Parallel()

	rawPicture := "not-a-real-url"
	got, err := resolveProviderPicture(
		context.Background(),
		"https://auth.example.com/application/o/patchmon/",
		&oauth2.Token{AccessToken: "unused"},
		rawPicture,
	)
	if err != nil {
		t.Fatalf("resolveProviderPicture() error = %v", err)
	}
	if got != rawPicture {
		t.Fatalf("expected raw picture %q, got %q", rawPicture, got)
	}
}
