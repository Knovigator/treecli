package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMCPCallRefreshesOnceThenGivesUp(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	refreshes := 0
	SetOAuthRefresher(func(stale string) (string, error) {
		refreshes++
		return "fresh", nil
	})
	defer SetOAuthRefresher(nil)

	client, err := NewTokenClient(server.URL, "stale", OAuthClient, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CallTool(context.Background(), "treechat_account", nil); err == nil {
		t.Fatal("expected an error after the refreshed token is also rejected")
	}
	if requests != 2 || refreshes != 1 {
		t.Fatalf("expected one retry after one refresh, got %d requests and %d refreshes", requests, refreshes)
	}
}

func TestOAuthMediaDownloadsSendNoCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("access-token") != "" {
			t.Error("credential sent with a media download")
		}
		w.Write([]byte("media"))
	}))
	defer server.Close()
	data, err := DownloadMedia(server.URL+"/rails/active_storage/blobs/x/out.png", server.URL, "expired-bearer", OAuthClient, "user-1")
	if err != nil || string(data) != "media" {
		t.Fatalf("download: %q %v", data, err)
	}
}
