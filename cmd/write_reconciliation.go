package cmd

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/Knovigator/treecli/api"
)

func shouldReconcileWrite(err error) bool {
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		// Transport and response-decoding errors are ambiguous: the write may have landed.
		return true
	}

	switch httpErr.StatusCode {
	case http.StatusRequestTimeout,
		http.StatusConflict,
		http.StatusUnprocessableEntity,
		http.StatusTooEarly,
		http.StatusTooManyRequests:
		return true
	default:
		return httpErr.StatusCode >= http.StatusInternalServerError
	}
}

func reconcileAnswerWrite(
	profile profileConfig,
	answerID string,
	questID string,
	spaceID string,
	content string,
	deltaJSON string,
) (api.CreateAnswerResponse, bool) {
	result, err := api.GetAnswer(
		profile.BackendURL,
		answerID,
		profile.AccessToken,
		profile.Client,
		profile.UID,
	)
	if err != nil || !sameAnswerWrite(result.Answer, profile.CurrentUserID, answerID, questID, spaceID, content, deltaJSON) {
		return api.CreateAnswerResponse{}, false
	}

	return api.CreateAnswerResponse{
		Answer: result.Answer,
		Raw:    result.Raw,
	}, true
}

func sameAnswerWrite(
	answer api.Answer,
	userID string,
	answerID string,
	questID string,
	spaceID string,
	content string,
	deltaJSON string,
) bool {
	return strings.TrimSpace(userID) != "" &&
		answer.ID == answerID &&
		answer.UserID == userID &&
		answer.QuestID == questID &&
		answer.SpaceID == spaceID &&
		answer.Content == content &&
		sameWriteJSON(answer.DeltaJSON, deltaJSON)
}

func reconcileQuestWrite(
	profile profileConfig,
	questID string,
	spaceID string,
	content string,
	deltaJSON string,
	url string,
	bareQuestJSON bool,
) (api.CreateQuestResponse, bool) {
	result, err := api.GetThread(
		profile.BackendURL,
		questID,
		profile.AccessToken,
		profile.Client,
		profile.UID,
	)
	if err != nil || !sameQuestWrite(result.Quest, profile.CurrentUserID, questID, spaceID, content, deltaJSON, url) {
		return api.CreateQuestResponse{}, false
	}

	raw := result.Raw
	if bareQuestJSON {
		var envelope map[string]json.RawMessage
		if json.Unmarshal(result.Raw, &envelope) != nil || len(envelope["quest"]) == 0 {
			return api.CreateQuestResponse{}, false
		}
		raw = envelope["quest"]
	}

	return api.CreateQuestResponse{Quest: result.Quest, Raw: raw}, true
}

func sameQuestWrite(
	quest api.Quest,
	userID string,
	questID string,
	spaceID string,
	content string,
	deltaJSON string,
	url string,
) bool {
	if strings.TrimSpace(userID) == "" || quest.ID != questID || quest.SpaceID != spaceID {
		return false
	}

	ownerID := quest.UserID
	storedContent := quest.Content
	if quest.Parent != nil {
		if ownerID == "" {
			ownerID = quest.Parent.UserID
		}
		storedContent = quest.Parent.Content
	}
	if ownerID != userID || storedContent != content || quest.Parent == nil ||
		!sameWriteJSON(quest.Parent.DeltaJSON, deltaJSON) {
		return false
	}

	if url == "" {
		return true
	}
	return quest.Parent != nil && quest.Parent.URL != nil && quest.Parent.URL.Address == url
}

func sameWriteJSON(stored json.RawMessage, requested string) bool {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return len(stored) == 0 || string(stored) == "null"
	}

	storedValue, storedOK := comparableJSON(stored)
	requestedValue, requestedOK := comparableJSON([]byte(requested))
	return storedOK && requestedOK && reflect.DeepEqual(storedValue, requestedValue)
}

func comparableJSON(raw []byte) (interface{}, bool) {
	var value interface{}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}

	if encoded, ok := value.(string); ok {
		decoder = json.NewDecoder(strings.NewReader(encoded))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
	}

	return value, true
}
