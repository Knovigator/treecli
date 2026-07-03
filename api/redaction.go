package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
)

const redactedValue = "[REDACTED]"

var sensitiveJSONFields = map[string]struct{}{
	"access_token":          {},
	"api_key":               {},
	"auth_token":            {},
	"authentication_token":  {},
	"client":                {},
	"confirmation_token":    {},
	"email":                 {},
	"email_address":         {},
	"email_addresses":       {},
	"emails":                {},
	"encrypted_password":    {},
	"id_token":              {},
	"password":              {},
	"password_confirmation": {},
	"password_digest":       {},
	"password_hash":         {},
	"private_key":           {},
	"refresh_token":         {},
	"reset_password_token":  {},
	"secret":                {},
	"token":                 {},
	"tokens":                {},
	"uid":                   {},
	"unlock_token":          {},
}

var emailTextPattern = regexp.MustCompile(`[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)+`)

func PrettyJSON(raw []byte) (string, error) {
	payload, err := decodeSingleJSONValue(raw)
	if err != nil {
		return "", err
	}

	redactSensitiveJSONValue(payload)

	prettyJSON, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}

	return string(prettyJSON), nil
}

func SafeResponseBody(raw []byte) string {
	trimmedBody := strings.TrimSpace(string(raw))
	if trimmedBody == "" {
		return ""
	}

	if payload, err := decodeSingleJSONValue(raw); err == nil {
		redactSensitiveJSONValue(payload)
		redactSensitiveJSONStrings(payload)
		if encodedBody, marshalErr := json.Marshal(payload); marshalErr == nil {
			return string(encodedBody)
		}
	}

	return redactSensitiveText(trimmedBody)
}

func decodeSingleJSONValue(raw []byte) (interface{}, error) {
	var payload interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}

	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("invalid JSON: multiple JSON values")
		}
		return nil, err
	}

	return payload, nil
}

func redactSensitiveJSONValue(value interface{}) {
	switch typedValue := value.(type) {
	case map[string]interface{}:
		for key, nestedValue := range typedValue {
			if isSensitiveJSONField(key) {
				typedValue[key] = redactedValue
				continue
			}
			redactSensitiveJSONValue(nestedValue)
		}
	case []interface{}:
		for _, nestedValue := range typedValue {
			redactSensitiveJSONValue(nestedValue)
		}
	}
}

func redactSensitiveJSONStrings(value interface{}) {
	switch typedValue := value.(type) {
	case map[string]interface{}:
		for key, nestedValue := range typedValue {
			if stringValue, ok := nestedValue.(string); ok {
				typedValue[key] = redactSensitiveText(stringValue)
				continue
			}
			redactSensitiveJSONStrings(nestedValue)
		}
	case []interface{}:
		for index, nestedValue := range typedValue {
			if stringValue, ok := nestedValue.(string); ok {
				typedValue[index] = redactSensitiveText(stringValue)
				continue
			}
			redactSensitiveJSONStrings(nestedValue)
		}
	}
}

func isSensitiveJSONField(key string) bool {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	normalizedKey = strings.ReplaceAll(normalizedKey, "-", "_")
	normalizedKey = strings.ReplaceAll(normalizedKey, " ", "_")
	if _, ok := sensitiveJSONFields[normalizedKey]; ok {
		return true
	}

	compactKey := strings.ReplaceAll(normalizedKey, "_", "")
	return strings.Contains(compactKey, "email")
}

func redactSensitiveText(value string) string {
	return emailTextPattern.ReplaceAllString(value, redactedValue)
}
