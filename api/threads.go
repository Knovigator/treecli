package api

import (
	"encoding/json"
	"regexp"
	"strings"
)

// ThreadListOptions describes an authored, creation-ordered collection.
type ThreadListOptions struct {
	User  string
	Scope string
	Page  int
	Limit int
	// Public lists the public feed instead of an author's threads; Order is
	// forwarded as-is (time, upvalue, upvalueTrending) and the server validates it.
	Public bool
	Order  string
	// Before is the keyset cursor for the time-ordered public feed; ranked
	// orders page by Page instead.
	Before string
}

type ThreadPagination struct {
	Page     int  `json:"page"`
	Limit    int  `json:"limit"`
	NextPage *int `json:"next_page"`
	// NextCursor is set instead of NextPage by the time-ordered public feed.
	NextCursor *string `json:"next_cursor"`
	HasMore    bool    `json:"has_more"`
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
	if user == "me" || userIDPattern.MatchString(user) {
		return user, nil
	}
	var out struct {
		User User `json:"user"`
	}
	_, err := sharedRead(backendURL, accessToken, client, uid, "user", map[string]interface{}{"username": strings.TrimPrefix(user, "@")}, &out)
	return out.User.ID, err
}

func ListThreads(backendURL, accessToken, client, uid string, options ThreadListOptions) (ThreadsResponse, error) {
	var out ThreadsResponse
	if options.Public {
		args := map[string]interface{}{"public": true}
		if options.Order != "" {
			args["order"] = options.Order
		}
		if options.Order == "" || options.Order == "time" {
			args["limit"] = options.Limit
			if options.Before != "" {
				args["before"] = options.Before
			}
		} else {
			args["page"] = options.Page
		}
		raw, err := sharedRead(backendURL, accessToken, client, uid, "threads", args, &out)
		out.Raw = raw
		return out, err
	}
	raw, err := sharedRead(backendURL, accessToken, client, uid, "threads", map[string]interface{}{"user": options.User, "thread_scope": options.Scope, "page": options.Page, "limit": options.Limit}, &out)
	out.Raw = raw
	return out, err
}

// ThreadsForAnswer preserves complete thread payloads and server-side visibility.
func ThreadsForAnswer(backendURL, accessToken, client, uid, answerID string) (ThreadsResponse, error) {
	var out ThreadsResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "threads", map[string]interface{}{"answer": answerID}, &out)
	out.Raw = raw
	return out, err
}
