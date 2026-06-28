package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

type MultipartFile struct {
	FieldName string
	FileName  string
	Content   []byte
}

type CreateQuestRequest struct {
	QuestID            string
	ParentAnswerID     string
	SpaceID            string
	Content            string
	DeltaJSON          string
	ActionRequestsJSON string
	MessageType        string
	ThreadType         string
	TeamID             string
	Public             *bool
	Private            *bool
	Uploads            []MultipartFile
}

type CreateAnswerRequest struct {
	AnswerID           string
	ChildQuestID       string
	QuestID            string
	SpaceID            string
	UserID             string
	Content            string
	DeltaJSON          string
	ActionRequestsJSON string
	MessageType        string
	Uploads            []MultipartFile
}

type CreateClipQuestRequest struct {
	URL         string
	Content     string
	DeltaJSON   string
	Image       []byte
	Video       []byte
	File        []byte
	Title       string
	Destination map[string]interface{}
}

type ResolvedMediaLink struct {
	MediaLink    string
	RawMediaLink string
}

// GetMessages fetches messages from the API and returns them
func GetMessages(backendURL, accessToken, client, uid string, messageIDs []string) (MessagesResponse, error) {
	requestBody := map[string][]string{"ids": messageIDs}

	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetBody(requestBody).
		Post(fmt.Sprintf("%s/api/v1/answers/bulk", backendURL))

	if err != nil {
		return MessagesResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return MessagesResponse{}, fmt.Errorf("error: received status code %d. Response body: %s", resp.StatusCode(), resp.Body())
	}

	var messagesInfo MessagesResponse
	err = json.Unmarshal(resp.Body(), &messagesInfo)
	if err != nil {
		return MessagesResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	messagesInfo.Raw = append(messagesInfo.Raw[:0], resp.Body()...)

	return messagesInfo, nil
}

func GetThread(backendURL, threadID, accessToken, client, uid string) (ThreadResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s/api/v1/quests/%s", backendURL, threadID))

	if err != nil {
		return ThreadResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return ThreadResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var threadInfo ThreadResponse
	err = json.Unmarshal(resp.Body(), &threadInfo)
	if err != nil {
		return ThreadResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	threadInfo.Raw = append(threadInfo.Raw[:0], resp.Body()...)

	return threadInfo, nil
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
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetQueryParam("notifications", "true")

	if strings.TrimSpace(spaceID) != "" {
		request.SetQueryParam("space_id", strings.TrimSpace(spaceID))
	}
	if page > 0 {
		request.SetQueryParam("page", fmt.Sprintf("%d", page))
	}
	if unseenOnly {
		request.SetQueryParam("unseen", "true")
	}
	if clear {
		request.SetQueryParam("clear", "true")
	}

	resp, err := request.Get(fmt.Sprintf("%s/api/v1/quests", backendURL))
	if err != nil {
		return NotificationsResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return NotificationsResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var notifications NotificationsResponse
	err = json.Unmarshal(resp.Body(), &notifications)
	if err != nil {
		return NotificationsResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	notifications.Raw = append(notifications.Raw[:0], resp.Body()...)

	return notifications, nil
}

func GetNotificationsCount(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
) (NotificationsCountResponse, error) {
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json")

	if strings.TrimSpace(spaceID) != "" {
		request.SetQueryParam("space_id", strings.TrimSpace(spaceID))
	}

	resp, err := request.Get(fmt.Sprintf("%s/api/v1/notifications/count", backendURL))
	if err != nil {
		return NotificationsCountResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return NotificationsCountResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var count NotificationsCountResponse
	err = json.Unmarshal(resp.Body(), &count)
	if err != nil {
		return NotificationsCountResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	count.Raw = append(count.Raw[:0], resp.Body()...)

	return count, nil
}

func GetAnswer(backendURL, answerID, accessToken, client, uid string) (AnswerResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s/api/v1/answers/%s", backendURL, answerID))

	if err != nil {
		return AnswerResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return AnswerResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var answerInfo AnswerResponse
	err = json.Unmarshal(resp.Body(), &answerInfo)
	if err != nil {
		return AnswerResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	answerInfo.Raw = append(answerInfo.Raw[:0], resp.Body()...)

	return answerInfo, nil
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
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json")

	if strings.TrimSpace(startDate) != "" {
		request.SetQueryParam("start_date", strings.TrimSpace(startDate))
	}
	if strings.TrimSpace(endDate) != "" {
		request.SetQueryParam("end_date", strings.TrimSpace(endDate))
	}
	if limit > 0 {
		request.SetQueryParam("limit", fmt.Sprintf("%d", limit))
	}

	resp, err := request.Get(fmt.Sprintf("%s/api/v1/leaderboards/upvalued_content", backendURL))
	if err != nil {
		return UpvaluedContentLeaderboardResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return UpvaluedContentLeaderboardResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var leaderboard UpvaluedContentLeaderboardResponse
	err = json.Unmarshal(resp.Body(), &leaderboard)
	if err != nil {
		return UpvaluedContentLeaderboardResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	leaderboard.Raw = append(leaderboard.Raw[:0], resp.Body()...)

	return leaderboard, nil
}

func ListAIModels(backendURL, accessToken, client, uid string) (AIModelsResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		Get(fmt.Sprintf("%s/api/v1/ai_models", backendURL))
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var models AIModelsResponse
	err = json.Unmarshal(resp.Body(), &models)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	return models, nil
}

func ListTeams(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
	publicOnly bool,
) (TeamsResponse, error) {
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetQueryParam("space_id", spaceID)

	if publicOnly {
		request.SetQueryParam("public", "true")
	} else {
		request.SetQueryParam("include_hidden", "false")
	}

	resp, err := request.Get(fmt.Sprintf("%s/api/v1/teams", backendURL))
	if err != nil {
		return TeamsResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return TeamsResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var teamsResponse TeamsResponse
	err = json.Unmarshal(resp.Body(), &teamsResponse)
	if err != nil {
		return TeamsResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	teamsResponse.Raw = append(teamsResponse.Raw[:0], resp.Body()...)

	return teamsResponse, nil
}

func ListPublicTeams(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	spaceID string,
) (TeamsResponse, error) {
	resp, err := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetQueryParam("space_id", spaceID).
		Get(fmt.Sprintf("%s/api/v1/teams/public", backendURL))
	if err != nil {
		return TeamsResponse{}, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return TeamsResponse{}, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	var teamsResponse TeamsResponse
	err = json.Unmarshal(resp.Body(), &teamsResponse)
	if err != nil {
		return TeamsResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	teamsResponse.Raw = append(teamsResponse.Raw[:0], resp.Body()...)

	return teamsResponse, nil
}

func CreateQuest(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	request CreateQuestRequest,
) (CreateQuestResponse, error) {
	form := neturl.Values{}
	form.Set("id", request.QuestID)
	form.Set("space_id", request.SpaceID)
	form.Set("parent_attributes[id]", request.ParentAnswerID)
	form.Set("parent_attributes[content]", request.Content)
	if request.DeltaJSON != "" {
		form.Set("parent_attributes[delta_json]", request.DeltaJSON)
	}
	if request.ActionRequestsJSON != "" {
		form.Set("parent_attributes[action_requests]", request.ActionRequestsJSON)
	}

	if request.MessageType != "" {
		form.Set("parent_attributes[message_type]", request.MessageType)
	}
	if request.ThreadType != "" {
		form.Set("thread_type", request.ThreadType)
	}
	if request.TeamID != "" {
		form.Set("team_id", request.TeamID)
	}
	if request.Public != nil {
		form.Set("public", fmt.Sprintf("%t", *request.Public))
	}
	if request.Private != nil {
		form.Set("private", fmt.Sprintf("%t", *request.Private))
	}

	resp, err := postMultipart(
		backendURL,
		"/api/v1/quests",
		accessToken,
		client,
		uid,
		form,
		request.Uploads,
	)
	if err != nil {
		return CreateQuestResponse{}, err
	}

	var questResponse CreateQuestResponse
	err = json.Unmarshal(resp.Body(), &questResponse)
	if err != nil {
		return CreateQuestResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	questResponse.Raw = append(questResponse.Raw[:0], resp.Body()...)

	return questResponse, nil
}

func CreateAnswer(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	request CreateAnswerRequest,
) (CreateAnswerResponse, error) {
	form := neturl.Values{}
	form.Set("id", request.AnswerID)
	form.Set("space_id", request.SpaceID)
	form.Set("quest_id", request.QuestID)
	if request.UserID != "" {
		form.Set("user_id", request.UserID)
	}
	form.Set("content", request.Content)
	if request.DeltaJSON != "" {
		form.Set("delta_json", request.DeltaJSON)
	}
	if request.ActionRequestsJSON != "" {
		form.Set("action_requests", request.ActionRequestsJSON)
	}

	if request.ChildQuestID != "" {
		form.Set("child_quest_id", request.ChildQuestID)
	}
	if request.MessageType != "" {
		form.Set("message_type", request.MessageType)
	}

	resp, err := postMultipart(
		backendURL,
		"/api/v1/answers",
		accessToken,
		client,
		uid,
		form,
		request.Uploads,
	)
	if err != nil {
		return CreateAnswerResponse{}, err
	}

	var answerResponse CreateAnswerResponse
	err = json.Unmarshal(resp.Body(), &answerResponse)
	if err != nil {
		return CreateAnswerResponse{}, fmt.Errorf("error parsing response: %v", err)
	}
	answerResponse.Raw = append(answerResponse.Raw[:0], resp.Body()...)

	return answerResponse, nil
}

func CreateClipQuest(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	request CreateClipQuestRequest,
) (CreateQuestResponse, error) {
	form := neturl.Values{}

	if request.URL != "" {
		form.Set("quest[answers_attributes][0][url_attributes][address]", request.URL)
		form.Set("quest[answers_attributes][0][url_attributes][title]", request.Title)
	} else if request.Title != "" {
		form.Set("quest[answers_attributes][0][url_attributes][title]", request.Title)
	}

	if request.Content != "" {
		form.Set("quest[answers_attributes][0][content]", request.Content)
	}
	if request.DeltaJSON != "" {
		form.Set("quest[answers_attributes][0][delta_json]", request.DeltaJSON)
	}

	if request.Destination != nil {
		if destType, ok := request.Destination["type"].(string); ok {
			form.Set("destination[type]", destType)
		}
		switch destID := request.Destination["id"].(type) {
		case float64:
			form.Set("destination[id]", fmt.Sprintf("%d", int(destID)))
		case string:
			form.Set("destination[id]", destID)
		}
	}

	uploads := []MultipartFile{}
	if len(request.Image) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][images]",
			FileName:  "image",
			Content:   request.Image,
		})
	}
	if len(request.Video) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][recording]",
			FileName:  "video",
			Content:   request.Video,
		})
	}
	if len(request.File) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][files]",
			FileName:  "file",
			Content:   request.File,
		})
	}

	resp, err := postMultipart(backendURL, "/plugin_new/clip", accessToken, client, uid, form, uploads)
	if err != nil {
		return CreateQuestResponse{}, err
	}

	var quest Quest
	err = json.Unmarshal(resp.Body(), &quest)
	if err != nil {
		return CreateQuestResponse{}, fmt.Errorf("error parsing response: %v", err)
	}

	return CreateQuestResponse{
		Quest: quest,
		Raw:   append([]byte(nil), resp.Body()...),
	}, nil
}

func ClipLink(
	backendURL string,
	accessToken string,
	client string,
	uid string,
	url string,
	image []byte,
	video []byte,
	file []byte,
	title string,
	content string,
	destination map[string]interface{},
) (map[string]interface{}, error) {
	clipResponse, err := CreateClipQuest(
		backendURL,
		accessToken,
		client,
		uid,
		CreateClipQuestRequest{
			URL:         url,
			Content:     content,
			Image:       image,
			Video:       video,
			File:        file,
			Title:       title,
			Destination: destination,
		},
	)
	if err != nil {
		return nil, err
	}

	var clipInfo map[string]interface{}
	err = json.Unmarshal(clipResponse.Raw, &clipInfo)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	return clipInfo, nil
}

func newRequest(accessToken, client, uid string) *resty.Request {
	return newRequestWithTimeout(accessToken, client, uid, 10*time.Second)
}

func newRequestWithTimeout(accessToken, client, uid string, timeout time.Duration) *resty.Request {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	restyClient := resty.New()
	restyClient.SetTimeout(timeout)

	return restyClient.R().
		SetHeader("access-token", accessToken).
		SetHeader("client", client).
		SetHeader("uid", uid)
}

func postMultipart(
	backendURL string,
	path string,
	accessToken string,
	client string,
	uid string,
	form neturl.Values,
	uploads []MultipartFile,
) (*resty.Response, error) {
	request := newRequest(accessToken, client, uid).
		SetHeader("accept", "application/json").
		SetFormDataFromValues(form)

	for _, upload := range uploads {
		request.SetFileReader(upload.FieldName, upload.FileName, bytes.NewReader(upload.Content))
	}

	resp, err := request.Post(fmt.Sprintf("%s%s", backendURL, path))
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	return resp, nil
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
