package openai

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Keep the first chunk's ID, including when an upstream sends a separate prelude.
func normalizeChatStreamID(data string, responseID *string, fallbackID string) (string, error) {
	id := gjson.Get(data, "id").String()
	if *responseID == "" {
		*responseID = id
		if *responseID == "" {
			*responseID = fallbackID
		}
	}
	if id == *responseID {
		return data, nil
	}
	// Patch only the ID so provider extensions and billing details survive.
	return sjson.Set(data, "id", *responseID)
}
