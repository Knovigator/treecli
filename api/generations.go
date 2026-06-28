package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
) (GenerationResponse, error) {
	body := map[string]interface{}{"tag": tag, "prompt": prompt}
	if len(settings) > 0 {
		body["settings"] = settings
	}

	resp, err := newRequest(accessToken, client, uid).
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

// DownloadMedia fetches a generated media URL, sending auth headers in case the URL is an
// app-served (non-CDN) media endpoint rather than a pre-signed object URL.
func DownloadMedia(mediaURL, accessToken, client, uid string) ([]byte, error) {
	resp, err := newRequest(accessToken, client, uid).
		Get(mediaURL)
	if err != nil {
		return nil, fmt.Errorf("error downloading media: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("download failed (status %d)", resp.StatusCode())
	}
	return resp.Body(), nil
}
