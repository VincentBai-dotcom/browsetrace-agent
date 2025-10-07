package models

type Event struct {
	TSUTC int64          `json:"ts_utc"`
	TSISO string         `json:"ts_iso"`
	URL   string         `json:"url"`
	Title *string        `json:"title"` // nullable
	Type  string         `json:"type"`  // navigate|visible_text|click|input|scroll|focus
	Data  map[string]any `json:"data"`  // arbitrary JSON
}

type Batch struct {
	Events []Event `json:"events"`
}