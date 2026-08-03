package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Knovigator/treecli/api"
)

type writeOutcomeError struct {
	WriteID string
	Err     error
}

type structuredWriteError struct {
	WriteID string
	Message string
	Err     error
}

type writeErrorPayload struct {
	Status  string `json:"status"`
	WriteID string `json:"write_id"`
	Error   string `json:"error"`
}

func (e *writeOutcomeError) Error() string {
	return fmt.Sprintf(
		"%v (write id: %s; reuse this value with --id for any retry)",
		e.Err,
		e.WriteID,
	)
}

func (e *writeOutcomeError) Unwrap() error {
	return e.Err
}

func (e *structuredWriteError) Error() string {
	return e.Err.Error()
}

func (e *structuredWriteError) Unwrap() error {
	return e.Err
}

func resolveWriteID(requestedID string) (string, error) {
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		generatedID, err := newUUID()
		if err != nil {
			return "", fmt.Errorf("generating write id: %w", err)
		}
		return generatedID, nil
	}

	if !looksLikeUUID(requestedID) {
		return "", fmt.Errorf("--id must be a UUID")
	}

	return strings.ToLower(requestedID), nil
}

func withWriteID(writeID string, err error) error {
	if err == nil || strings.TrimSpace(writeID) == "" {
		return err
	}

	var existingWriteError *writeOutcomeError
	if errors.As(err, &existingWriteError) {
		return err
	}

	return &writeOutcomeError{WriteID: writeID, Err: err}
}

func writeErrorForOutput(err error, outputFormat string) error {
	if err == nil || outputFormat != "json" {
		return err
	}

	var writeErr *writeOutcomeError
	if !errors.As(err, &writeErr) {
		return err
	}

	message := strings.Replace(err.Error(), writeErr.Error(), writeErr.Err.Error(), 1)
	return &structuredWriteError{
		WriteID: writeErr.WriteID,
		Message: message,
		Err:     err,
	}
}

// PrintError writes machine-readable write failures when a command requested JSON.
// Other errors retain the CLI's historical human-readable format.
func PrintError(writer io.Writer, err error) {
	var structuredErr *structuredWriteError
	if errors.As(err, &structuredErr) {
		encoder := json.NewEncoder(writer)
		encoder.SetIndent("", "  ")
		if encoder.Encode(writeErrorPayload{
			Status:  "error",
			WriteID: structuredErr.WriteID,
			Error:   structuredErr.Message,
		}) == nil {
			return
		}
	}

	fmt.Fprintln(writer, "Error:", err)
}

func prettyWriteSuccessJSON(raw json.RawMessage, writeID string) (string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	encodedWriteID, err := json.Marshal(writeID)
	if err != nil {
		return "", err
	}
	payload["write_id"] = encodedWriteID

	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return api.PrettyJSON(encoded)
}
