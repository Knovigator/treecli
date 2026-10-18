package api

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type ResolvedMediaLink struct {
	MediaLink    string
	RawMediaLink string
}

// GetMessages fetches messages from the API and returns them
func GetMessages(backendURL, accessToken, client, uid string, messageIDs []string) (MessagesResponse, error) {
	var out MessagesResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "messages", map[string]interface{}{"ids": messageIDs}, &out)
	out.Raw = raw
	return out, err
}

func GetThread(backendURL, threadID, accessToken, client, uid string) (ThreadResponse, error) {
	var out ThreadResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "thread", map[string]interface{}{"id": threadID}, &out)
	out.Raw = raw
	return out, err
}

func GetNotifications(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
	page int,
	unseenOnly bool,
	clear bool,
) (NotificationsResponse, error) {
	var out NotificationsResponse
	tool := "treechat_read"
	args := map[string]interface{}{"space_id": spaceID, "page": page, "unseen": unseenOnly}
	envelope := map[string]interface{}{"operation": "notifications", "arguments": args}
	if clear {
		tool = "clear_notifications"
		delete(envelope, "operation")
	}
	raw, err := SharedOperation(backendURL, accessToken, client, uid, tool, envelope)
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Raw = raw
	return out, err
}

func GetNotificationsCount(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
) (NotificationsCountResponse, error) {
	var out NotificationsCountResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "notification_count", map[string]interface{}{"space_id": spaceID}, &out)
	out.Raw = raw
	return out, err
}

func GetUpvalueHistory(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	page int,
	perPage int,
) (UpvalueHistoryResponse, error) {
	var out UpvalueHistoryResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "upvalues", map[string]interface{}{"page": page, "per_page": perPage}, &out)
	out.Raw = raw
	return out, err
}

func GetAnswer(backendURL, answerID, accessToken, client, uid string) (AnswerResponse, error) {
	var out AnswerResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "post", map[string]interface{}{"id": answerID}, &out)
	out.Raw = raw
	return out, err
}

func GetUpvaluedContentLeaderboard(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	startDate string,
	endDate string,
	limit int,
) (UpvaluedContentLeaderboardResponse, error) {
	var out UpvaluedContentLeaderboardResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "leaderboard", map[string]interface{}{"start_date": startDate, "end_date": endDate, "limit": limit}, &out)
	out.Raw = raw
	return out, err
}

func ListAIModels(backendURL, accessToken, client, uid string) (AIModelsResponse, error) {
	var out AIModelsResponse
	_, err := sharedRead(backendURL, accessToken, client, uid, "models", map[string]interface{}{}, &out)
	return out, err
}

func ListTeams(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
	publicOnly bool,
) (TeamsResponse, error) {
	var out TeamsResponse
	operation := "streams"
	if publicOnly {
		operation = "public_streams"
	}
	raw, err := sharedRead(backendURL, accessToken, client, uid, operation, map[string]interface{}{"space_id": spaceID}, &out)
	out.Raw = raw
	return out, err
}

func ListPublicTeams(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
) (TeamsResponse, error) {
	var out TeamsResponse
	raw, err := sharedRead(backendURL, accessToken, client, uid, "public_streams", map[string]interface{}{"space_id": spaceID}, &out)
	out.Raw = raw
	return out, err
}

func newRequestWithTimeout(accessToken, client, uid string, timeout time.Duration) *resty.Request {
	return scopedRequest(context.Background(), "", accessToken, client, uid, timeout)
}

// directUploadTarget is the response from a direct-upload registration endpoint: a blob signed_id
// plus the presigned PUT URL/headers the client uses to send the bytes straight to storage.
type directUploadTarget struct {
	SignedID     string `json:"signed_id"`
	DirectUpload struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	} `json:"direct_upload"`
}

func md5Base64(data []byte) string {
	sum := md5.Sum(data)
	return base64.StdEncoding.EncodeToString(sum[:])
}

