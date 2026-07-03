package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerationResponse is the payload from the direct (post-less) generation endpoints.
type GenerationResponse struct {
	ID         string                 `json:"id"`
	Status     string                 `json:"status"`
	Tag        string                 `json:"tag"`
	Source     string                 `json:"source"`
	Provider   string                 `json:"provider,omitempty"`
	MediaURLs  []string               `json:"media_urls"`
	AmountSats int64                  `json:"amount_sats,omitempty"`
	AmountUSD  float64                `json:"amount_usd,omitempty"`
	Quote      *GenerationQuote       `json:"quote,omitempty"`
	Failure    map[string]interface{} `json:"failure,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Raw        []byte                 `json:"-"`
}

// GenerationQuote is the price for a generation, returned when quote=true (no media is produced).
type GenerationQuote struct {
	AmountSats int64   `json:"amount_sats"`
	AmountUSD  float64 `json:"amount_usd"`
	Tag        string  `json:"tag"`
	Provider   string  `json:"provider,omitempty"`
}

// TagInfo describes one model tag available to the direct generation endpoint and what it accepts.
type TagInfo struct {
	Tag                  string   `json:"tag"`
	Provider             string   `json:"provider"`
	Kind                 string   `json:"kind"` // image | audio | video
	Async                bool     `json:"async"`
	AcceptsReference     bool     `json:"accepts_reference"`
	SupportsInstrumental bool     `json:"supports_instrumental"`
	DurationMin          int      `json:"duration_min,omitempty"`
	DurationMax          int      `json:"duration_max,omitempty"`
	Inputs               []string `json:"inputs,omitempty"`
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
	quote bool,
	timeout time.Duration,
) (GenerationResponse, error) {
	body := map[string]interface{}{"tag": tag, "prompt": prompt}
	if len(settings) > 0 {
		body["settings"] = settings
	}
	if quote {
		body["quote"] = true
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
			msg = SafeResponseBody(resp.Body())
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
		return out, fmt.Errorf("status %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
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

// ReferenceUploadResponse is returned by the reference-upload endpoint.
type ReferenceUploadResponse struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Error string `json:"error,omitempty"`
}

// UploadReference uploads a local file to use as a model reference and returns its presigned URL.
// POST /api/v1/ai/generations/references (multipart). The file is held on a direct AiRun; no charge.
func UploadReference(backendURL, accessToken, client, uid, filePath string) (ReferenceUploadResponse, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ReferenceUploadResponse{}, fmt.Errorf("reading reference file %s: %w", filePath, err)
	}
	uploads := []MultipartFile{{FieldName: "file", FileName: filepath.Base(filePath), Content: data}}

	resp, err := postMultipart(
		backendURL,
		"/api/v1/ai/generations/references",
		accessToken,
		client,
		uid,
		neturl.Values{},
		uploads,
	)
	if err != nil {
		return ReferenceUploadResponse{}, err
	}

	var out ReferenceUploadResponse
	if err := json.Unmarshal(resp.Body(), &out); err != nil {
		return ReferenceUploadResponse{}, fmt.Errorf("parsing reference upload response: %w", err)
	}
	if out.URL == "" {
		msg := out.Error
		if msg == "" {
			msg = SafeResponseBody(resp.Body())
		}
		return out, fmt.Errorf("reference upload returned no url: %s", msg)
	}
	return out, nil
}

// ListGenerationTags fetches the model tags the direct generation endpoint supports and what each
// accepts. GET /api/v1/ai/generations/tags.
func ListGenerationTags(backendURL, accessToken, client, uid string) ([]TagInfo, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s/api/v1/ai/generations/tags", backendURL))
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
	}

	// Accept either a bare array or {"tags": [...]}.
	var wrapped struct {
		Tags []TagInfo `json:"tags"`
	}
	if err := json.Unmarshal(resp.Body(), &wrapped); err == nil && len(wrapped.Tags) > 0 {
		return wrapped.Tags, nil
	}
	var bare []TagInfo
	if err := json.Unmarshal(resp.Body(), &bare); err != nil {
		return nil, fmt.Errorf("parsing tags response: %w", err)
	}
	return bare, nil
}
