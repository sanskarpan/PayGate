package eventschema

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestSchemaFixturesValidateAndHaveSamples(t *testing.T) {
	root := filepath.Join("..", "..", "schemas", "events")
	schemaFiles, err := filepath.Glob(filepath.Join(root, "*", "*.schema.json"))
	if err != nil {
		t.Fatalf("glob schema fixtures: %v", err)
	}
	sort.Strings(schemaFiles)
	if len(schemaFiles) == 0 {
		t.Fatal("expected schema fixtures")
	}

	gotSubjects := map[string]bool{}
	for _, schemaPath := range schemaFiles {
		subject := filepath.Base(filepath.Dir(schemaPath))
		gotSubjects[subject] = true
	}
	for _, subject := range expectedSchemaFixtureSubjects() {
		if !gotSubjects[subject] {
			t.Fatalf("missing schema fixtures for subject %s", subject)
		}
	}

	for _, schemaPath := range schemaFiles {
		t.Run(strings.TrimPrefix(schemaPath, root+"/"), func(t *testing.T) {
			schema, sample, version := loadFixturePair(t, schemaPath)
			if err := ValidateDocument(schema); err != nil {
				t.Fatalf("validate schema: %v", err)
			}
			if gotVersion, _ := sample["schema_version"].(string); gotVersion != version {
				t.Fatalf("expected sample schema_version %q, got %#v", version, sample["schema_version"])
			}
			validateSampleAgainstRequiredFields(t, schema, sample, "")
		})
	}
}

func TestCheckCompatibilityAllowsAdditiveOptionalFields(t *testing.T) {
	oldSchema, _, _ := loadFixturePair(t, filepath.Join("..", "..", "schemas", "events", "payment.captured", "1.0.0.schema.json"))
	newSchema, _, _ := loadFixturePair(t, filepath.Join("..", "..", "schemas", "events", "payment.captured", "1.1.0.schema.json"))

	checks := CheckCompatibility(oldSchema, newSchema)
	if len(checks) != 2 {
		t.Fatalf("expected 2 compatibility checks, got %d", len(checks))
	}
	for _, check := range checks {
		if !check.Compatible {
			t.Fatalf("expected additive optional field to be compatible for %s, got %+v", check.CheckType, check)
		}
	}
}

func loadFixturePair(t *testing.T, schemaPath string) (Document, map[string]any, string) {
	t.Helper()
	rawSchema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema fixture: %v", err)
	}
	var schema Document
	if err := json.Unmarshal(rawSchema, &schema); err != nil {
		t.Fatalf("decode schema fixture: %v", err)
	}

	samplePath := strings.TrimSuffix(schemaPath, ".schema.json") + ".sample.json"
	rawSample, err := os.ReadFile(samplePath)
	if err != nil {
		t.Fatalf("read sample fixture: %v", err)
	}
	var sample map[string]any
	if err := json.Unmarshal(rawSample, &sample); err != nil {
		t.Fatalf("decode sample fixture: %v", err)
	}
	version := strings.TrimSuffix(filepath.Base(schemaPath), ".schema.json")
	return schema, sample, version
}

func expectedSchemaFixtureSubjects() []string {
	return []string{
		"dispute.accepted",
		"dispute.created",
		"dispute.lost",
		"dispute.updated",
		"dispute.won",
		"order.created",
		"order.paid",
		"payment.authorized",
		"payment.authorization_reversed",
		"payment.auto_refunded",
		"payment.captured",
		"payment.failed",
		"payout.cancelled",
		"payout.completed",
		"payout.created",
		"payout.failed",
		"payout.initiated",
		"payout.returned",
		"payout.reversed",
		"refund.created",
		"refund.failed",
		"refund.processed",
		"refund.reversed",
		"risk.alert",
		"settlement.rollback_marked",
		"settlement.processed",
	}
}

func validateSampleAgainstRequiredFields(t *testing.T, schema Document, sample map[string]any, prefix string) {
	t.Helper()
	for _, field := range schema.Required {
		value, ok := sample[field]
		if !ok {
			t.Fatalf("sample missing required field %s", joinPath(prefix, field))
		}
		fieldSchema, ok := schema.Properties[field]
		if !ok {
			t.Fatalf("schema missing definition for required field %s", joinPath(prefix, field))
		}
		if fieldSchema.Type == "object" {
			nested, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("sample field %s must be object", joinPath(prefix, field))
			}
			validateSampleAgainstRequiredFields(t, Document{
				Type:       fieldSchema.Type,
				Properties: fieldSchema.Properties,
				Required:   fieldSchema.Required,
			}, nested, joinPath(prefix, field))
		}
	}
}
