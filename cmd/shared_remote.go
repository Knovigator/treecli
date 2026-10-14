package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Knovigator/treecli/api"
)

func sharedCall(profile profileConfig, tool string, args map[string]interface{}, out interface{}) error {
	raw, err := api.SharedOperation(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, tool, args)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
func uploadSharedFile(profile profileConfig, path, purpose string) (map[string]interface{}, error) {
	return api.UploadSharedFile(profile.BackendURL, profile.AccessToken, profile.Client, profile.UID, path, purpose)
}
func sharedNativeWrite(profile profileConfig, operation string, args map[string]interface{}, attachment string, cap int, out interface{}) error {
	id, err := resolveWriteID(stringValue(args["id"]))
	if err != nil {
		return err
	}
	args["id"] = id
	// File selection/byte reading is local; ownership and input classification belong to the backend.
	if attachment != "" {
		upload, err := uploadSharedFile(profile, attachment, "attachment")
		if err != nil {
			return withWriteID(id, err)
		}
		args["files"] = []interface{}{upload["signed_id"]}
	}
	var quote struct {
		QuoteID string `json:"quote_id"`
	}
	if err := sharedCall(profile, "quote_treechat_write", map[string]interface{}{"operation": operation, "arguments": args, "max_price_cents": cap}, &quote); err != nil {
		return withWriteID(id, err)
	}
	if quote.QuoteID == "" {
		return withWriteID(id, fmt.Errorf("backend omitted write quote ID"))
	}
	fmt.Fprintf(os.Stderr, "Write %s: authorized maximum generation fee %d USD cents (BSV network fees separate).\n", id, cap)
	var result struct {
		OperationID string `json:"operation_id"`
		State       string `json:"state"`
		Result      struct {
			Status int             `json:"status"`
			Body   json.RawMessage `json:"body"`
		} `json:"result"`
	}
	if err := sharedCall(profile, "create_treechat_write", map[string]interface{}{"quote_id": quote.QuoteID, "idempotency_key": id}, &result); err != nil {
		return withWriteID(id, err)
	}
	if result.OperationID != quote.QuoteID {
		return withWriteID(id, fmt.Errorf("backend returned a different write operation; inspect %s before retrying", quote.QuoteID))
	}
	if result.State == "rejected" || result.Result.Status >= 400 {
		return withWriteID(id, fmt.Errorf("write rejected (status %d): %s", result.Result.Status, api.SafeCredentialResponseBody(result.Result.Body, profile.AccessToken, profile.Client, profile.UID)))
	}
	if result.State != "completed" {
		return withWriteID(id, fmt.Errorf("write operation %s is %s; inspect with get_treechat_write before retrying", result.OperationID, result.State))
	}
	if err := json.Unmarshal(result.Result.Body, out); err != nil {
		return withWriteID(id, err)
	}
	switch value := out.(type) {
	case *api.CreateQuestResponse:
		value.Raw = result.Result.Body
	case *api.CreateAnswerResponse:
		value.Raw = result.Result.Body
	}
	return nil
}
func stringValue(value interface{}) string { v, _ := value.(string); return v }
func optional(args map[string]interface{}, key, value string) {
	if value != "" {
		args[key] = value
	}
}
func optionalBool(args map[string]interface{}, key string, value *bool) {
	if value != nil {
		args[key] = *value
	}
}

func uploadLocalInputs(profile profileConfig, value interface{}) (interface{}, error) {
	switch item := value.(type) {
	case string:
		if len(item) > 0 && item[0] == '@' {
			result, err := uploadSharedFile(profile, item[1:], "reference")
			if err != nil {
				return nil, err
			}
			id := stringValue(result["id"])
			if id == "" {
				id = stringValue(result["run_id"])
			}
			if id == "" {
				return nil, fmt.Errorf("reference upload returned no run ID")
			}
			return "run:" + id, nil
		}
	case []interface{}:
		for i, nested := range item {
			resolved, err := uploadLocalInputs(profile, nested)
			if err != nil {
				return nil, err
			}
			item[i] = resolved
		}
	case map[string]interface{}:
		for key, nested := range item {
			resolved, err := uploadLocalInputs(profile, nested)
			if err != nil {
				return nil, err
			}
			item[key] = resolved
		}
	}
	return value, nil
}
