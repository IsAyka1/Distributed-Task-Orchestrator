package workflow

import "encoding/json"

// Payload is opaque activity data; the orchestrator does not interpret its fields.
type Payload struct {
	Value json.RawMessage
}
