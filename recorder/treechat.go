package recorder

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Knovigator/treecli/api"
)

// Treechat recording modes.
const (
	TreechatModeOff     = "off"     // never post
	TreechatModeSession = "session" // one thread per session: an opening post and a closing summary
	TreechatModeTurns   = "turns"   // additionally one reply per turn (prompt + final reply + tool summary)
)

// TreechatModes lists the accepted modes.
var TreechatModes = []string{TreechatModeOff, TreechatModeSession, TreechatModeTurns}

// ParseTreechatMode validates a mode string.
func ParseTreechatMode(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return TreechatModeOff, nil
	}
	for _, mode := range TreechatModes {
		if mode == normalized {
			return mode, nil
		}
	}
	return "", fmt.Errorf("unknown treechat mode %q (use off, session, or turns)", value)
}

// MaxTreechatPostChars bounds the text of one recorded post.
const MaxTreechatPostChars = 3500

// Credentials is the authenticated Treechat identity used for posting.
type Credentials struct {
	BackendURL    string
	AppHost       string
	AccessToken   string
	Client        string
	UID           string
	SpaceID       string
	CurrentUserID string
}

// TreechatPoster records sessions as threads in a stream (Team). Every write
// uses a deterministic id derived from the session, so retries and re-runs
// land on the same thread and the same replies instead of duplicating them.
type TreechatPoster struct {
	Credentials Credentials
	Store       *Store
	// TeamID is the stream to post into. When empty, threads are created as
	// private threads of the posting account.
	TeamID string
	Mode   string
	// Now lets tests pin timestamps.
	Now func() time.Time
}

func (p *TreechatPoster) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Enabled reports whether posting is configured at all.
func (p *TreechatPoster) Enabled() bool {
	return p != nil && p.Mode != "" && p.Mode != TreechatModeOff
}

// SyncSession posts whatever the mode calls for and has not been posted yet.
// ended marks the final sync of a session (posts the closing summary).
func (p *TreechatPoster) SyncSession(ctx context.Context, session *Session, ended bool) (TreechatState, error) {
	if !p.Enabled() {
		return TreechatState{}, nil
	}
	if p.Credentials.AccessToken == "" {
		return TreechatState{}, errors.New("treechat recording needs a logged-in account (run `treecli login`)")
	}
	state, _, err := p.Store.LoadTreechatState(session.Agent, session.SessionID)
	if err != nil {
		return state, err
	}
	turns := session.Turns()
	if len(turns) == 0 {
		return state, nil
	}
	if state.QuestID == "" {
		if err := p.createSessionThread(ctx, session, turns[0], &state); err != nil {
			return state, err
		}
		if err := p.Store.SaveTreechatState(state); err != nil {
			return state, err
		}
	}
	if p.Mode == TreechatModeTurns {
		for index := state.PostedTurns; index < len(turns); index++ {
			turn := turns[index]
			// A turn still in flight has no reply yet; wait for the next sync.
			if turn.FinalReply == "" && !ended {
				break
			}
			if err := p.postTurn(ctx, session, turn, &state); err != nil {
				return state, err
			}
			state.PostedTurns = index + 1
			if err := p.Store.SaveTreechatState(state); err != nil {
				return state, err
			}
		}
	}
	if ended && state.ClosedAt.IsZero() {
		if err := p.postClosing(ctx, session, turns, &state); err != nil {
			return state, err
		}
		state.ClosedAt = p.now().UTC()
		if err := p.Store.SaveTreechatState(state); err != nil {
			return state, err
		}
	}
	return state, nil
}

// PostNote posts an explicit note (from the session_note / treechat_post MCP
// tools) into the session thread, creating the thread first when needed.
func (p *TreechatPoster) PostNote(ctx context.Context, session *Session, noteKey, text string) (TreechatState, string, error) {
	if !p.Enabled() {
		return TreechatState{}, "", errors.New("treechat recording is off (run `treecli mcp config --treechat-mode session`)")
	}
	if p.Credentials.AccessToken == "" {
		return TreechatState{}, "", errors.New("treechat recording needs a logged-in account (run `treecli login`)")
	}
	state, _, err := p.Store.LoadTreechatState(session.Agent, session.SessionID)
	if err != nil {
		return state, "", err
	}
	if state.QuestID == "" {
		turns := session.Turns()
		first := Turn{}
		if len(turns) > 0 {
			first = turns[0]
		}
		if err := p.createSessionThread(ctx, session, first, &state); err != nil {
			return state, "", err
		}
		if err := p.Store.SaveTreechatState(state); err != nil {
			return state, "", err
		}
	}
	for _, posted := range state.PostedNotes {
		if posted == noteKey {
			return state, state.QuestURL, nil
		}
	}
	answerID := DeterministicID("note", string(session.Agent), session.SessionID, noteKey)
	result, err := api.CreateAnswer(p.Credentials.BackendURL, p.Credentials.AccessToken, p.Credentials.Client, p.Credentials.UID, api.CreateAnswerRequest{
		AnswerID: answerID,
		QuestID:  state.QuestID,
		SpaceID:  p.Credentials.SpaceID,
		Content:  Truncate(text, MaxTreechatPostChars),
	})
	if err != nil {
		return state, "", fmt.Errorf("posting note to treechat: %w", err)
	}
	if result.Answer.ID == "" {
		return state, "", errors.New("treechat did not return the created note")
	}
	state.PostedNotes = append(state.PostedNotes, noteKey)
	if err := p.Store.SaveTreechatState(state); err != nil {
		return state, "", err
	}
	return state, state.QuestURL, nil
}

