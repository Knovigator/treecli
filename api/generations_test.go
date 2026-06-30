package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDownloadMediaDoesNotSendTreechatAuthToExternalMediaURL(t *testing.T) {
	receivedHeaders := http.Header{}
	mediaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("image-bytes"))
	}))
	defer mediaServer.Close()

	backendServer := httptest.NewServer(http.NotFoundHandler())
	defer backendServer.Close()

	data, err := DownloadMedia(mediaServer.URL+"/asset.png", backendServer.URL, "secret-token", "client-id", "user@example.test")
	if err != nil {
		t.Fatalf("DownloadMedia returned error: %v", err)
	}
	if string(data) != "image-bytes" {
		t.Fatalf("expected downloaded bytes, got %q", string(data))
	}
	for _, header := range []string{"access-token", "client", "uid"} {
		if got := receivedHeaders.Get(header); got != "" {
			t.Fatalf("expected no %s header for external media URL, got %q", header, got)
		}
	}
}

func TestDownloadMediaSendsTreechatAuthToSameOriginAPIURL(t *testing.T) {
	receivedHeaders := http.Header{}
	backendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("private-bytes"))
	}))
	defer backendServer.Close()

	data, err := DownloadMedia(backendServer.URL+"/api/v1/blob/1", backendServer.URL, "secret-token", "client-id", "user@example.test")
	if err != nil {
		t.Fatalf("DownloadMedia returned error: %v", err)
	}
	if string(data) != "private-bytes" {
		t.Fatalf("expected downloaded bytes, got %q", string(data))
	}
	if got := receivedHeaders.Get("access-token"); got != "secret-token" {
		t.Fatalf("expected access-token header, got %q", got)
	}
	if got := receivedHeaders.Get("client"); got != "client-id" {
		t.Fatalf("expected client header, got %q", got)
	}
	if got := receivedHeaders.Get("uid"); got != "user@example.test" {
		t.Fatalf("expected uid header, got %q", got)
	}
}

func TestCreateGenerationUsesCallerTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(GenerationResponse{ID: "run-1", Status: "succeeded"})
	}))
	defer server.Close()

	startedAt := time.Now()
	_, err := CreateGeneration(
		server.URL,
		"secret-token",
		"client-id",
		"user@example.test",
		"flux",
		"wide hero",
		nil,
		false,
		20*time.Millisecond,
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("expected caller timeout to abort quickly, took %s", elapsed)
	}
}
