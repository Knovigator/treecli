package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func GetMessages(cmd *cobra.Command, args []string) {
	messageIDs := args

	// load credentials from viper config
	accessToken := viper.GetString("access_token")
	client := viper.GetString("client")
	uid := viper.GetString("uid")
	backendURL := viper.GetString("backend_url")

	if accessToken == "" || client == "" || uid == "" || backendURL == "" {
		fmt.Println("Error: Missing credentials. Please login first.")
		return
	}

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
		fmt.Printf("Error making request: %v\n", err)
		return
	}

	if resp.StatusCode() != http.StatusOK {
		fmt.Printf("Error: Received status code %d\n", resp.StatusCode())
		fmt.Printf("Response body: %s\n", resp.Body())
		return
	}

	// parse and print the response
	var messagesInfo map[string]interface{}
	err = json.Unmarshal(resp.Body(), &messagesInfo)
	if err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		return
	}

	// pretty print the messages info
	prettyJSON, err := json.MarshalIndent(messagesInfo, "", "  ")
	if err != nil {
		fmt.Printf("Error formatting JSON: %v\n", err)
		return
	}

	fmt.Println(string(prettyJSON))
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
