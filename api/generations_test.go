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

func TestUploadSharedFileUsesSharedRegistrationAndDirectStorage(t *testing.T) {
	referenceData := []byte("\x89PNG\r\n\x1a\nreference-bytes")
	referencePath := writeTempReferenceFile(t, "frame.png", referenceData)

	seenRegister := false
	seenPut := false
	seenAttach := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		var call struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if path == "/api/v1/treecli/call" {
			if r.Method != http.MethodPost {
				t.Error("RPC must POST")
			}
			if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
				t.Fatal(err)
			}
			path = call.Name
		}
		switch path {
		case "register_upload":
			seenRegister = true
			if got := r.Header.Get("access-token"); got != "secret-token" {
				t.Fatalf("expected auth header on direct-upload registration, got %q", got)
			}
			payload := call.Arguments
			if payload["filename"] != "frame.png" || payload["content_type"] != "image/png" {
				t.Fatalf("unexpected registration payload: %#v", payload)
			}
			if payload["checksum"] != md5Base64(referenceData) {
				t.Fatalf("expected checksum %q, got %#v", md5Base64(referenceData), payload["checksum"])
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"structuredContent": map[string]interface{}{
				"signed_id": "signed-reference",
				"direct_upload": map[string]interface{}{
					"url": server.URL + "/s3/reference.png",
					"headers": map[string]string{
						"Content-Type": "image/png",
						"Content-MD5":  md5Base64(referenceData),
					},
				},
			}})
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
		case "finish_reference_upload":
			seenAttach = true
			if call.Arguments["signed_id"] != "signed-reference" {
				t.Errorf("wrong finish arguments: %#v", call.Arguments)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"structuredContent":{"id":"ref-1","url":"https://cdn.example.test/ref.png","content_type":"image/png","kind":"image"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	out, err := UploadSharedFile(server.URL, "secret-token", "client-id", "user@example.test", referencePath, "reference")
	if err != nil {
		t.Fatalf("UploadReference returned error: %v", err)
	}
	if !seenRegister || !seenPut || !seenAttach {
		t.Fatalf("expected register, PUT, and attach requests; got register=%t put=%t attach=%t", seenRegister, seenPut, seenAttach)
	}
	if out["url"] != "https://cdn.example.test/ref.png" || out["content_type"] != "image/png" || out["kind"] != "image" {
		t.Fatalf("unexpected response: %#v", out)
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
