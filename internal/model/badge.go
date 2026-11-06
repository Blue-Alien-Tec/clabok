package model

// Badge is one generated repository badge. ID selects the detector; a non-empty Message overrides
// the detector, so a hand-written value stays possible. Link wraps the badge in a link when set.
type Badge struct {
	ID      string `json:"id"`
	Label   string `json:"label,omitempty"`
	Message string `json:"message,omitempty"`
	Link    string `json:"link,omitempty"`
}
