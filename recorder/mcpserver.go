package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ServerInstructions is sent to hosts at initialize. Keep the first sentence
// self-contained: Codex only reads the opening 512 characters reliably.
const ServerInstructions = `treecli memory: this machine's coding-agent sessions (Claude Code, Codex, Grok) are recorded into memdb, a local durable-memory store, and optionally mirrored into Treechat. Call memory_recall before starting work that may have history in this project, memory_search to find past decisions or commands, and memory_learn to save a decision, lesson or preference worth keeping across sessions. session_note and treechat_post write to the session log; session_info shows what is being recorded.`

// MCPContext is what the server knows about the conversation it serves. Hosts
// do not tell MCP servers their session id, so the hook breadcrumb (Marker)
// supplies it; without one, tools work at project scope.
type MCPContext struct {
	Agent      Agent
	CWD        string
	ProjectKey string
	SessionID  string
	SessionKey string
	Marker     *Marker
}

// ResolveMCPContext builds the context for agent+cwd from the hook marker.
func ResolveMCPContext(store *Store, agent Agent, cwd string) MCPContext {
	context := MCPContext{Agent: agent, CWD: cwd, ProjectKey: ProjectKey(cwd)}
	if marker, ok, err := store.LoadMarker(agent, cwd); err == nil && ok && !marker.Ended {
		context.SessionID = marker.SessionID
		context.SessionKey = SessionKey(agent, context.ProjectKey, marker.SessionID)
		context.Marker = &marker
	}
	return context
}

// MCPServerOptions configures NewMCPServer.
type MCPServerOptions struct {
	Agent    Agent
	CWD      string
	Recorder *Recorder
	Version  string
	// Now lets tests pin timestamps.
	Now func() time.Time
}

type mcpServer struct {
	options MCPServerOptions
}

func (s *mcpServer) now() time.Time {
	if s.options.Now != nil {
		return s.options.Now()
	}
	return time.Now()
}

func (s *mcpServer) context() MCPContext {
	return ResolveMCPContext(s.options.Recorder.Store, s.options.Agent, s.options.CWD)
}

// NewMCPServer builds the treecli MCP server with its tools registered.
func NewMCPServer(options MCPServerOptions) *mcp.Server {
	if options.CWD == "" {
		options.CWD, _ = os.Getwd()
	}
	if options.Version == "" {
		options.Version = "dev"
	}
	s := &mcpServer{options: options}
	server := mcp.NewServer(&mcp.Implementation{Name: "treecli", Title: "treecli memory", Version: options.Version}, &mcp.ServerOptions{Instructions: ServerInstructions})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}
	additive := &mcp.ToolAnnotations{DestructiveHint: boolPtr(false), IdempotentHint: true}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_recall",
		Description: "Recall memory for the current session and project from memdb. mode=thread returns recent turns of this session plus query hits; mode=long-term returns durable learned memories (decisions, lessons, preferences) matching the query; mode=both returns both. Use it at the start of work that may have history here.",
		Annotations: readOnly,
	}, s.memoryRecall)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_search",
		Description: "Full-text (or hybrid semantic, when Qdrant is configured) search over recorded agent sessions. scope=session (this conversation), project (every session in this working directory), agent (every project of this agent), all (every agent). include_events adds tool calls and tool results.",
		Annotations: readOnly,
	}, s.memorySearch)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "memory_learn",
		Description: "Save a durable memory in memdb so future sessions can recall it: a decision, lesson, preference, commitment, handoff, project fact or person. scope=project (default) ties it to this working directory, session to this conversation, global to every project.",
		Annotations: additive,
	}, s.memoryLearn)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_note",
		Description: "Append a note to this session's recording (memdb). Set post_to_treechat=true to also post it into the session's Treechat thread when Treechat recording is on.",
		Annotations: additive,
	}, s.sessionNote)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "treechat_post",
		Description: "Post text into this session's Treechat thread (created on first use inside the configured stream). Only works when Treechat recording is configured; returns the thread URL.",
		Annotations: additive,
	}, s.treechatPost)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "session_info",
		Description: "Show what treecli knows about the current session: agent, session id, project key, memdb scope, recording path, turn count, and Treechat thread if any.",
		Annotations: readOnly,
	}, s.sessionInfo)
	return server
}

func boolPtr(value bool) *bool { return &value }

