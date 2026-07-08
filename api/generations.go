package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"os"
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

// GenerationActionInfo describes one AI action available to the direct generation endpoint and what it accepts.
type GenerationActionInfo struct {
	Action               string        `json:"action"`
	Tag                  string        `json:"tag,omitempty"`
	Provider             string        `json:"provider"`
	Kind                 string        `json:"kind"` // image | audio | video
	Async                bool          `json:"async"`
	AcceptsReference     bool          `json:"accepts_reference"`
	RequiresReference    bool          `json:"requires_reference"`
	ReferenceKinds       []string      `json:"reference_kinds,omitempty"`
	SupportsInstrumental bool          `json:"supports_instrumental"`
	Settings             []SettingInfo `json:"settings,omitempty"`
	DurationMin          int           `json:"duration_min,omitempty"`
	DurationMax          int           `json:"duration_max,omitempty"`
	Inputs               []string      `json:"inputs,omitempty"`
}

// TagInfo is kept as a source-compatible alias for older callers.
type TagInfo = GenerationActionInfo

// SettingInfo describes one backend-advertised direct generation setting.
type SettingInfo struct {
	Name          string      `json:"name"`
	Type          string      `json:"type,omitempty"`
	Description   string      `json:"description,omitempty"`
	Default       interface{} `json:"default,omitempty"`
	Min           int         `json:"min,omitempty"`
	Max           int         `json:"max,omitempty"`
	AllowedValues []int       `json:"allowed_values,omitempty"`
}

