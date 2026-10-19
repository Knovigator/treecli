package recorder

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Recorder wires the sinks together: parse a host transcript (or build one
// from hook payloads), write the memdb ingest file, ingest it, and mirror the
// session into Treechat when that lane is on.
type Recorder struct {
	Store  *Store
	Memdb  *Memdb
	Poster *TreechatPoster // nil or Mode off disables the Treechat lane
}

// SyncResult reports what one sync did.
type SyncResult struct {
	Session       *Session
	RecordingPath string
	Ingest        IngestSummary
	Treechat      TreechatState
	TreechatErr   error
}

// ParseTranscript reads a host transcript for the given agent.
func ParseTranscript(agent Agent, path string) (*Session, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var session *Session
	switch agent {
	case AgentClaudeCode:
		session, err = ParseClaudeTranscript(file)
	case AgentCodex:
		session, err = ParseCodexRollout(file)
	default:
		session, err = ReadRecording(file)
		if err == nil {
			session.Agent = agent
		}
	}
	if err != nil {
		return nil, err
	}
	session.SourcePath = path
	return session, nil
}

// SyncTranscript converts a host transcript and pushes it to every sink.
// sessionID fills a gap the transcript does not state itself; cwd, when
// given, overrides the transcript's own record of it.
func (r *Recorder) SyncTranscript(ctx context.Context, agent Agent, transcriptPath, sessionID, cwd string, ended bool) (SyncResult, error) {
	session, err := ParseTranscript(agent, transcriptPath)
	if err != nil {
		return SyncResult{}, err
	}
	if session.SessionID == "" {
		session.SessionID = sessionID
	}
	// The host's working directory wins over what the transcript recorded: it
	// is what the hook marker and the MCP server key their scope on.
	if strings.TrimSpace(cwd) != "" {
		session.CWD = cwd
	}
	if session.SessionID == "" {
		return SyncResult{}, fmt.Errorf("transcript %s has no session id", transcriptPath)
	}
	return r.SyncSession(ctx, session, ended)
}

// SyncSession writes the recording, ingests it into memdb and mirrors it to
// Treechat. A Treechat failure never blocks the memdb write; it is returned
// in SyncResult.TreechatErr.
func (r *Recorder) SyncSession(ctx context.Context, session *Session, ended bool) (SyncResult, error) {
	result := SyncResult{Session: session}
	result.RecordingPath = r.Store.RecordingPath(session.Agent, session.SessionID)
	if err := WriteRecordingFile(result.RecordingPath, session); err != nil {
		return result, err
	}
	if r.Memdb != nil {
		summary, err := r.Memdb.Ingest(ctx, string(session.Agent), result.RecordingPath)
		if err != nil {
			return result, err
		}
		result.Ingest = summary
	}
	if r.Poster.Enabled() {
		state, err := r.Poster.SyncSession(ctx, session, ended)
		result.Treechat = state
		result.TreechatErr = err
	}
	return result, nil
}

// LoadRecording reads back a recording written earlier (payload-driven
// agents accumulate their session this way).
func (r *Recorder) LoadRecording(agent Agent, sessionID string) (*Session, bool, error) {
	path := r.Store.RecordingPath(agent, sessionID)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer file.Close()
	session, err := ReadRecording(file)
	if err != nil {
		return nil, false, err
	}
	session.Agent = agent
	session.SourcePath = path
	return session, true, nil
}

// ReadRecording parses the memdb JSONL this package writes back into a Session.
func ReadRecording(reader io.Reader) (*Session, error) {
	session := &Session{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1024*1024), 64*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			return nil, fmt.Errorf("corrupt recording line: %w", err)
		}
		switch probe.Type {
		case "session":
			var record memdbSessionRecord
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return nil, err
			}
			session.SessionID = record.ID
			session.CWD = record.CWD
			session.Model = record.Model
			session.Agent = Agent(record.ChatApp)
			session.StartedAt, _ = time.Parse(time.RFC3339Nano, record.Timestamp)
		case "message":
			var record struct {
				Timestamp string `json:"timestamp"`
				ID        string `json:"id"`
				ParentID  string `json:"parentId"`
				ToolName  string `json:"toolName"`
				CallID    string `json:"toolCallId"`
				Message   struct {
					Role    string          `json:"role"`
					Content json.RawMessage `json:"content"`
				} `json:"message"`
			}
			if err := json.Unmarshal([]byte(line), &record); err != nil {
				return nil, err
			}
			message := Message{ID: record.ID, ParentID: record.ParentID, Role: record.Message.Role, ToolName: record.ToolName, ToolCallID: record.CallID}
			message.Timestamp, _ = time.Parse(time.RFC3339Nano, record.Timestamp)
			var text string
			if err := json.Unmarshal(record.Message.Content, &text); err == nil {
				message.Text = text
			} else {
				var blocks []struct {
					Type      string          `json:"type"`
					Text      string          `json:"text"`
					ID        string          `json:"id"`
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if err := json.Unmarshal(record.Message.Content, &blocks); err != nil {
					return nil, fmt.Errorf("corrupt recording message %s: %w", record.ID, err)
				}
				var texts []string
				for _, block := range blocks {
					switch block.Type {
					case "text":
						texts = append(texts, block.Text)
					case "toolCall":
						message.ToolCalls = append(message.ToolCalls, ToolCall{ID: block.ID, Name: block.Name, Arguments: block.Arguments})
					}
				}
				message.Text = strings.Join(texts, "\n")
			}
			session.Messages = append(session.Messages, message)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return session, nil
}

// ProjectRecent returns the most recent user/assistant messages recorded for
// an agent in a project, across sessions (for "last time in this repo").
func (r *Recorder) ProjectRecent(ctx context.Context, agent Agent, cwd string, limit int) ([]RecallItem, error) {
	if r.Memdb == nil {
		return nil, nil
	}
	output, err := r.Memdb.run(ctx, "recent", "-n", fmt.Sprintf("%d", limit), "--chat-app", string(agent), "--chat-id", ProjectKey(cwd))
	if err != nil {
		return nil, err
	}
	var payload struct {
		Results []RecallItem `json:"results"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, fmt.Errorf("parsing memdb recent output: %w", err)
	}
	return payload.Results, nil
}

// TranscriptSource enumerates host transcripts for backfills.
type TranscriptSource struct {
	Agent Agent
	Path  string
	Mtime time.Time
}

// DiscoverTranscripts lists Claude Code and Codex transcripts on this machine,
// newest first, filtered by minimum modification time.
func DiscoverTranscripts(agent Agent, since time.Time) ([]TranscriptSource, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	var patterns []string
	switch agent {
	case AgentClaudeCode:
		patterns = []string{filepath.Join(claudeHome(home), "projects", "*", "*.jsonl")}
	case AgentCodex:
		patterns = []string{
			filepath.Join(codexHome(home), "sessions", "*", "*", "*", "rollout-*.jsonl"),
			filepath.Join(codexHome(home), "archived_sessions", "rollout-*.jsonl"),
		}
	default:
		return nil, fmt.Errorf("%s keeps no transcripts treecli can discover", AgentLabel(agent))
	}
	sources := []TranscriptSource{}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil || info.IsDir() {
				continue
			}
			if !since.IsZero() && info.ModTime().Before(since) {
				continue
			}
			sources = append(sources, TranscriptSource{Agent: agent, Path: match, Mtime: info.ModTime()})
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Mtime.After(sources[j].Mtime) })
	return sources, nil
}

func claudeHome(home string) string {
	if value := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); value != "" {
		return value
	}
	return filepath.Join(home, ".claude")
}

func codexHome(home string) string {
	if value := strings.TrimSpace(os.Getenv("CODEX_HOME")); value != "" {
		return value
	}
	return filepath.Join(home, ".codex")
}
