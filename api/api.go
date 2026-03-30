package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"time"

	"github.com/go-resty/resty/v2"
)

type MultipartFile struct {
	FieldName string
	FileName  string
	Content   []byte
}

type CreateQuestRequest struct {
	QuestID        string
	ParentAnswerID string
	SpaceID        string
	Content        string
	MessageType    string
	ThreadType     string
	TeamID         string
	Public         *bool
	Private        *bool
	Uploads        []MultipartFile
}

type CreateAnswerRequest struct {
	AnswerID     string
	ChildQuestID string
	QuestID      string
	SpaceID      string
	Content      string
	MessageType  string
	Uploads      []MultipartFile
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
	form.Set("content", request.Content)

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
	form := neturl.Values{}

	if url != "" {
		form.Set("quest[answers_attributes][0][url_attributes][address]", url)
		form.Set("quest[answers_attributes][0][url_attributes][title]", title)
	} else if title != "" {
		form.Set("quest[answers_attributes][0][url_attributes][title]", title)
	}

	if content != "" {
		form.Set("quest[answers_attributes][0][content]", content)
	}

	if destination != nil {
		if destType, ok := destination["type"].(string); ok {
			form.Set("destination[type]", destType)
		}
		switch destID := destination["id"].(type) {
		case float64:
			form.Set("destination[id]", fmt.Sprintf("%d", int(destID)))
		case string:
			form.Set("destination[id]", destID)
		}
	}

	uploads := []MultipartFile{}

	if len(image) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][images]",
			FileName:  "image",
			Content:   image,
		})
	}
	if len(video) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][recording]",
			FileName:  "video",
			Content:   video,
		})
	}
	if len(file) > 0 {
		uploads = append(uploads, MultipartFile{
			FieldName: "quest[answers_attributes][0][files]",
			FileName:  "file",
			Content:   file,
		})
	}

	resp, err := postMultipart(backendURL, "/plugin_new/clip", accessToken, client, uid, form, uploads)
	if err != nil {
		return nil, err
	}

	var clipInfo map[string]interface{}
	err = json.Unmarshal(resp.Body(), &clipInfo)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	return clipInfo, nil
}

func newRequest(accessToken, client, uid string) *resty.Request {
	restyClient := resty.New()
	restyClient.SetTimeout(10 * time.Second)

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
