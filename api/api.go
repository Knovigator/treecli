package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-resty/resty/v2"
)

// GetMessages fetches messages from the API and returns them
func GetMessages(backendURL, accessToken, client, uid string, messageIDs []string) (map[string]interface{}, error) {
	// create a new resty client
	restyClient := resty.New()

	// prepare query parameters
	queryParams := url.Values{}
	for _, id := range messageIDs {
		queryParams.Add("ids[]", id)
	}

	// convert url.Values to map[string]string
	queryMap := make(map[string]string)
	for key, values := range queryParams {
		if len(values) > 0 {
			queryMap[key] = values[0]
		}
	}

	// make the request
	resp, err := restyClient.R().
		SetQueryParams(queryMap).
		SetHeader("accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("access-token", accessToken).
		SetHeader("client", client).
		SetHeader("uid", uid).
		Get(fmt.Sprintf("%s/api/v1/answers/bulk", backendURL))

	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error: received status code %d. Response body: %s", resp.StatusCode(), resp.Body())
	}

	// parse the response
	var messagesInfo map[string]interface{}
	err = json.Unmarshal(resp.Body(), &messagesInfo)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	return messagesInfo, nil
}

func GetThread(backendURL, threadID, accessToken, client, uid string) (map[string]interface{}, error) {
	// create a new resty client
	restyClient := resty.New()

	// make the request
	resp, err := restyClient.R().
		SetHeader("accept", "application/json").
		SetHeader("access-token", accessToken).
		SetHeader("client", client).
		SetHeader("uid", uid).
		Get(fmt.Sprintf("%s/api/v1/quests/%s", backendURL, threadID))

	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("received status code %d: %s", resp.StatusCode(), resp.Body())
	}

	// parse the response
	var threadInfo map[string]interface{}
	err = json.Unmarshal(resp.Body(), &threadInfo)
	if err != nil {
		return nil, fmt.Errorf("error parsing response: %v", err)
	}

	return threadInfo, nil
}
