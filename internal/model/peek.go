package model

import "encoding/json"

// PeekVersion reads the raw manifest version without decoding or upgrading the document.
func PeekVersion(data []byte) (int, error) {
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return 0, &Error{Code: "model.invalid-json", Message: err.Error()}
	}
	return header.Version, nil
}
