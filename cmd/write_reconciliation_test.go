package cmd

import (
	"encoding/json"
	"testing"
)

func TestWriteSuccessJSONRejectsNullResponse(t *testing.T) {
	if _, err := prettyWriteSuccessJSON(json.RawMessage(`null`), testWriteID); err == nil {
		t.Fatal("expected an error for a null write response")
	}
}
