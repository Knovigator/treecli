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

func TestCreateGenerationSendsActionRequestPayload(t *testing.T) {
	var received map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/ai/generations" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(GenerationResponse{ID: "run-1", Status: "submitted"})
	}))
	defer server.Close()

	_, err := CreateGeneration(
		server.URL,
		"secret-token",
		"client-id",
		"user@example.test",
		"kling3",
		"animate this",
		map[string]interface{}{"reference_url": "https://cdn.example.test/frame.png"},
		false,
		time.Second,
	)
	if err != nil {
		t.Fatalf("CreateGeneration returned error: %v", err)
	}

	actionRequest, ok := received["action_request"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected action_request payload, got %#v", received)
	}
	if actionRequest["kind"] != "model" || actionRequest["action"] != "kling3" || actionRequest["prompt"] != "animate this" {
		t.Fatalf("unexpected action_request: %#v", actionRequest)
	}
	if _, ok := actionRequest["tag"]; ok {
		t.Fatalf("expected action_request to use action instead of legacy tag, got %#v", actionRequest)
	}
	settings, ok := actionRequest["settings"].(map[string]interface{})
	if !ok || settings["reference_url"] != "https://cdn.example.test/frame.png" {
		t.Fatalf("expected action_request settings, got %#v", actionRequest["settings"])
	}
	if received["action_key"] != "kling3" {
		t.Fatalf("expected top-level action_key field, got %#v", received["action_key"])
	}
	if _, ok := received["action"]; ok {
		t.Fatalf("expected no top-level action field because Rails reserves params[:action], got %#v", received["action"])
	}
}

func TestListGenerationActionsPrefersActionsEndpoint(t *testing.T) {
	var seenPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.Path)
		if r.URL.Path != "/api/v1/ai/generations/actions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"actions":[{"action":"flux2","provider":"replicate","kind":"image"}]}`))
	}))
	defer server.Close()

	actions, err := ListGenerationActions(server.URL, "secret-token", "client-id", "user@example.test")
	if err != nil {
		t.Fatalf("ListGenerationActions returned error: %v", err)
	}
	if len(actions) != 1 || actions[0].Action != "flux2" || actions[0].Tag != "flux2" {
		t.Fatalf("unexpected actions: %#v", actions)
	}
	if len(seenPaths) != 1 {
		t.Fatalf("expected one request to the actions endpoint, got %#v", seenPaths)
	}
}

func TestListGenerationActionsFallsBackToLegacyTagsEndpoint(t *testing.T) {
	var seenPaths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPaths = append(seenPaths, r.URL.Path)
		switch r.URL.Path {
		case "/api/v1/ai/generations/actions":
			http.NotFound(w, r)
		case "/api/v1/ai/generations/tags":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tags":[{"tag":"suno","provider":"suno","kind":"audio"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	actions, err := ListGenerationActions(server.URL, "secret-token", "client-id", "user@example.test")
	if err != nil {
		t.Fatalf("ListGenerationActions returned error: %v", err)
	}
	if len(actions) != 1 || actions[0].Action != "suno" || actions[0].Tag != "suno" {
		t.Fatalf("unexpected actions: %#v", actions)
	}
	if len(seenPaths) != 2 || seenPaths[0] != "/api/v1/ai/generations/actions" || seenPaths[1] != "/api/v1/ai/generations/tags" {
		t.Fatalf("expected actions then tags fallback, got %#v", seenPaths)
	}
}
