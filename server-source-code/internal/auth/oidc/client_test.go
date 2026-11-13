package oidc

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestResolveProviderPictureFetchesMicrosoftGraphPhoto(t *testing.T) {
	// Not parallel: mutates package-level microsoftGraphPhotoURL.
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

func TestResolveProviderPictureFallsBackToRenderableClaimWhenGraphFails(t *testing.T) {
	// Not parallel: mutates package-level microsoftGraphPhotoURL.
	// Graph endpoint returns 500 — we should fall back to the raw claim when it's a
	// non-Graph renderable URL (e.g. hybrid setups where the admin populated `picture`
	// with an external CDN).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	originalURL := microsoftGraphPhotoURL
	microsoftGraphPhotoURL = server.URL
	t.Cleanup(func() {
		microsoftGraphPhotoURL = originalURL
	})

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())

	tests := []struct {
		name    string
		picture string
		want    string
	}{
		{name: "external https", picture: "https://example.com/avatar.png", want: "https://example.com/avatar.png"},
		{name: "data url", picture: "data:image/png;base64,AAAA", want: "data:image/png;base64,AAAA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := resolveProviderPicture(
				ctx,
				"https://login.microsoftonline.com/tenant-id/v2.0",
				&oauth2.Token{AccessToken: "access-token"},
				tt.picture,
			)
			if got != tt.want {
				t.Fatalf("expected picture %q, got %q", tt.want, got)
			}
		})
	}
}

func TestResolveProviderPictureDiscardsGraphUrlClaim(t *testing.T) {
	// Not parallel: mutates package-level microsoftGraphPhotoURL.
	// The picture claim is set to a Graph API URL that the browser cannot render.
	// When Graph also fails, we must NOT fall back to the Graph URL — it'd break
	// the <img> tag. Expect empty string (UI falls back to initials).
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	originalURL := microsoftGraphPhotoURL
	microsoftGraphPhotoURL = server.URL
	t.Cleanup(func() {
		microsoftGraphPhotoURL = originalURL
	})

	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, server.Client())
	got, _ := resolveProviderPicture(
		ctx,
		"https://login.microsoftonline.com/tenant-id/v2.0",
		&oauth2.Token{AccessToken: "access-token"},
		"https://graph.microsoft.com/v1.0/me/photo/$value",
	)
	if got != "" {
		t.Fatalf("expected empty picture for unusable Graph URL claim, got %q", got)
	}
}

func TestResolveProviderPicturePrefersGraphOverClaim(t *testing.T) {
	// Not parallel: mutates package-level microsoftGraphPhotoURL.
	// Even when the picture claim is a renderable external URL, Microsoft Graph
	// is authoritative for the signed-in user's profile photo. Prefer Graph.
	photoBody := []byte("fake-jpeg-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
		"https://example.com/avatar.png",
	)
	if err != nil {
		t.Fatalf("resolveProviderPicture() error = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/jpeg;base64,") {
		t.Fatalf("expected data URL from Graph, got %q", got)
	}
}

func TestResolveProviderPictureGraphUrlNoFallback(t *testing.T) {
	t.Parallel()

	if !isMicrosoftGraphURL("https://graph.microsoft.com/v1.0/me/photo/$value") {
		t.Fatal("expected Graph URL detector to match /me/photo/$value")
	}
	if isMicrosoftGraphURL("https://example.com/avatar.png") {
		t.Fatal("expected non-Graph URL to not match detector")
	}
}

func TestResolveProviderPictureBoundsGraphFetchWithoutCancelingLoginContext(t *testing.T) {
	// Not parallel: mutates package-level microsoftGraphPhotoURL.
	requestCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(requestCanceled)
	}))
	defer server.Close()

	originalURL := microsoftGraphPhotoURL
	microsoftGraphPhotoURL = server.URL
	t.Cleanup(func() {
		microsoftGraphPhotoURL = originalURL
	})

	parent, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx := context.WithValue(parent, oauth2.HTTPClient, server.Client())
	got, err := resolveProviderPicture(
		ctx,
		"https://login.microsoftonline.com/tenant-id/v2.0",
		&oauth2.Token{AccessToken: "access-token"},
		"https://graph.microsoft.com/v1.0/me/photo/$value",
	)
	if got != "" {
		t.Fatalf("expected no photo after Graph timeout, got %q", got)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected Graph request deadline error, got %v", err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(time.Second):
		t.Fatal("Graph request was not canceled when its photo deadline expired")
	}
	if err := parent.Err(); err != nil {
		t.Fatalf("photo timeout canceled the login context: %v", err)
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
