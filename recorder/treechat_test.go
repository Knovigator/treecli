package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTreechat struct {
	mu      sync.Mutex
	quests  []map[string]string
	answers []map[string]string
	server  *httptest.Server
}

func newFakeTreechat(t *testing.T) *fakeTreechat {
	t.Helper()
	fake := &fakeTreechat{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/quests", func(w http.ResponseWriter, r *http.Request) {
		form, err := parseAnyForm(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		fake.mu.Lock()
		fake.quests = append(fake.quests, form)
		fake.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"quest": map[string]interface{}{"id": form["id"], "quest_url": "https://app.test/quest/" + form["id"], "team_id": form["team_id"]}})
	})
	mux.HandleFunc("/api/v1/answers", func(w http.ResponseWriter, r *http.Request) {
		form, err := parseAnyForm(r)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		fake.mu.Lock()
		fake.answers = append(fake.answers, form)
		fake.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"answer": map[string]interface{}{"id": form["id"], "quest_id": form["quest_id"]}})
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

// parseAnyForm accepts the url-encoded form treecli sends without uploads and
// the multipart form it sends with them.
func parseAnyForm(r *http.Request) (map[string]string, error) {
	form := map[string]string{}
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			return nil, err
		}
		for key := range r.MultipartForm.Value {
			form[key] = r.FormValue(key)
		}
		return form, nil
	}
	if err := r.ParseForm(); err != nil {
		return nil, err
	}
	for key := range r.PostForm {
		form[key] = r.PostFormValue(key)
	}
	return form, nil
}

func newPoster(t *testing.T, fake *fakeTreechat, mode, teamID string) *TreechatPoster {
	t.Helper()
	store, err := NewStore(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatal(err)
	}
	return &TreechatPoster{
		Credentials: Credentials{BackendURL: fake.server.URL, AccessToken: "token", Client: "client", UID: "uid", SpaceID: "space-1"},
		Store:       store,
		TeamID:      teamID,
		Mode:        mode,
		Now:         func() time.Time { return time.Date(2026, 9, 9, 16, 0, 0, 0, time.UTC) },
	}
}

func TestTreechatPosterSessionModeOpensAndClosesOnce(t *testing.T) {
	fake := newFakeTreechat(t)
	poster := newPoster(t, fake, TreechatModeSession, "team-1")
	session := sampleSession()
	ctx := context.Background()
	state, err := poster.SyncSession(ctx, session, false)
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if len(fake.quests) != 1 || len(fake.answers) != 0 {
		t.Fatalf("expected one thread and no replies, got %d/%d", len(fake.quests), len(fake.answers))
	}
	quest := fake.quests[0]
	if quest["team_id"] != "team-1" || quest["private"] != "" || quest["space_id"] != "space-1" {
		t.Fatalf("thread must target the team: %v", quest)
	}
	if quest["id"] != DeterministicID("session-thread", "claude-code", "sess-1") || quest["parent_attributes[id]"] != DeterministicID("session-root", "claude-code", "sess-1") {
		t.Fatalf("thread ids must be deterministic: %v", quest)
	}
	content := quest["parent_attributes[content]"]
	if !strings.Contains(content, "Claude Code session") || !strings.Contains(content, "First prompt:") || !strings.Contains(content, "Decision: record sessions into memdb.") {
		t.Fatalf("unexpected opening post:\n%s", content)
	}
	if state.QuestURL != "https://app.test/quest/"+quest["id"] {
		t.Fatalf("state must keep the thread url: %+v", state)
	}
	if _, err := poster.SyncSession(ctx, session, false); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if len(fake.quests) != 1 {
		t.Fatal("re-syncing must not create another thread")
	}
	if _, err := poster.SyncSession(ctx, session, true); err != nil {
		t.Fatalf("closing sync: %v", err)
	}
	if _, err := poster.SyncSession(ctx, session, true); err != nil {
		t.Fatalf("repeat closing sync: %v", err)
	}
	if len(fake.answers) != 1 || !strings.Contains(fake.answers[0]["content"], "Session ended · 1 turns · 1 tool calls") {
		t.Fatalf("expected exactly one closing post, got %v", fake.answers)
	}
	if fake.answers[0]["id"] != DeterministicID("closing", "claude-code", "sess-1") {
		t.Fatalf("closing post id must be deterministic: %v", fake.answers[0])
	}
}