func textResult(value interface{}) (*mcp.CallToolResult, any, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(encoded)}}}, nil, nil
}

type recallInput struct {
	Query string `json:"query,omitempty" jsonschema:"what to look for; optional for mode=thread, required for long-term hits"`
	Mode  string `json:"mode,omitempty" jsonschema:"thread, long-term, or both (default both)"`
	Limit int    `json:"limit,omitempty" jsonschema:"max hits per list (default 6)"`
}

type recallHit struct {
	When string `json:"when"`
	Role string `json:"role,omitempty"`
	Kind string `json:"kind,omitempty"`
	Text string `json:"text"`
}

func compactHits(items []RecallItem, limit int) []recallHit {
	hits := []recallHit{}
	for _, item := range items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		hits = append(hits, recallHit{When: shortWhen(item.TS), Role: item.AuthorRole, Kind: item.Kind, Text: Truncate(text, 700)})
		if limit > 0 && len(hits) >= limit {
			break
		}
	}
	return hits
}

func shortWhen(ts string) string {
	if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return parsed.Local().Format("2006-01-02 15:04")
	}
	return ts
}

func (s *mcpServer) memoryRecall(ctx context.Context, _ *mcp.CallToolRequest, input recallInput) (*mcp.CallToolResult, any, error) {
	memdb := s.options.Recorder.Memdb
	if memdb == nil {
		return nil, nil, ErrMemdbNotFound
	}
	current := s.context()
	limit := input.Limit
	if limit <= 0 {
		limit = 6
	}
	mode := strings.ToLower(strings.TrimSpace(input.Mode))
	if mode == "" {
		mode = "both"
	}
	query := FTSQuery(input.Query)
	response := map[string]interface{}{
		"scope": map[string]string{"agent": string(current.Agent), "project": current.ProjectKey, "session_id": current.SessionID},
	}
	if mode == "thread" || mode == "both" {
		request := RecallRequest{Mode: "thread", Query: query, RecentN: limit, QueryN: limit, ChatApp: string(current.Agent), ChatID: current.ProjectKey}
		if current.SessionID != "" {
			request.SessionKey = current.SessionKey
		}
		payload, _, err := memdb.Recall(ctx, request)
		if err != nil {
			return nil, nil, err
		}
		thread := map[string]interface{}{}
		if payload.Thread != nil {
			thread["recent"] = compactHits(payload.Thread.Recent, limit)
			thread["query_hits"] = compactHits(payload.Thread.Query, limit)
		}
		response["thread"] = thread
	}
	if mode == "long-term" || mode == "long_term" || mode == "both" {
		if query == "" {
			response["long_term"] = "pass a query to search durable memories"
		} else {
			payload, _, err := memdb.Recall(ctx, RecallRequest{Mode: "long-term", Query: query, QueryN: limit, ChatApp: string(current.Agent), ChatID: current.ProjectKey})
			if err != nil {
				return nil, nil, err
			}
			hits := []recallHit{}
			if payload.LongTerm != nil {
				hits = compactHits(payload.LongTerm.Results, limit)
			}
			response["long_term"] = hits
		}
	}
	return textResult(response)
}

type searchInput struct {
	Query         string `json:"query" jsonschema:"search terms"`
	Scope         string `json:"scope,omitempty" jsonschema:"session, project (default), agent, or all"`
	Limit         int    `json:"limit,omitempty" jsonschema:"max hits (default 10)"`
	IncludeEvents bool   `json:"include_events,omitempty" jsonschema:"also search tool calls and tool results"`
}

