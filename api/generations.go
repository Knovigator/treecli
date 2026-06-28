package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// GenerationResponse is the payload from the direct (post-less) generation endpoints.
type GenerationResponse struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	Tag       string                 `json:"tag"`
	Source    string                 `json:"source"`
	MediaURLs []string               `json:"media_urls"`
	Failure   map[string]interface{} `json:"failure,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Raw       []byte                 `json:"-"`
}

// CreateGeneration runs a direct AI generation that charges the user and returns media
// without ever creating a post. POST /api/v1/ai/generations.
func CreateGeneration(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	tag string,
	prompt string,
	settings map[string]interface{},
	timeout time.Duration,
) (GenerationResponse, error) {
	body := map[string]interface{}{"tag": tag, "prompt": prompt}
	if len(settings) > 0 {
		body["settings"] = settings
	}

	resp, err := newRequestWithTimeout(accessToken, client, uid, timeout).
		SetHeader("accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post(fmt.Sprintf("%s/api/v1/ai/generations", backendURL))
	if err != nil {
		return GenerationResponse{}, fmt.Errorf("error making request: %w", err)
	}

	var out GenerationResponse
	_ = json.Unmarshal(resp.Body(), &out)
	out.Raw = append(out.Raw[:0], resp.Body()...)

	if resp.StatusCode() != http.StatusCreated && resp.StatusCode() != http.StatusOK {
		msg := out.Error
		if msg == "" {
			msg = string(resp.Body())
		}
		return out, fmt.Errorf("generation request failed (status %d): %s", resp.StatusCode(), msg)
	}
	return out, nil
}

// GetGeneration polls a direct generation by id. GET /api/v1/ai/generations/:id.
func GetGeneration(backendURL, id, accessToken, client, uid string) (GenerationResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s/api/v1/ai/generations/%s", backendURL, id))
	if err != nil {
		return GenerationResponse{}, fmt.Errorf("error making request: %w", err)
	}

	var out GenerationResponse
	_ = json.Unmarshal(resp.Body(), &out)
	out.Raw = append(out.Raw[:0], resp.Body()...)

	if resp.StatusCode() != http.StatusOK {
		return out, fmt.Errorf("status %d: %s", resp.StatusCode(), resp.Body())
	}
	return out, nil
}

// DownloadMedia fetches a generated media URL. Treechat auth headers are sent only
// to same-origin API URLs so signed storage/CDN URLs never receive credentials.
func DownloadMedia(mediaURL, backendURL, accessToken, client, uid string) ([]byte, error) {
	canonicalURL := canonicalizeURL(mediaURL, backendURL)
	if strings.TrimSpace(canonicalURL) == "" {
		return nil, fmt.Errorf("download failed: empty media URL")
	}

	if shouldSendTreechatAuth(canonicalURL, backendURL) {
		resp, err := newRequestWithTimeout(accessToken, client, uid, 60*time.Second).Get(canonicalURL)
		if err != nil {
			return nil, fmt.Errorf("error downloading media: %w", err)
		}
		if resp.StatusCode() != http.StatusOK {
			return nil, fmt.Errorf("download failed (status %d)", resp.StatusCode())
		}
		return resp.Body(), nil
	}

	request, err := http.NewRequest(http.MethodGet, canonicalURL, nil)
	if err != nil {
		return nil, fmt.Errorf("error preparing media download: %w", err)
	}
	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("error downloading media: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download failed (status %d)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading media response: %w", err)
	}
	return data, nil
}

func shouldSendTreechatAuth(requestURL, backendURL string) bool {
	parsedRequestURL, err := neturl.Parse(requestURL)
	if err != nil || parsedRequestURL.Scheme == "" || parsedRequestURL.Host == "" {
		return false
	}

	parsedBackendURL, err := neturl.Parse(backendURL)
	if err != nil || parsedBackendURL.Scheme == "" || parsedBackendURL.Host == "" {
		return false
	}

	return strings.EqualFold(parsedRequestURL.Scheme, parsedBackendURL.Scheme) &&
		strings.EqualFold(parsedRequestURL.Host, parsedBackendURL.Host) &&
		strings.HasPrefix(parsedRequestURL.EscapedPath(), "/api/")
}
