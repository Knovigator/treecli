package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestClientExplicitCredentialsAndIsolation(t *testing.T) {
	t.Setenv("TREECLI_ACCESS_TOKEN", "must-not-use")
	if _, err := NewOAuthClient("https://example.test", ""); err == nil {
		t.Fatal("accepted absent credential")
	}
	if _, err := NewTokenClient("https://example.test", "", "", ""); err == nil {
		t.Fatal("accepted absent credentials")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			if r.Header.Get("access-token") != "" || r.Header.Get("client") != "" || r.Header.Get("uid") != "" {
				t.Error("mixed auth headers")
			}
			if r.Header.Get("Authorization") != "Bearer "+r.URL.Query().Get("user") {
				t.Error("cross-user auth")
			}
		} else if r.Header.Get("access-token") != "token" || r.Header.Get("client") != "client" || r.Header.Get("uid") != "uid" {
			t.Error("incorrect DTA auth")
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for _, user := range []string{"one", "two"} {
		c, _ := NewOAuthClient(server.URL, user)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if err := c.DoJSON(context.Background(), "GET", "/api/v1/gon", map[string][]string{"user": {user}}, nil, nil); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	c, _ := NewTokenClient(server.URL, "token", "client", "uid")
	if err := c.DoJSON(context.Background(), "GET", "/api/v1/gon", nil, nil, nil); err != nil {
		t.Error(err)
	}
	wg.Wait()
}

func TestClientRejectsPathsAndRedirects(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v1/gon", http.StatusFound)
	}))
	defer server.Close()
	c, _ := NewOAuthClient(server.URL, "token")
	for _, path := range []string{target.URL + "/api/v1/gon", "//example.test/api/v1/gon", "/auth", "/api/v1/../auth", "/api/v1/%2e%2e/auth", "/api/v1/gon?q=1", "/api/v1/gon#x", "/api/v1/\\evil"} {
		if err := c.DoJSON(context.Background(), "GET", path, nil, nil, nil); err == nil {
			t.Errorf("accepted %q", path)
		}
	}
	if err := c.DoJSON(context.Background(), "GET", "/api/v1/gon", nil, nil, nil); err == nil {
		t.Fatal("followed redirect")
	}
	if hits.Load() != 0 {
		t.Fatal("redirect leaked request")
	}
}

func TestClientOriginValidation(t *testing.T) {
	for _, origin := range []string{"http://example.test", "https://user:pass@example.test", "https://example.test/path", "https://example.test?x=1", "//example.test"} {
		if _, err := NewOAuthClient(origin, "token"); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
}
