package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MemdbCommandTimeout bounds every memdb invocation; hooks must never hang
// the agent.
const MemdbCommandTimeout = 45 * time.Second

// Memdb runs the memdb-rust CLI. memdb owns the schema, FTS and the optional
// Qdrant sidecar, so treecli only ever shells out; it never opens the SQLite
// file itself.
type Memdb struct {
	Binary string
	DBPath string
	// Env is appended to the subprocess environment (MEMDB_QDRANT_URL etc.).
	Env []string
}

// LearnedMemoryTypes mirrors memdb-rust's LEARNED_MEMORY_TYPES.
var LearnedMemoryTypes = []string{"commitment", "decision", "handoff", "lesson", "person", "preference", "project"}

// IngestSummary is memdb's `ingest` JSON output.
type IngestSummary struct {
	Files         int   `json:"files"`
	MessagesAdded int64 `json:"messages_added"`
	EventsAdded   int64 `json:"events_added"`
}

// RecallRequest maps to `memdb-rust recall`.
type RecallRequest struct {
	Mode       string // "thread" or "long-term"
	Query      string
	RecentN    int
	QueryN     int
	SessionKey string
	ChatApp    string
	ChatID     string
	ThreadID   string
}

// QueryRequest maps to `memdb-rust query`.
type QueryRequest struct {
	Query         string
	N             int
	IncludeEvents bool
	SessionKey    string
	ChatApp       string
	ChatID        string
	ThreadID      string
}

// LearnRequest maps to `memdb-rust learn`.
type LearnRequest struct {
	Type       string
	Title      string
	Summary    string
	Key        string
	Priority   string
	Tags       []string
	SourceRefs []string
	Evidence   []string
	SessionID  string
	SessionKey string
	ChatApp    string
	ChatID     string
	ThreadID   string
}

