package api

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SharedOperation is the sole RPC envelope used by legacy CLI adapters and MCP.
// The backend owns operation validation, authorization and business decisions.
func SharedOperation(backendURL, accessToken, clientID, uid, tool string, arguments map[string]interface{}) (json.RawMessage, error) {
	client, err := NewTokenClient(backendURL, accessToken, clientID, uid)
	if err != nil {
		return nil, err
	}
	result, err := client.CallTool(context.Background(), tool, arguments)
	if err != nil {
		return nil, err
	}
	if result.StructuredContent == nil {
		return nil, fmt.Errorf("backend omitted structured tool result")
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}
	if json.Unmarshal(raw, &envelope) == nil && envelope.Status != 0 {
		if envelope.Status >= 400 {
			return nil, &HTTPError{StatusCode: envelope.Status, Body: SafeCredentialResponseBody(envelope.Body, accessToken, clientID, uid)}
		}
		return envelope.Body, nil
	}
	if result.IsError {
		return nil, fmt.Errorf("Treechat operation failed: %s", SafeCredentialResponseBody(raw, accessToken, clientID, uid))
	}
	return raw, nil
}

func sharedRead(backendURL, token, client, uid, operation string, args map[string]interface{}, out interface{}) (json.RawMessage, error) {
	raw, err := SharedOperation(backendURL, token, client, uid, "treechat_read", map[string]interface{}{"operation": operation, "arguments": args})
	if err == nil {
		err = json.Unmarshal(raw, out)
	}
	return raw, err
}

// DecodeGeneration adapts response naming only; billing and media decisions stay server-side.
func DecodeGeneration(raw json.RawMessage) (GenerationResponse, error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(raw, &fields); err != nil {
		return GenerationResponse{}, err
	}
	if fields["generation_id"] != nil {
		fields["id"] = fields["generation_id"]
	}
	if failure, ok := fields["error"].(map[string]interface{}); ok {
		fields["failure"] = failure
		delete(fields, "error")
	}
	adapted, _ := json.Marshal(fields)
	var out GenerationResponse
	err := json.Unmarshal(adapted, &out)
	out.Raw = raw
	out.normalizeMediaOutputs()
	out.normalizeAction()
	return out, err
}

// UploadSharedFile keeps file selection and storage transport local, while the
// shared server operations own registration, validation and reference ownership.
func UploadSharedFile(backendURL, token, client, uid, path, purpose string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw, err := SharedOperation(backendURL, token, client, uid, "register_upload", map[string]interface{}{
		"purpose": purpose, "filename": filepath.Base(path), "content_type": DetectFileContentType(path, data),
		"byte_size": len(data), "checksum": md5Base64(data),
	})
	if err != nil {
		return nil, err
	}
	var target directUploadTarget
	if err := json.Unmarshal(raw, &target); err != nil {
		return nil, err
	}
	signedID, err := putDirectUpload(target, data)
	if err != nil {
		return nil, err
	}
	if purpose == "attachment" {
		return map[string]interface{}{"signed_id": signedID}, nil
	}
	raw, err = SharedOperation(backendURL, token, client, uid, "finish_reference_upload", map[string]interface{}{"signed_id": signedID})
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	err = json.Unmarshal(raw, &result)
	return result, err
}
