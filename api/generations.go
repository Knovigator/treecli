package api

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"path/filepath"
	"strings"
	"time"
)

// GenerationResponse is the payload from the direct (post-less) generation endpoints.
type GenerationResponse struct {
	ID           string                 `json:"id"`
	Status       string                 `json:"status"`
	Tag          string                 `json:"tag"`
	Action       string                 `json:"action,omitempty"`
	Source       string                 `json:"source"`
	Provider     string                 `json:"provider,omitempty"`
	PaymentMode  string                 `json:"payment_mode,omitempty"`
	MediaURLs    []string               `json:"media_urls"`
	MediaOutputs []GenerationMedia      `json:"media_outputs,omitempty"`
	AmountSats   int64                  `json:"amount_sats,omitempty"`
	AmountUSD    float64                `json:"amount_usd,omitempty"`
	Quote        *GenerationQuote       `json:"quote,omitempty"`
	Failure      map[string]interface{} `json:"failure,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Raw          []byte                 `json:"-"`
}

// GenerationMedia describes one generated media artifact.
type GenerationMedia struct {
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

// GenerationQuote is the price for a generation, returned when quote=true (no media is produced).
type GenerationQuote struct {
	AmountSats  int64   `json:"amount_sats"`
	AmountUSD   float64 `json:"amount_usd"`
	Action      string  `json:"action,omitempty"`
	Tag         string  `json:"tag"`
	Provider    string  `json:"provider,omitempty"`
	PaymentMode string  `json:"payment_mode,omitempty"`
}

// GetGeneration polls a direct generation by id. GET /api/v1/ai/generations/:id.
func GetGeneration(backendURL, id, accessToken, client, uid string) (GenerationResponse, error) {
	raw, err := SharedOperation(backendURL, accessToken, client, uid, "get_generation", map[string]interface{}{"generation_id": id})
	if err != nil {
		return GenerationResponse{}, err
	}
	return DecodeGeneration(raw)
}

// DownloadMedia fetches a generated media URL. Treechat auth headers are sent only
// to same-origin API URLs so signed storage/CDN URLs never receive credentials.
func DownloadMedia(mediaURL, backendURL, accessToken, client, uid string) ([]byte, error) {
	canonicalURL := canonicalizeURL(mediaURL, backendURL)
	if strings.TrimSpace(canonicalURL) == "" {
		return nil, fmt.Errorf("download failed: empty media URL")
	}

	// OAuth (connect) bearers work only on /mcp, and the media URLs it returns are
	// signed, so those downloads carry no credential (none that could expire).
	if client != OAuthClient && shouldSendTreechatAuth(canonicalURL, backendURL) {
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
	MarkNetwork()
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

// DetectFileContentType returns a stable media MIME type for local upload paths.
func DetectFileContentType(filePath string, data []byte) string {
	if fromExt := strings.TrimSpace(mime.TypeByExtension(strings.ToLower(filepath.Ext(filePath)))); fromExt != "" {
		return strings.Split(fromExt, ";")[0]
	}
	return http.DetectContentType(data)
}

func (out *GenerationResponse) normalizeMediaOutputs() {
	if len(out.MediaURLs) == 0 && len(out.MediaOutputs) > 0 {
		for _, media := range out.MediaOutputs {
			if strings.TrimSpace(media.URL) != "" {
				out.MediaURLs = append(out.MediaURLs, media.URL)
			}
		}
	}
	if len(out.MediaURLs) == 0 {
		return
	}
	if len(out.MediaOutputs) > 0 {
		return
	}
	out.MediaOutputs = make([]GenerationMedia, 0, len(out.MediaURLs))
	for _, mediaURL := range out.MediaURLs {
		if strings.TrimSpace(mediaURL) == "" {
			continue
		}
		out.MediaOutputs = append(out.MediaOutputs, GenerationMedia{URL: mediaURL})
	}
}

func (out *GenerationResponse) normalizeAction() {
	if strings.TrimSpace(out.Tag) == "" {
		out.Tag = out.Action
	}
	if strings.TrimSpace(out.Action) == "" {
		out.Action = out.Tag
	}
}
