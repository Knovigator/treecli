package api

import (
	"strings"
	"testing"
)

func TestPrettyJSONRedactsSensitiveFieldsRecursively(t *testing.T) {
	raw := []byte(`{
		"answers": [
			{
				"user": {
					"id": "user-1",
					"name": "Alice",
					"email": "alice@example.test",
					"user_email": "alice.alias@example.test",
					"emailAddress": "alice.camel@example.test"
				},
				"content": "Contact me at public@example.test"
			}
		],
		"meta": {
			"access-token": "secret-token",
			"uid": "current@example.test"
		}
	}`)

	prettyJSON, err := PrettyJSON(raw)
	if err != nil {
		t.Fatalf("PrettyJSON returned error: %v", err)
	}

	for _, leakedValue := range []string{
		"alice@example.test",
		"alice.alias@example.test",
		"alice.camel@example.test",
		"secret-token",
		"current@example.test",
	} {
		if strings.Contains(prettyJSON, leakedValue) {
			t.Fatalf("expected %q to be redacted from %s", leakedValue, prettyJSON)
		}
	}
	if !strings.Contains(prettyJSON, "Contact me at public@example.test") {
		t.Fatalf("expected non-identity content to be preserved, got %s", prettyJSON)
	}
	if !strings.Contains(prettyJSON, redactedValue) {
		t.Fatalf("expected redacted marker in %s", prettyJSON)
	}
}

func TestSafeResponseBodyRedactsJSONFieldsAndStringEmails(t *testing.T) {
	body := SafeResponseBody([]byte(`{
		"error": "failed for alice@example.test",
		"email": "bob@example.test",
		"reset_password_token": "secret-reset-token"
	}`))

	for _, leakedValue := range []string{
		"alice@example.test",
		"bob@example.test",
		"secret-reset-token",
	} {
		if strings.Contains(body, leakedValue) {
			t.Fatalf("expected %q to be redacted from %s", leakedValue, body)
		}
	}
}

func TestSafeResponseBodyRedactsPlainTextEmails(t *testing.T) {
	body := SafeResponseBody([]byte("failed for alice@example.test"))

	if strings.Contains(body, "alice@example.test") {
		t.Fatalf("expected email to be redacted from %s", body)
	}
	if !strings.Contains(body, redactedValue) {
		t.Fatalf("expected redacted marker in %s", body)
	}
}
