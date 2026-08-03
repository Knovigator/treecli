package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPerformSignupCreatesAccountAndReturnsAuthTokens(t *testing.T) {
	const (
		username = "tree_user"
		email    = "tree-user@example.test"
		password = "correct horse battery staple"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != signupEndpoint {
			t.Errorf("expected %s, got %s", signupEndpoint, r.URL.Path)
		}
		if contentType := r.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
			t.Errorf("expected JSON content type, got %q", contentType)
		}

		var request signupRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode signup request: %v", err)
		}
		if request.Name != username || request.Email != email || request.Password != password {
			t.Errorf("unexpected signup request: %#v", request)
		}
		if request.PasswordConfirmation != password {
			t.Errorf("expected password confirmation to match password")
		}

		w.Header().Set("access-token", "signup-access-token")
		w.Header().Set("client", "signup-client")
		w.Header().Set("uid", email)
		w.Header().Set("expiry", "4102444800")
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(server.Close)

	tokens, err := performSignup(server.URL, username, email, password)
	if err != nil {
		t.Fatalf("performSignup returned error: %v", err)
	}
	if tokens.AccessToken != "signup-access-token" || tokens.Client != "signup-client" || tokens.UID != email {
		t.Fatalf("unexpected auth tokens: %#v", tokens)
	}
	if tokens.Expiry != "4102444800" {
		t.Fatalf("unexpected expiry: %q", tokens.Expiry)
	}
}

func TestPerformSignupReturnsRedactedBackendError(t *testing.T) {
	const email = "already-used@example.test"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"email": "already-used@example.test",
			"password": "backend-echoed-password",
			"message": "An account for already-used@example.test already exists"
		}`))
	}))
	t.Cleanup(server.Close)

	_, err := performSignup(server.URL, "tree_user", email, "backend-echoed-password")
	if err == nil {
		t.Fatal("expected signup error")
	}
	if !strings.Contains(err.Error(), "status 422") {
		t.Fatalf("expected status in error, got %v", err)
	}
	for _, secret := range []string{email, "backend-echoed-password"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("signup error exposed %q: %v", secret, err)
		}
	}
}

func TestSignupCommandSupportsInteractiveAndStdinModes(t *testing.T) {
	if SignupCmd.Use != "signup" {
		t.Fatalf("unexpected signup command use: %q", SignupCmd.Use)
	}

	for _, flagName := range []string{"username", "email", "password-stdin"} {
		if SignupCmd.Flags().Lookup(flagName) == nil {
			t.Errorf("signup command is missing --%s", flagName)
		}
	}
	if SignupCmd.Flags().Lookup("password") != nil {
		t.Fatal("signup command should not accept passwords as command-line arguments")
	}
}
