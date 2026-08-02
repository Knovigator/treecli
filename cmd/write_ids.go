package cmd

import (
	"errors"
	"fmt"
	"strings"
)

type writeOutcomeError struct {
	WriteID string
	Err     error
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
