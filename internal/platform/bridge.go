package platform

import (
	"encoding/json"
	"fmt"
)

// pageRequest is one call from the page: an id the page uses to match the
// answer, the method and its parameters.
type pageRequest struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// answerPage runs one call from the page and returns the script that hands
// the answer back to window.__agsReply. Go's JSON encoder escapes <, >, &,
// U+2028 and U+2029, so its output is a safe JavaScript literal. A message
// that is not a request has no id to answer, so it is an error instead.
func answerPage(message []byte, handle PageHandler) (string, error) {
	var request pageRequest
	if err := json.Unmarshal(message, &request); err != nil {
		return "", fmt.Errorf("decode page request: %w", err)
	}
	result, err := handle(request.Method, request.Params)
	encoded := []byte("null")
	if err == nil {
		if encoded, err = json.Marshal(result); err != nil {
			encoded, err = []byte("null"), fmt.Errorf("encode result: %w", err)
		}
	}
	failure := []byte("null")
	if err != nil {
		failure, _ = json.Marshal(err.Error()) // a string always encodes
	}
	return fmt.Sprintf("window.__agsReply(%d, %s, %s)", request.ID, encoded, failure), nil
}