func (s *mcpServer) memorySearch(ctx context.Context, _ *mcp.CallToolRequest, input searchInput) (*mcp.CallToolResult, any, error) {
	memdb := s.options.Recorder.Memdb
	if memdb == nil {
		return nil, nil, ErrMemdbNotFound
	}
	query := FTSQuery(input.Query)
	if query == "" {
		return nil, nil, fmt.Errorf("query needs at least one searchable word")
	}
	current := s.context()
	limit := input.Limit
	if limit <= 0 {
		limit = 10
	}
	request := QueryRequest{Query: query, N: limit, IncludeEvents: input.IncludeEvents}
	switch strings.ToLower(strings.TrimSpace(input.Scope)) {
	case "session":
		if current.SessionID == "" {
			return nil, nil, fmt.Errorf("no active session is known for this directory; use scope=project")
		}
		request.SessionKey = current.SessionKey
	case "", "project":
		request.ChatApp = string(current.Agent)
		request.ChatID = current.ProjectKey
	case "agent":
		request.ChatApp = string(current.Agent)
	case "all":
	default:
		return nil, nil, fmt.Errorf("unknown scope %q (use session, project, agent, or all)", input.Scope)
	}
	output, err := memdb.Query(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	var payload struct {
		Results []RecallItem `json:"results"`
	}
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, nil, fmt.Errorf("parsing memdb query output: %w", err)
	}
	type searchHit struct {
		recallHit
		Session string `json:"session,omitempty"`
		Agent   string `json:"agent,omitempty"`
	}
	hits := []searchHit{}
	for _, item := range payload.Results {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		hits = append(hits, searchHit{recallHit: recallHit{When: shortWhen(item.TS), Role: item.AuthorRole, Kind: item.Kind, Text: Truncate(text, 700)}, Session: item.ThreadID, Agent: item.ChatApp})
	}
	return textResult(map[string]interface{}{"query": input.Query, "scope": firstNonEmptyString(input.Scope, "project"), "hits": hits})
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type learnInput struct {
	Type     string   `json:"type" jsonschema:"decision, lesson, preference, commitment, handoff, project, or person"`
	Title    string   `json:"title" jsonschema:"one-line memory"`
	Summary  string   `json:"summary,omitempty" jsonschema:"a few sentences of detail"`
	Key      string   `json:"key,omitempty" jsonschema:"stable slug; reuse it to update an earlier memory"`
	Priority string   `json:"priority,omitempty" jsonschema:"red, yellow, or green"`
	Tags     []string `json:"tags,omitempty"`
	Evidence []string `json:"evidence,omitempty" jsonschema:"quotes, paths or commands backing the memory"`
	Scope    string   `json:"scope,omitempty" jsonschema:"project (default), session, or global"`
}

func (s *mcpServer) memoryLearn(ctx context.Context, _ *mcp.CallToolRequest, input learnInput) (*mcp.CallToolResult, any, error) {
	memdb := s.options.Recorder.Memdb
	if memdb == nil {
		return nil, nil, ErrMemdbNotFound
	}
	memoryType := strings.ToLower(strings.TrimSpace(input.Type))
	valid := false
	for _, candidate := range LearnedMemoryTypes {
		if candidate == memoryType {
			valid = true
		}
	}
	if !valid {
		return nil, nil, fmt.Errorf("type must be one of %s", strings.Join(LearnedMemoryTypes, ", "))
	}
	if strings.TrimSpace(input.Title) == "" {
		return nil, nil, fmt.Errorf("title is required")
	}
	current := s.context()
	request := LearnRequest{Type: memoryType, Title: input.Title, Summary: input.Summary, Key: input.Key, Priority: input.Priority, Tags: input.Tags, Evidence: input.Evidence, SessionID: current.SessionID}
	switch strings.ToLower(strings.TrimSpace(input.Scope)) {
	case "", "project":
		request.ChatApp = string(current.Agent)
		request.ChatID = current.ProjectKey
	case "session":
		if current.SessionID == "" {
			return nil, nil, fmt.Errorf("no active session is known for this directory; use scope=project")
		}
		request.SessionKey = current.SessionKey
	case "global":
	default:
		return nil, nil, fmt.Errorf("unknown scope %q (use project, session, or global)", input.Scope)
	}
	output, err := memdb.Learn(ctx, request)
	if err != nil {
		return nil, nil, err
	}
	var summary map[string]interface{}
	if err := json.Unmarshal(output, &summary); err != nil {
		summary = map[string]interface{}{"raw": string(output)}
	}
	summary["scope"] = firstNonEmptyString(input.Scope, "project")
	return textResult(summary)
}

type noteInput struct {
	Text           string `json:"text" jsonschema:"the note"`
	PostToTreechat bool   `json:"post_to_treechat,omitempty" jsonschema:"also post it into the session's Treechat thread"`
}

func (s *mcpServer) sessionNote(ctx context.Context, _ *mcp.CallToolRequest, input noteInput) (*mcp.CallToolResult, any, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, nil, fmt.Errorf("text is required")
	}
	current := s.context()
	if current.SessionID == "" {
		return nil, nil, fmt.Errorf("no active session is known for this directory (hooks not installed?); nothing recorded")
	}
	session, ok, err := s.options.Recorder.LoadRecording(current.Agent, current.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		session = &Session{Agent: current.Agent, SessionID: current.SessionID, CWD: current.CWD, StartedAt: s.now().UTC()}
	}
	now := s.now().UTC()
	session.Messages = append(session.Messages, Message{
		ID:        DeterministicID("note", string(current.Agent), current.SessionID, fmt.Sprintf("%d", now.UnixNano())),
		Timestamp: now, Role: RoleAssistant, Text: "Note: " + text,
	})
	result, err := s.options.Recorder.SyncSession(ctx, session, false)
	if err != nil {
		return nil, nil, err
	}
	response := map[string]interface{}{"recorded": true, "recording_path": result.RecordingPath}
	if input.PostToTreechat {
		poster := s.options.Recorder.Poster
		if !poster.Enabled() {
			response["treechat"] = "off (configure with `treecli mcp config --treechat-mode session`)"
		} else {
			state, url, err := poster.PostNote(ctx, session, DeterministicID("note-key", text), text)
			if err != nil {
				response["treechat_error"] = err.Error()
			} else {
				response["treechat_thread"] = firstNonEmptyString(url, state.QuestID)
			}
		}
	}
	return textResult(response)
}