func putDirectUpload(du directUploadTarget, content []byte) (string, error) {
	if du.SignedID == "" || du.DirectUpload.URL == "" {
		return "", fmt.Errorf("direct-upload: missing signed_id or url")
	}

	// PUT the bytes straight to S3 with a bare client and ONLY the headers the backend returned:
	// the presigned URL carries its own auth, and adding our access-token headers would break the
	// S3 signature.
	MarkNetwork()
	putClient := resty.New()
	putClient.SetTimeout(15 * time.Minute)
	putReq := putClient.R()
	for k, v := range du.DirectUpload.Headers {
		putReq.SetHeader(k, v)
	}
	putResp, err := putReq.SetBody(content).Put(du.DirectUpload.URL)
	if err != nil {
		return "", fmt.Errorf("direct-upload PUT: %w", err)
	}
	if putResp.StatusCode() >= 300 {
		return "", fmt.Errorf("direct-upload PUT: status %d", putResp.StatusCode())
	}

	return du.SignedID, nil
}

func ResolveAnswerMediaLinks(answer Answer, fallbackBase string) []ResolvedMediaLink {
	canonicalURLs := answer.CanonicalMediaURLs(fallbackBase)
	resolvedLinks := make([]ResolvedMediaLink, 0, len(canonicalURLs))
	seenPairs := map[string]bool{}

	for _, mediaURL := range canonicalURLs {
		canonicalURL := canonicalizeURL(mediaURL, fallbackBase)
		if strings.TrimSpace(canonicalURL) == "" {
			continue
		}

		resolvedURL := ResolveFinalURL(canonicalURL, fallbackBase)
		pairKey := canonicalURL + "\n" + resolvedURL
		if seenPairs[pairKey] {
			continue
		}

		seenPairs[pairKey] = true
		resolvedLinks = append(resolvedLinks, ResolvedMediaLink{
			MediaLink:    canonicalURL,
			RawMediaLink: resolvedURL,
		})
	}

	return resolvedLinks
}

func ResolveFinalURL(rawURL string, fallbackBase string) string {
	canonicalURL := canonicalizeURL(rawURL, fallbackBase)
	if canonicalURL == "" {
		return ""
	}

	canonicalURL = preferredResolvableURL(canonicalURL, fallbackBase)

	if !shouldResolveFinalURL(canonicalURL, fallbackBase) {
		return canonicalURL
	}

	request, err := http.NewRequest(http.MethodHead, canonicalURL, nil)
	if err != nil {
		return canonicalURL
	}

	MarkNetwork()
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}

			return nil
		},
	}

	response, err := client.Do(request)
	if response != nil {
		defer response.Body.Close()
		finalURL := strings.TrimSpace(response.Request.URL.String())
		if finalURL != "" {
			return finalURL
		}
	}

	if err != nil {
		return canonicalURL
	}

	return canonicalURL
}

func shouldResolveFinalURL(candidate string, fallbackBase string) bool {
	candidateURL, err := neturl.Parse(candidate)
	if err != nil {
		return false
	}

	if candidateURL.Scheme != "http" && candidateURL.Scheme != "https" {
		return false
	}

	if strings.Contains(candidateURL.Path, "/api/v1/blob/") ||
		strings.Contains(candidateURL.Path, "/api/v1/answers/") ||
		strings.Contains(candidateURL.Path, "/rails/active_storage/") {
		return true
	}

	fallbackURL, err := neturl.Parse(strings.TrimSpace(fallbackBase))
	if err != nil {
		return false
	}

	return fallbackURL.Host != "" && strings.EqualFold(candidateURL.Host, fallbackURL.Host)
}

func preferredResolvableURL(candidate string, fallbackBase string) string {
	candidateURL, err := neturl.Parse(candidate)
	if err != nil {
		return candidate
	}

	if !strings.Contains(candidateURL.Path, "/api/v1/blob/") &&
		!strings.Contains(candidateURL.Path, "/api/v1/answers/") &&
		!strings.Contains(candidateURL.Path, "/rails/active_storage/") {
		return candidate
	}

	fallbackURL, err := neturl.Parse(strings.TrimSpace(fallbackBase))
	if err != nil || fallbackURL.Host == "" {
		return candidate
	}

	candidateURL.Scheme = fallbackURL.Scheme
	candidateURL.Host = fallbackURL.Host
	return candidateURL.String()
}
