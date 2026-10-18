package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// Client binds explicit credentials to one origin. It never reads CLI configuration.
// Bearer support here does not grant access to any server endpoint or scope.
type Client struct {
	origin      string
	bearer      string
	accessToken string
	clientID    string
	uid         string
}

func NewOAuthClient(origin, bearer string) (*Client, error) {
	c, err := newClient(origin)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(bearer) == "" || strings.ContainsAny(bearer, "\r\n") {
		return nil, fmt.Errorf("bearer credential is required")
	}
	c.bearer = bearer
	return c, nil
}

func NewTokenClient(origin, accessToken, clientID, uid string) (*Client, error) {
	if clientID == OAuthClient {
		return NewOAuthClient(origin, accessToken)
	}
	c, err := newClient(origin)
	if err != nil {
		return nil, err
	}
	for _, value := range []string{accessToken, clientID, uid} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("complete token credentials are required")
		}
	}
	c.accessToken, c.clientID, c.uid = accessToken, clientID, uid
	return c, nil
}

func newClient(origin string) (*Client, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("backend must be an absolute origin")
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))) {
		return nil, fmt.Errorf("backend requires HTTPS (except loopback)")
	}
	return &Client{origin: strings.TrimRight(origin, "/")}, nil
}

func scopedRequest(ctx context.Context, bearer, accessToken, clientID, uid string, timeout time.Duration) *resty.Request {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := resty.New().SetTimeout(timeout)
	// Do not forward account credentials on redirects, including same-origin redirects.
	client.SetRedirectPolicy(resty.NoRedirectPolicy())
	request := client.R().SetContext(ctx)
	if bearer != "" {
		return request.SetHeader("Authorization", "Bearer "+bearer)
	}
	return request.SetHeader("access-token", accessToken).SetHeader("client", clientID).SetHeader("uid", uid)
}

// DoJSON is a transport primitive for typed operations, not an MCP tool. Callers
// select the endpoint; model-supplied arbitrary methods/paths must not be exposed.
func (c *Client) DoJSON(ctx context.Context, method, path string, query url.Values, body, out interface{}) error {
	u, err := url.Parse(path)
	if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/api/v1/") || strings.Contains(u.Path, "\\") {
		return fmt.Errorf("expected relative API endpoint")
	}
	for _, part := range strings.Split(u.Path, "/") {
		if part == "." || part == ".." {
			return fmt.Errorf("invalid API endpoint")
		}
	}
	request := scopedRequest(ctx, c.bearer, c.accessToken, c.clientID, c.uid, 0).SetHeader("Accept", "application/json").SetQueryParamsFromValues(query)
	if body != nil {
		request.SetHeader("Content-Type", "application/json").SetBody(body)
	}
	response, err := request.Execute(method, c.origin+path)
	if err != nil {
		return fmt.Errorf("API request failed")
	}
	if response.StatusCode() < 200 || response.StatusCode() >= 300 {
		return fmt.Errorf("API request failed (status %d): %s", response.StatusCode(), SafeCredentialResponseBody(response.Body(), c.bearer, c.accessToken, c.clientID, c.uid))
	}
	if out == nil || len(response.Body()) == 0 {
		return nil
	}
	if err := json.Unmarshal(response.Body(), out); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	return nil
}
