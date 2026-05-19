package eventschema

import (
	"encoding/json"
	"testing"
)

func FuzzValidateDocument(f *testing.F) {
	f.Add(`{"type":"object","required":["event_id","event_type","occurred_at","correlation_id","causation_id","schema_version","merchant_id","payload"],"properties":{"event_id":{"type":"string"},"event_type":{"type":"string"},"occurred_at":{"type":"string"},"correlation_id":{"type":"string"},"causation_id":{"type":"string"},"schema_version":{"type":"string"},"merchant_id":{"type":"string"},"payload":{"type":"object","properties":{},"required":[]}}}`)
	f.Add(`{}`)
	f.Add(`[]`)
	f.Add(`"bad"`)

	f.Fuzz(func(t *testing.T, raw string) {
		var doc Document
		err := json.Unmarshal([]byte(raw), &doc)
		if err != nil {
			return
		}
		_ = ValidateDocument(doc)
	})
}