func (p *TreechatPoster) createSessionThread(ctx context.Context, session *Session, first Turn, state *TreechatState) error {
	questID := DeterministicID("session-thread", string(session.Agent), session.SessionID)
	rootAnswerID := DeterministicID("session-root", string(session.Agent), session.SessionID)
	request := api.CreateQuestRequest{
		QuestID:        questID,
		ParentAnswerID: rootAnswerID,
		SpaceID:        p.Credentials.SpaceID,
		Content:        p.openingPost(session, first),
	}
	if p.TeamID != "" {
		request.TeamID = p.TeamID
	} else {
		private := true
		request.Private = &private
	}
	result, err := api.CreateQuest(p.Credentials.BackendURL, p.Credentials.AccessToken, p.Credentials.Client, p.Credentials.UID, request)
	if err != nil {
		return fmt.Errorf("creating treechat session thread: %w", err)
	}
	if result.Quest.ID == "" {
		return errors.New("treechat did not return the created thread")
	}
	state.TeamID = p.TeamID
	state.QuestID = result.Quest.ID
	state.QuestURL = result.Quest.QuestURL
	state.RootAnswerID = rootAnswerID
	return nil
}

func (p *TreechatPoster) postTurn(ctx context.Context, session *Session, turn Turn, state *TreechatState) error {
	answerID := DeterministicID("turn", string(session.Agent), session.SessionID, fmt.Sprintf("%d", turn.Index))
	_, err := api.CreateAnswer(p.Credentials.BackendURL, p.Credentials.AccessToken, p.Credentials.Client, p.Credentials.UID, api.CreateAnswerRequest{
		AnswerID: answerID,
		QuestID:  state.QuestID,
		SpaceID:  p.Credentials.SpaceID,
		Content:  p.turnPost(turn),
	})
	if err != nil {
		return fmt.Errorf("posting turn %d to treechat: %w", turn.Index+1, err)
	}
	return nil
}

func (p *TreechatPoster) postClosing(ctx context.Context, session *Session, turns []Turn, state *TreechatState) error {
	answerID := DeterministicID("closing", string(session.Agent), session.SessionID)
	_, err := api.CreateAnswer(p.Credentials.BackendURL, p.Credentials.AccessToken, p.Credentials.Client, p.Credentials.UID, api.CreateAnswerRequest{
		AnswerID: answerID,
		QuestID:  state.QuestID,
		SpaceID:  p.Credentials.SpaceID,
		Content:  p.closingPost(session, turns),
	})
	if err != nil {
		return fmt.Errorf("posting session summary to treechat: %w", err)
	}
	return nil
}

// AgentLabel is the human name used in posts.
func AgentLabel(agent Agent) string {
	switch agent {
	case AgentClaudeCode:
		return "Claude Code"
	case AgentCodex:
		return "Codex"
	case AgentGrok:
		return "Grok"
	}
	return string(agent)
}

func (p *TreechatPoster) openingPost(session *Session, first Turn) string {
	started := session.StartedAt
	if started.IsZero() {
		started = p.now()
	}
	lines := []string{
		fmt.Sprintf("%s session · %s", AgentLabel(session.Agent), started.Local().Format("Jan 2, 2006 15:04")),
		fmt.Sprintf("Project: %s", projectLabel(session.CWD)),
	}
	if session.Model != "" {
		lines = append(lines, "Model: "+session.Model)
	}
	if prompt := strings.TrimSpace(first.UserPrompt); prompt != "" {
		lines = append(lines, "", "First prompt:", Truncate(prompt, 1200))
	}
	lines = append(lines, "", "Recorded by treecli · session "+session.SessionID)
	return Truncate(strings.Join(lines, "\n"), MaxTreechatPostChars)
}

func (p *TreechatPoster) turnPost(turn Turn) string {
	lines := []string{fmt.Sprintf("Turn %d", turn.Index+1)}
	if prompt := strings.TrimSpace(turn.UserPrompt); prompt != "" {
		lines = append(lines, "", "> "+strings.ReplaceAll(Truncate(prompt, 800), "\n", "\n> "))
	}
	if reply := strings.TrimSpace(turn.FinalReply); reply != "" {
		lines = append(lines, "", Truncate(reply, 2000))
	}
	if summary := turn.ToolSummary(); summary != "" {
		lines = append(lines, "", summary)
	}
	return Truncate(strings.Join(lines, "\n"), MaxTreechatPostChars)
}

func (p *TreechatPoster) closingPost(session *Session, turns []Turn) string {
	toolCalls := 0
	for _, turn := range turns {
		toolCalls += turn.ToolCallsSum
	}
	var last Turn
	if len(turns) > 0 {
		last = turns[len(turns)-1]
	}
	lines := []string{
		fmt.Sprintf("Session ended · %d turns · %d tool calls", len(turns), toolCalls),
	}
	if !session.StartedAt.IsZero() && !last.EndedAt.IsZero() && last.EndedAt.After(session.StartedAt) {
		lines[0] += " · " + last.EndedAt.Sub(session.StartedAt).Round(time.Minute).String()
	}
	if reply := strings.TrimSpace(last.FinalReply); reply != "" {
		lines = append(lines, "", "Last reply:", Truncate(reply, 1500))
	}
	return Truncate(strings.Join(lines, "\n"), MaxTreechatPostChars)
}

func projectLabel(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "(unknown)"
	}
	return cwd
}
