package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		"",
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
		"stripe_metered",
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
	if actionRequest["payment_mode"] != "stripe_metered" {
		t.Fatalf("expected action_request payment_mode, got %#v", actionRequest["payment_mode"])
	}
	settings, ok := actionRequest["settings"].(map[string]interface{})
	if !ok || settings["reference_url"] != "https://cdn.example.test/frame.png" {
		t.Fatalf("expected action_request settings, got %#v", actionRequest["settings"])
	}
	if received["action_key"] != "kling3" {
		t.Fatalf("expected top-level action_key field, got %#v", received["action_key"])
	}
	if received["payment_mode"] != "stripe_metered" {
		t.Fatalf("expected top-level payment_mode field, got %#v", received["payment_mode"])
	}
	if _, ok := received["action"]; ok {
		t.Fatalf("expected no top-level action field because Rails reserves params[:action], got %#v", received["action"])
	}
}

func TestUploadReferenceUsesDirectUploadEndpoint(t *testing.T) {
	referenceData := []byte("\x89PNG\r\n\x1a\nreference-bytes")
	referencePath := writeTempReferenceFile(t, "frame.png", referenceData)

	seenRegister := false
	seenPut := false
	seenAttach := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ai/generations/references/direct_upload":
			seenRegister = true
			if got := r.Header.Get("access-token"); got != "secret-token" {
				t.Fatalf("expected auth header on direct-upload registration, got %q", got)
			}
			var payload map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decoding direct-upload registration: %v", err)
			}
			if payload["filename"] != "frame.png" || payload["content_type"] != "image/png" {
				t.Fatalf("unexpected registration payload: %#v", payload)
			}
			if payload["checksum"] != md5Base64(referenceData) {
				t.Fatalf("expected checksum %q, got %#v", md5Base64(referenceData), payload["checksum"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"signed_id": "signed-reference",
				"direct_upload": map[string]interface{}{
					"url": server.URL + "/s3/reference.png",
					"headers": map[string]string{
						"Content-Type": "image/png",
						"Content-MD5":  md5Base64(referenceData),
					},
				},
			})
		case "/s3/reference.png":
			seenPut = true
			for _, header := range []string{"access-token", "client", "uid"} {
				if got := r.Header.Get(header); got != "" {
					t.Fatalf("expected no Treechat auth header %s on direct PUT, got %q", header, got)
				}
			}
			if got := r.Header.Get("Content-Type"); got != "image/png" {
				t.Fatalf("expected PUT Content-Type image/png, got %q", got)
			}
			if got := r.Header.Get("Content-MD5"); got != md5Base64(referenceData) {
				t.Fatalf("expected PUT Content-MD5 %q, got %q", md5Base64(referenceData), got)
			}
			if got := r.ContentLength; got != int64(len(referenceData)) {
				t.Fatalf("expected PUT Content-Length %d, got %d", len(referenceData), got)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading PUT body: %v", err)
			}
			if !bytes.Equal(body, referenceData) {
				t.Fatalf("unexpected PUT body %q", string(body))
			}
			w.WriteHeader(http.StatusOK)
		case "/api/v1/ai/generations/references":
			seenAttach = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parsing signed_id attach form: %v", err)
			}
			if got := r.FormValue("signed_id"); got != "signed-reference" {
				t.Fatalf("expected signed_id attach, got %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ref-1","url":"https://cdn.example.test/ref.png","content_type":"image/png","kind":"image"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	out, err := UploadReference(server.URL, "secret-token", "client-id", "user@example.test", referencePath)
	if err != nil {
		t.Fatalf("UploadReference returned error: %v", err)
	}
	if !seenRegister || !seenPut || !seenAttach {
		t.Fatalf("expected register, PUT, and attach requests; got register=%t put=%t attach=%t", seenRegister, seenPut, seenAttach)
	}
	if out.URL != "https://cdn.example.test/ref.png" || out.ContentType != "image/png" || out.Kind != "image" {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestUploadReferenceFallsBackToMultipartWhenDirectUploadUnavailable(t *testing.T) {
	referenceData := []byte("\x89PNG\r\n\x1a\nreference-bytes")
	referencePath := writeTempReferenceFile(t, "frame.png", referenceData)

	seenDirectUpload := false
	seenMultipart := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/ai/generations/references/direct_upload":
			seenDirectUpload = true
			http.NotFound(w, r)
		case "/api/v1/ai/generations/references":
			seenMultipart = true
			if err := r.ParseMultipartForm(2 << 20); err != nil {
				t.Fatalf("parsing multipart fallback: %v", err)
			}
			file, header, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("expected multipart file fallback: %v", err)
			}
			defer file.Close()
			if header.Filename != "frame.png" {
				t.Fatalf("expected fallback filename frame.png, got %q", header.Filename)
			}
			if got := header.Header.Get("Content-Type"); got != "image/png" {
				t.Fatalf("expected image/png fallback content type, got %q", got)
			}
			body, err := io.ReadAll(file)
			if err != nil {
				t.Fatalf("reading fallback file: %v", err)
			}
			if !bytes.Equal(body, referenceData) {
				t.Fatalf("unexpected fallback file body %q", string(body))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"ref-1","url":"https://cdn.example.test/ref.png","content_type":"image/png","kind":"image"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	out, err := UploadReference(server.URL, "secret-token", "client-id", "user@example.test", referencePath)
	if err != nil {
		t.Fatalf("UploadReference returned error: %v", err)
	}
	if !seenDirectUpload || !seenMultipart {
		t.Fatalf("expected direct-upload attempt and multipart fallback; got direct=%t multipart=%t", seenDirectUpload, seenMultipart)
	}
	if out.URL != "https://cdn.example.test/ref.png" {
		t.Fatalf("unexpected response: %#v", out)
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
		_, _ = w.Write([]byte(`{"actions":[{"action":"qwen","provider":"replicate","kind":"image","accepts_reference":true,"reference_kinds":["image"]}]}`))
	}))
	defer server.Close()

	actions, err := ListGenerationActions(server.URL, "secret-token", "client-id", "user@example.test")
	if err != nil {
		t.Fatalf("ListGenerationActions returned error: %v", err)
	}
	if len(actions) != 1 || actions[0].Action != "qwen" || actions[0].Tag != "qwen" {
		t.Fatalf("unexpected actions: %#v", actions)
	}
	if !actions[0].AcceptsReference || actions[0].RequiresReference || len(actions[0].ReferenceKinds) != 1 || actions[0].ReferenceKinds[0] != "image" {
		t.Fatalf("expected optional image reference metadata, got %#v", actions[0])
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

func writeTempReferenceFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("writing temp reference file: %v", err)
	}
	return path
}