// CreateGeneration runs a direct AI generation that charges the user and returns media
// without ever creating a post. POST /api/v1/ai/generations.
func CreateGeneration(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	action string,
	prompt string,
	settings map[string]interface{},
	paymentMode string,
	quote bool,
	timeout time.Duration,
) (GenerationResponse, error) {
	actionRequest := map[string]interface{}{
		"kind":             "model",
		"action":           action,
		"prompt":           prompt,
		"generation_count": 1,
	}
	body := map[string]interface{}{
		"action_key":     action,
		"prompt":         prompt,
		"action_request": actionRequest,
	}
	if len(settings) > 0 {
		body["settings"] = settings
		actionRequest["settings"] = settings
	}
	if strings.TrimSpace(paymentMode) != "" {
		body["payment_mode"] = paymentMode
		actionRequest["payment_mode"] = paymentMode
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
	out.normalizeAction()
	out.normalizeMediaOutputs()

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
	out.normalizeAction()
	out.normalizeMediaOutputs()

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
	ID          string `json:"id"`
	URL         string `json:"url"`
	ContentType string `json:"content_type,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Error       string `json:"error,omitempty"`
}

// UploadReference uploads a local file to use as a model reference and returns its presigned URL.
// POST /api/v1/ai/generations/references (multipart). The file is held on a direct AiRun; no charge.
func UploadReference(backendURL, accessToken, client, uid, filePath string) (ReferenceUploadResponse, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ReferenceUploadResponse{}, fmt.Errorf("reading reference file %s: %w", filePath, err)
	}
	contentType := DetectFileContentType(filePath, data)
	filename := filepath.Base(filePath)

	// Prefer S3 direct upload: register a reference blob (the server keeps an extensioned key so the
	// presigned URL path carries the file extension for extension-checking providers), PUT the bytes
	// straight to S3, then attach by signed_id. Fall back to a raw multipart file upload when the
	// direct-upload endpoint is unavailable (older backend) or any step fails.
	if out, ok := uploadReferenceDirect(backendURL, accessToken, client, uid, filename, contentType, data); ok {
		return out, nil
	}

	uploads := []MultipartFile{{FieldName: "file", FileName: filename, ContentType: contentType, Content: data}}
	resp, err := postRawMultipart(backendURL, "/api/v1/ai/generations/references", accessToken, client, uid, neturl.Values{}, uploads)
	if err != nil {
		return ReferenceUploadResponse{}, err
	}
	return parseReferenceUploadResponse(resp.Body(), contentType)
}

// uploadReferenceDirect uploads the reference file straight to S3 via the reference direct-upload
// registration endpoint and attaches it by signed_id. Returns (response, true) on success, or
// (_, false) so the caller can fall back to a raw multipart upload.
func uploadReferenceDirect(backendURL, accessToken, client, uid, filename, contentType string, data []byte) (ReferenceUploadResponse, bool) {
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	body := map[string]interface{}{
		"filename":     filename,
		"content_type": ct,
		"byte_size":    len(data),
		"checksum":     md5Base64(data),
	}
	signedID, err := registerDirectUploadAndPut(backendURL, accessToken, client, uid, "/api/v1/ai/generations/references/direct_upload", body, data)
	if err != nil || signedID == "" {
		return ReferenceUploadResponse{}, false
	}
	resp, err := postRawMultipart(backendURL, "/api/v1/ai/generations/references", accessToken, client, uid, neturl.Values{"signed_id": {signedID}}, nil)
	if err != nil {
		return ReferenceUploadResponse{}, false
	}
	out, err := parseReferenceUploadResponse(resp.Body(), contentType)
	if err != nil || out.URL == "" {
		return ReferenceUploadResponse{}, false
	}
	return out, true
}

func parseReferenceUploadResponse(body []byte, contentType string) (ReferenceUploadResponse, error) {
	var out ReferenceUploadResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return ReferenceUploadResponse{}, fmt.Errorf("parsing reference upload response: %w", err)
	}
	if out.URL == "" {
		msg := out.Error
		if msg == "" {
			msg = SafeResponseBody(body)
		}
		return out, fmt.Errorf("reference upload returned no url: %s", msg)
	}
	if out.ContentType == "" {
		out.ContentType = contentType
	}
	return out, nil
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

func (info *GenerationActionInfo) normalizeAction() {
	if strings.TrimSpace(info.Action) == "" {
		info.Action = info.Tag
	}
	if strings.TrimSpace(info.Tag) == "" {
		info.Tag = info.Action
	}
}

// ListGenerationActions fetches the AI actions the direct generation endpoint supports and what each
// accepts. It prefers GET /api/v1/ai/generations/actions and falls back to /tags for older backends.
func ListGenerationActions(backendURL, accessToken, client, uid string) ([]GenerationActionInfo, error) {
	actions, statusCode, err := listGenerationActionsAt(backendURL, accessToken, client, uid, "/api/v1/ai/generations/actions")
	if err == nil {
		return actions, nil
	}
	if statusCode != http.StatusNotFound {
		return nil, err
	}
	actions, _, err = listGenerationActionsAt(backendURL, accessToken, client, uid, "/api/v1/ai/generations/tags")
	return actions, err
}

// ListGenerationTags is kept as a compatibility wrapper for older callers.
func ListGenerationTags(backendURL, accessToken, client, uid string) ([]TagInfo, error) {
	return ListGenerationActions(backendURL, accessToken, client, uid)
}

func listGenerationActionsAt(backendURL, accessToken, client, uid, path string) ([]GenerationActionInfo, int, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s%s", backendURL, path))
	if err != nil {
		return nil, 0, fmt.Errorf("error making request: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, resp.StatusCode(), fmt.Errorf("status %d: %s", resp.StatusCode(), SafeResponseBody(resp.Body()))
	}

	actions, err := parseGenerationActions(resp.Body())
	if err != nil {
		return nil, resp.StatusCode(), err
	}
	return actions, resp.StatusCode(), nil
}

func parseGenerationActions(body []byte) ([]GenerationActionInfo, error) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err == nil {
		if raw, ok := envelope["actions"]; ok {
			var actions []GenerationActionInfo
			if err := json.Unmarshal(raw, &actions); err != nil {
				return nil, fmt.Errorf("parsing actions response: %w", err)
			}
			normalizeGenerationActions(actions)
			return actions, nil
		}
		if raw, ok := envelope["tags"]; ok {
			var actions []GenerationActionInfo
			if err := json.Unmarshal(raw, &actions); err != nil {
				return nil, fmt.Errorf("parsing tags response: %w", err)
			}
			normalizeGenerationActions(actions)
			return actions, nil
		}
	}

	var bare []GenerationActionInfo
	if err := json.Unmarshal(body, &bare); err != nil {
		return nil, fmt.Errorf("parsing actions response: %w", err)
	}
	normalizeGenerationActions(bare)
	return bare, nil
}

func normalizeGenerationActions(actions []GenerationActionInfo) {
	for index := range actions {
		actions[index].normalizeAction()
	}
}
