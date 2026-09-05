package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ThreadListOptions describes an authored, creation-ordered collection.
type ThreadListOptions struct {
	User  string
	Scope string
	Page  int
	Limit int
}

type ThreadPagination struct {
	Page     int  `json:"page"`
	Limit    int  `json:"limit"`
	NextPage *int `json:"next_page"`
	HasMore  bool `json:"has_more"`
}

type ThreadsResponse struct {
	Threads    []Quest           `json:"threads"`
	Pagination *ThreadPagination `json:"pagination,omitempty"`
	Raw        json.RawMessage   `json:"-"`
}

var userIDPattern = regexp.MustCompile(`^(?:[0-9]+|[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`)

// ResolveThreadUser accepts IDs, usernames (optionally @-prefixed), and me.
// The server resolves me from the authenticated session, even for old profiles.
func ResolveThreadUser(backendURL, accessToken, client, uid, user string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "me" || userIDPattern.MatchString(user) {
		return user, nil
	}
	name := strings.TrimPrefix(user, "@")
	if name == "" {
		return "", fmt.Errorf("user must not be empty")
	}
	resp, err := newRequest(accessToken, client, uid).SetHeader("accept", "application/json").Get(strings.TrimRight(backendURL, "/") + "/api/v1/users/name/" + url.PathEscape(name))
	if err != nil {
		return "", fmt.Errorf("resolving user: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return "", fmt.Errorf("resolving user: received status code %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
	}
	var result struct {
		User User `json:"user"`
	}
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return "", fmt.Errorf("parsing user: %w", err)
	}
	if result.User.ID == "" {
		return "", fmt.Errorf("user response is missing an ID")
	}
	return result.User.ID, nil
}

func ListThreads(backendURL, accessToken, client, uid string, options ThreadListOptions) (ThreadsResponse, error) {
	if options.Page < 1 || options.Limit < 1 || options.Limit > 100 {
		return ThreadsResponse{}, fmt.Errorf("page must be positive and limit must be between 1 and 100")
	}
	if options.Scope != "all" && options.Scope != "root" && options.Scope != "branch" {
		return ThreadsResponse{}, fmt.Errorf("invalid thread scope %q", options.Scope)
	}
	user, err := ResolveThreadUser(backendURL, accessToken, client, uid, options.User)
	if err != nil {
		return ThreadsResponse{}, err
	}
	resp, err := newRequest(accessToken, client, uid).SetHeader("accept", "application/json").SetQueryParams(map[string]string{
		"authored": "true", "user": user, "thread_scope": options.Scope,
		"page": strconv.Itoa(options.Page), "limit": strconv.Itoa(options.Limit),
	}).Get(strings.TrimRight(backendURL, "/") + "/api/v1/quests")
	if err != nil {
		return ThreadsResponse{}, fmt.Errorf("listing threads: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return ThreadsResponse{}, fmt.Errorf("listing threads: received status code %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
	}
	var result ThreadsResponse
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return ThreadsResponse{}, fmt.Errorf("parsing threads: %w", err)
	}
	// Older servers ignore unknown query parameters and return an ordinary feed.
	if result.Threads == nil || result.Pagination == nil {
		return ThreadsResponse{}, fmt.Errorf("backend does not support authored thread listing; update the Treechat backend")
	}
	result.Raw = append(json.RawMessage(nil), resp.Body()...)
	return result, nil
}

// ThreadsForAnswer preserves complete thread payloads and server-side visibility.
func ThreadsForAnswer(backendURL, accessToken, client, uid, answerID string) (ThreadsResponse, error) {
	answer, err := GetAnswer(backendURL, url.PathEscape(answerID), accessToken, client, uid)
	if err != nil {
		return ThreadsResponse{}, err
	}
	if answer.Answer.ID == "" {
		return ThreadsResponse{}, fmt.Errorf("answer response is missing an ID")
	}
	result := ThreadsResponse{Threads: []Quest{}}
	rawThreads := []json.RawMessage{}
	seen := map[string]bool{}
	for _, child := range answer.Answer.ChildQuests {
		if child.ID == "" || seen[child.ID] {
			continue
		}
		seen[child.ID] = true
		// A child reference may become inaccessible between the answer and thread reads.
		resp, err := newRequest(accessToken, client, uid).SetHeader("accept", "application/json").Get(strings.TrimRight(backendURL, "/") + "/api/v1/quests/" + url.PathEscape(child.ID))
		if err != nil {
			return ThreadsResponse{}, fmt.Errorf("fetching child thread: %w", err)
		}
		if resp.StatusCode() == http.StatusForbidden || resp.StatusCode() == http.StatusNotFound {
			continue
		}
		if resp.StatusCode() != http.StatusOK {
			return ThreadsResponse{}, fmt.Errorf("fetching child thread: received status code %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
		}
		var thread ThreadResponse
		if err := json.Unmarshal(resp.Body(), &thread); err != nil {
			return ThreadsResponse{}, fmt.Errorf("parsing child thread: %w", err)
		}
		var raw struct {
			Quest json.RawMessage `json:"quest"`
		}
		if err := json.Unmarshal(resp.Body(), &raw); err != nil {
			return ThreadsResponse{}, err
		}
		if thread.Quest.ID == "" {
			return ThreadsResponse{}, fmt.Errorf("child thread response is missing an ID")
		}
		result.Threads = append(result.Threads, thread.Quest)
		rawThreads = append(rawThreads, raw.Quest)
	}
	result.Raw, err = json.Marshal(struct {
		Threads []json.RawMessage `json:"threads"`
	}{rawThreads})
	return result, err
}