func TestTreechatPosterTurnsModePostsEachFinishedTurn(t *testing.T) {
	fake := newFakeTreechat(t)
	poster := newPoster(t, fake, TreechatModeTurns, "")
	base := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	session := &Session{Agent: AgentCodex, SessionID: "c-1", CWD: "/w", StartedAt: base, Messages: []Message{
		{ID: "u1", Role: RoleUser, Text: "first", Timestamp: base},
		{ID: "a1", Role: RoleAssistant, Text: "first reply", Timestamp: base.Add(time.Minute)},
		{ID: "u2", Role: RoleUser, Text: "second", Timestamp: base.Add(2 * time.Minute)},
	}}
	ctx := context.Background()
	if _, err := poster.SyncSession(ctx, session, false); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if fake.quests[0]["private"] != "true" || fake.quests[0]["team_id"] != "" {
		t.Fatalf("without a team the thread must be private: %v", fake.quests[0])
	}
	if len(fake.answers) != 1 || !strings.Contains(fake.answers[0]["content"], "Turn 1") || !strings.Contains(fake.answers[0]["content"], "first reply") {
		t.Fatalf("only the finished turn should post, got %v", fake.answers)
	}
	session.Messages = append(session.Messages, Message{ID: "a2", Role: RoleAssistant, Text: "second reply", Timestamp: base.Add(3 * time.Minute)})
	if _, err := poster.SyncSession(ctx, session, false); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	if len(fake.answers) != 2 || !strings.Contains(fake.answers[1]["content"], "Turn 2") {
		t.Fatalf("second turn should post once finished, got %v", fake.answers)
	}
	if fake.answers[0]["id"] != DeterministicID("turn", "codex", "c-1", "0") || fake.answers[1]["id"] != DeterministicID("turn", "codex", "c-1", "1") {
		t.Fatalf("turn ids must be deterministic: %v", fake.answers)
	}
	if _, err := poster.SyncSession(ctx, session, false); err != nil {
		t.Fatalf("sync 3: %v", err)
	}
	if len(fake.answers) != 2 {
		t.Fatal("nothing new means no new posts")
	}
}

func TestTreechatPosterNotesAreIdempotent(t *testing.T) {
	fake := newFakeTreechat(t)
	poster := newPoster(t, fake, TreechatModeSession, "team-1")
	session := sampleSession()
	ctx := context.Background()
	_, url, err := poster.PostNote(ctx, session, "note-a", "remember this")
	if err != nil {
		t.Fatalf("note: %v", err)
	}
	if !strings.HasPrefix(url, "https://app.test/quest/") {
		t.Fatalf("expected thread url, got %q", url)
	}
	if _, _, err := poster.PostNote(ctx, session, "note-a", "remember this"); err != nil {
		t.Fatalf("repeat note: %v", err)
	}
	if len(fake.quests) != 1 || len(fake.answers) != 1 || fake.answers[0]["content"] != "remember this" {
		t.Fatalf("expected one thread and one note, got %d/%d", len(fake.quests), len(fake.answers))
	}
	if _, _, err := poster.PostNote(ctx, session, "note-b", fmt.Sprintf("%s", strings.Repeat("x", MaxTreechatPostChars+500))); err != nil {
		t.Fatalf("long note: %v", err)
	}
	if got := len([]rune(fake.answers[1]["content"])); got > MaxTreechatPostChars+40 {
		t.Fatalf("notes must be bounded, got %d runes", got)
	}
}

func TestTreechatPosterDisabledIsNoop(t *testing.T) {
	var poster *TreechatPoster
	if poster.Enabled() {
		t.Fatal("nil poster must be disabled")
	}
	fake := newFakeTreechat(t)
	off := newPoster(t, fake, TreechatModeOff, "")
	if _, err := off.SyncSession(context.Background(), sampleSession(), true); err != nil {
		t.Fatalf("off mode must not error: %v", err)
	}
	if len(fake.quests) != 0 {
		t.Fatal("off mode must not post")
	}
}