type postInput struct {
	Text string `json:"text" jsonschema:"the post body"`
}

func (s *mcpServer) treechatPost(ctx context.Context, _ *mcp.CallToolRequest, input postInput) (*mcp.CallToolResult, any, error) {
	text := strings.TrimSpace(input.Text)
	if text == "" {
		return nil, nil, fmt.Errorf("text is required")
	}
	poster := s.options.Recorder.Poster
	if !poster.Enabled() {
		return nil, nil, fmt.Errorf("treechat recording is off; run `treecli mcp config --treechat-mode session [--treechat-team <stream id>]`")
	}
	current := s.context()
	if current.SessionID == "" {
		return nil, nil, fmt.Errorf("no active session is known for this directory (hooks not installed?)")
	}
	session, ok, err := s.options.Recorder.LoadRecording(current.Agent, current.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		session = &Session{Agent: current.Agent, SessionID: current.SessionID, CWD: current.CWD, StartedAt: s.now().UTC()}
	}
	state, url, err := poster.PostNote(ctx, session, DeterministicID("post-key", text), text)
	if err != nil {
		return nil, nil, err
	}
	return textResult(map[string]interface{}{"posted": true, "thread_url": url, "quest_id": state.QuestID})
}

type emptyInput struct{}

func (s *mcpServer) sessionInfo(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, any, error) {
	current := s.context()
	rec := s.options.Recorder
	info := map[string]interface{}{
		"agent":       string(current.Agent),
		"cwd":         current.CWD,
		"project_key": current.ProjectKey,
		"session_id":  current.SessionID,
		"session_key": current.SessionKey,
		"data_dir":    rec.Store.DataDir,
	}
	if current.SessionID != "" {
		info["recording_path"] = rec.Store.RecordingPath(current.Agent, current.SessionID)
		if current.Marker != nil {
			info["turns"] = current.Marker.Turns
			info["started_at"] = current.Marker.StartedAt
		}
		if state, ok, _ := rec.Store.LoadTreechatState(current.Agent, current.SessionID); ok {
			info["treechat_thread"] = map[string]interface{}{"quest_id": state.QuestID, "url": state.QuestURL, "posted_turns": state.PostedTurns}
		}
	} else {
		info["note"] = "no active session marker for this directory; install hooks with `treecli mcp install` so tools scope to the live conversation"
	}
	if rec.Memdb != nil {
		info["memdb"] = map[string]string{"binary": rec.Memdb.Binary, "db": rec.Memdb.DBPath}
	} else {
		info["memdb"] = "not configured"
	}
	if rec.Poster.Enabled() {
		info["treechat"] = map[string]string{"mode": rec.Poster.Mode, "team_id": rec.Poster.TeamID}
	} else {
		info["treechat"] = "off"
	}
	return textResult(info)
}

// RunStdio serves MCP over stdin/stdout until the host disconnects.
func RunStdio(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}