// RecallItem is one memdb hit (a subset of memdb's RecallResult).
type RecallItem struct {
	Type       string `json:"type"`
	ID         int64  `json:"id"`
	TS         string `json:"ts"`
	Text       string `json:"text"`
	AuthorRole string `json:"author_role"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	SessionKey string `json:"session_key"`
	ChatApp    string `json:"chat_app"`
	ThreadID   string `json:"thread_id"`
	SourceFile string `json:"source_file"`
}

// RecallPayload is memdb's `recall` JSON output.
type RecallPayload struct {
	Mode  string `json:"mode"`
	Query string `json:"query"`
	Scope struct {
		SessionKey      string `json:"session_key"`
		ChatApp         string `json:"chat_app"`
		ChatID          string `json:"chat_id"`
		ThreadID        string `json:"thread_id"`
		ConversationKey string `json:"conversation_key"`
	} `json:"scope"`
	Thread *struct {
		Recent []RecallItem `json:"recent"`
		Query  []RecallItem `json:"query"`
	} `json:"thread"`
	LongTerm *struct {
		Results []RecallItem `json:"results"`
	} `json:"long_term"`
}

// ErrMemdbNotFound is returned when no memdb-rust binary can be located.
var ErrMemdbNotFound = errors.New("memdb-rust binary not found (set mcp.memdb_bin in config or TREECLI_MEMDB_BIN, or put memdb-rust on PATH)")

// ResolveMemdbBinary finds the memdb-rust executable: an explicit path first,
// then PATH, then the conventional build locations on the machines memdb
// already lives on.
func ResolveMemdbBinary(configured string) (string, error) {
	candidates := []string{}
	if configured = strings.TrimSpace(configured); configured != "" {
		candidates = append(candidates, expandHome(configured))
	}
	if value := strings.TrimSpace(os.Getenv("TREECLI_MEMDB_BIN")); value != "" {
		candidates = append(candidates, expandHome(value))
	}
	for _, name := range []string{"memdb-rust", "memdb"} {
		if found, err := exec.LookPath(name); err == nil {
			candidates = append(candidates, found)
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, "src", "memdb-rust", "target", "release", "memdb-rust"),
			filepath.Join(home, "memdb-rust", "target", "release", "memdb-rust"),
		)
	}
	candidates = append(candidates, "/root/memdb-rust/target/release/memdb-rust", "/root/clawd/bin/memdb-rust")
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", ErrMemdbNotFound
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Init creates the database schema (idempotent).
func (m *Memdb) Init(ctx context.Context) error {
	_, err := m.run(ctx, "init")
	return err
}

// Ingest ingests recording files. chatApp overrides the app for rows whose
// transcript carries no routing clue; the session key in the recording
// already agrees with it.
func (m *Memdb) Ingest(ctx context.Context, chatApp string, paths ...string) (IngestSummary, error) {
	args := []string{"ingest"}
	if chatApp != "" {
		args = append(args, "--chat-app", chatApp)
	}
	args = append(args, paths...)
	output, err := m.run(ctx, args...)
	if err != nil {
		return IngestSummary{}, err
	}
	var summary IngestSummary
	if err := json.Unmarshal(output, &summary); err != nil {
		return IngestSummary{}, fmt.Errorf("parsing memdb ingest output: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return summary, nil
}

// Recall runs `memdb-rust recall` and returns the parsed payload plus the raw JSON.
func (m *Memdb) Recall(ctx context.Context, request RecallRequest) (RecallPayload, json.RawMessage, error) {
	args := []string{"recall"}
	mode := request.Mode
	if mode == "" {
		mode = "thread"
	}
	args = append(args, "--mode", mode)
	if request.Query != "" {
		args = append(args, "--query", request.Query)
	}
	if request.RecentN > 0 {
		args = append(args, "--recent-n", strconv.Itoa(request.RecentN))
	}
	if request.QueryN > 0 {
		args = append(args, "--query-n", strconv.Itoa(request.QueryN))
	}
	args = appendScopeArgs(args, request.SessionKey, request.ChatApp, request.ChatID, request.ThreadID)
	output, err := m.run(ctx, args...)
	if err != nil {
		return RecallPayload{}, nil, err
	}
	var payload RecallPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return RecallPayload{}, nil, fmt.Errorf("parsing memdb recall output: %w", err)
	}
	return payload, json.RawMessage(output), nil
}

// Query runs `memdb-rust query` (BM25, or hybrid when Qdrant is configured).
func (m *Memdb) Query(ctx context.Context, request QueryRequest) (json.RawMessage, error) {
	args := []string{"query", request.Query}
	if request.N > 0 {
		args = append(args, "-n", strconv.Itoa(request.N))
	}
	if request.IncludeEvents {
		args = append(args, "--include-events")
	}
	args = appendScopeArgs(args, request.SessionKey, request.ChatApp, request.ChatID, request.ThreadID)
	return m.run(ctx, args...)
}

// Learn records an explicit durable memory.
func (m *Memdb) Learn(ctx context.Context, request LearnRequest) (json.RawMessage, error) {
	args := []string{"learn", "--type", request.Type, "--title", request.Title}
	if request.Summary != "" {
		args = append(args, "--summary", request.Summary)
	}
	if request.Key != "" {
		args = append(args, "--key", request.Key)
	}
	if request.Priority != "" {
		args = append(args, "--priority", request.Priority)
	}
	for _, tag := range request.Tags {
		args = append(args, "--tag", tag)
	}
	for _, ref := range request.SourceRefs {
		args = append(args, "--source-ref", ref)
	}
	for _, evidence := range request.Evidence {
		args = append(args, "--evidence", evidence)
	}
	if request.SessionID != "" {
		args = append(args, "--session-id", request.SessionID)
	}
	args = appendScopeArgs(args, request.SessionKey, request.ChatApp, request.ChatID, request.ThreadID)
	return m.run(ctx, args...)
}

func appendScopeArgs(args []string, sessionKey, chatApp, chatID, threadID string) []string {
	if sessionKey != "" {
		args = append(args, "--session-key", sessionKey)
	}
	if chatApp != "" {
		args = append(args, "--chat-app", chatApp)
	}
	if chatID != "" {
		args = append(args, "--chat-id", chatID)
	}
	if threadID != "" {
		args = append(args, "--thread-id", threadID)
	}
	return args
}

func (m *Memdb) run(ctx context.Context, args ...string) ([]byte, error) {
	if m.Binary == "" {
		return nil, ErrMemdbNotFound
	}
	if m.DBPath == "" {
		return nil, errors.New("memdb database path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(m.DBPath), 0o700); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, MemdbCommandTimeout)
	defer cancel()
	command := exec.CommandContext(runCtx, m.Binary, append([]string{"--db", m.DBPath}, args...)...)
	command.Env = append(os.Environ(), m.Env...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if runCtx.Err() != nil {
			return nil, fmt.Errorf("memdb %s timed out after %s", args[0], MemdbCommandTimeout)
		}
		return nil, fmt.Errorf("memdb %s failed: %w: %s", args[0], err, detail)
	}
	return stdout.Bytes(), nil
}
