package recorder

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/adrg/xdg"
)

// Store lays out the recorder's on-disk state under one data directory:
//
//	<data>/memdb.sqlite3                     default memdb database
//	<data>/recordings/<agent>/<session>.jsonl memdb ingest files (one per session)
//	<data>/sessions/<agent>/<project>.json   the session currently active in a cwd
//	<data>/treechat/<agent>/<session>.json   Treechat thread bookkeeping per session
//	<data>/logs/hooks.log                    hook diagnostics (hooks never fail the agent)
type Store struct {
	DataDir string
}

// DefaultDataDir resolves the data directory: TREECLI_MCP_DATA_DIR, then the
// XDG data home (~/Library/Application Support/treecli on macOS,
// ~/.local/share/treecli on Linux).
func DefaultDataDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TREECLI_MCP_DATA_DIR")); value != "" {
		return expandHome(value), nil
	}
	path, err := xdg.DataFile("treecli/.keep")
	if err != nil {
		return "", err
	}
	return filepath.Dir(path), nil
}

// NewStore creates the store rooted at dataDir (resolving the default when empty).
func NewStore(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		resolved, err := DefaultDataDir()
		if err != nil {
			return nil, err
		}
		dataDir = resolved
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	return &Store{DataDir: dataDir}, nil
}

// DefaultDBPath is the memdb database treecli uses unless configured otherwise.
func (s *Store) DefaultDBPath() string {
	return filepath.Join(s.DataDir, "memdb.sqlite3")
}

// RecordingPath is the memdb ingest file for a session.
func (s *Store) RecordingPath(agent Agent, sessionID string) string {
	return filepath.Join(s.DataDir, "recordings", string(agent), safeFileName(sessionID)+".jsonl")
}

// LogPath is the hook diagnostics log.
func (s *Store) LogPath() string {
	return filepath.Join(s.DataDir, "logs", "hooks.log")
}

// Marker records which session is active for an agent in a working directory.
// The MCP server has no session id of its own, so hooks leave this breadcrumb
// and the server reads it to scope memory calls to the live conversation.
type Marker struct {
	Agent          Agent     `json:"agent"`
	SessionID      string    `json:"session_id"`
	CWD            string    `json:"cwd"`
	TranscriptPath string    `json:"transcript_path,omitempty"`
	StartedAt      time.Time `json:"started_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Turns          int       `json:"turns"`
	Ended          bool      `json:"ended"`
}

func (s *Store) markerPath(agent Agent, cwd string) string {
	return filepath.Join(s.DataDir, "sessions", string(agent), ProjectKey(cwd)+".json")
}

// SaveMarker writes the active-session breadcrumb for agent+cwd.
func (s *Store) SaveMarker(marker Marker) error {
	marker.UpdatedAt = time.Now().UTC()
	if marker.StartedAt.IsZero() {
		marker.StartedAt = marker.UpdatedAt
	}
	return writeJSONAtomic(s.markerPath(marker.Agent, marker.CWD), marker)
}

// LoadMarker reads the active-session breadcrumb for agent+cwd.
func (s *Store) LoadMarker(agent Agent, cwd string) (Marker, bool, error) {
	var marker Marker
	data, err := os.ReadFile(s.markerPath(agent, cwd))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Marker{}, false, nil
		}
		return Marker{}, false, err
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return Marker{}, false, fmt.Errorf("corrupt session marker %s: %w", s.markerPath(agent, cwd), err)
	}
	return marker, true, nil
}

// ListMarkers returns every known session marker, newest first.
func (s *Store) ListMarkers() ([]Marker, error) {
	markers := []Marker{}
	root := filepath.Join(s.DataDir, "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return markers, nil
		}
		return nil, err
	}
	for _, agentDir := range entries {
		if !agentDir.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(root, agentDir.Name()))
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, agentDir.Name(), file.Name()))
			if err != nil {
				continue
			}
			var marker Marker
			if err := json.Unmarshal(data, &marker); err != nil {
				continue
			}
			markers = append(markers, marker)
		}
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].UpdatedAt.After(markers[j].UpdatedAt) })
	return markers, nil
}

// TreechatState tracks what a session already posted to Treechat.
type TreechatState struct {
	Agent        Agent     `json:"agent"`
	SessionID    string    `json:"session_id"`
	TeamID       string    `json:"team_id,omitempty"`
	QuestID      string    `json:"quest_id"`
	QuestURL     string    `json:"quest_url,omitempty"`
	RootAnswerID string    `json:"root_answer_id,omitempty"`
	PostedTurns  int       `json:"posted_turns"`
	PostedNotes  []string  `json:"posted_notes,omitempty"`
	ClosedAt     time.Time `json:"closed_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (s *Store) treechatStatePath(agent Agent, sessionID string) string {
	return filepath.Join(s.DataDir, "treechat", string(agent), safeFileName(sessionID)+".json")
}

// LoadTreechatState reads the Treechat bookkeeping for a session.
func (s *Store) LoadTreechatState(agent Agent, sessionID string) (TreechatState, bool, error) {
	data, err := os.ReadFile(s.treechatStatePath(agent, sessionID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TreechatState{Agent: agent, SessionID: sessionID}, false, nil
		}
		return TreechatState{}, false, err
	}
	var state TreechatState
	if err := json.Unmarshal(data, &state); err != nil {
		return TreechatState{}, false, fmt.Errorf("corrupt treechat state %s: %w", s.treechatStatePath(agent, sessionID), err)
	}
	return state, true, nil
}

// SaveTreechatState writes the Treechat bookkeeping for a session.
func (s *Store) SaveTreechatState(state TreechatState) error {
	state.UpdatedAt = time.Now().UTC()
	return writeJSONAtomic(s.treechatStatePath(state.Agent, state.SessionID), state)
}

// AppendLog appends one diagnostics line; failures are ignored on purpose.
func (s *Store) AppendLog(format string, args ...interface{}) {
	path := s.LogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	fmt.Fprintf(file, "%s %s\n", time.Now().UTC().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func writeJSONAtomic(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func safeFileName(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	if builder.Len() == 0 {
		return "unknown"
	}
	return builder.String()
}
