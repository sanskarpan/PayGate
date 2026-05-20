package eventschema

import (
	"fmt"
	"sort"
)

var requiredEnvelopeFields = []string{
	"event_id",
	"event_type",
	"occurred_at",
	"correlation_id",
	"causation_id",
	"schema_version",
	"merchant_id",
	"payload",
}

type CompatibilityResult struct {
	Compatible bool
	Summary    string
	Details    map[string]any
}

func ValidateDocument(doc Document) error {
	if doc.Type != "object" {
		return fmt.Errorf("%w: top-level schema must be an object", ErrInvalidSchemaDocument)
	}
	required := setOf(doc.Required)
	for _, field := range requiredEnvelopeFields {
		def, ok := doc.Properties[field]
		if !ok {
			return fmt.Errorf("%w: missing required envelope field %q", ErrInvalidSchemaDocument, field)
		}
		if !required[field] {
			return fmt.Errorf("%w: envelope field %q must be required", ErrInvalidSchemaDocument, field)
		}
		if field == "payload" && def.Type != "object" {
			return fmt.Errorf("%w: payload must be an object", ErrInvalidSchemaDocument)
		}
	}
	return nil
}

func CheckCompatibility(previous, candidate Document) []CompatibilityCheck {
	results := []CompatibilityCheck{
		{
			CheckType:  CheckTypeBackward,
			Compatible: true,
			Summary:    "backward compatible",
			Details:    map[string]any{},
		},
		{
			CheckType:  CheckTypeForward,
			Compatible: true,
			Summary:    "forward compatible",
			Details:    map[string]any{},
		},
	}

	backwardProblems := compareFields(previous, candidate, "")
	forwardProblems := compareForwardCompatibleFields(candidate, previous, "")

	results[0].Compatible = len(backwardProblems) == 0
	results[1].Compatible = len(forwardProblems) == 0
	if !results[0].Compatible {
		results[0].Summary = "backward compatibility check failed"
		results[0].Details["problems"] = backwardProblems
	}
	if !results[1].Compatible {
		results[1].Summary = "forward compatibility check failed"
		results[1].Details["problems"] = forwardProblems
	}
	return results
}

func compareFields(previous, candidate Document, prefix string) []string {
	prevRequired := setOf(previous.Required)
	candidateRequired := setOf(candidate.Required)
	var problems []string

	for field, prevDef := range previous.Properties {
		path := joinPath(prefix, field)
		candidateDef, ok := candidate.Properties[field]
		if !ok {
			problems = append(problems, fmt.Sprintf("missing field %s", path))
			continue
		}
		if prevDef.Type != candidateDef.Type {
			problems = append(problems, fmt.Sprintf("field %s changed type from %s to %s", path, prevDef.Type, candidateDef.Type))
			continue
		}
		if prevRequired[field] && !candidateRequired[field] {
			problems = append(problems, fmt.Sprintf("field %s is no longer required", path))
		}
		if !prevRequired[field] && candidateRequired[field] {
			problems = append(problems, fmt.Sprintf("field %s became newly required", path))
		}
		if prevDef.Type == "object" {
			nestedPrev := Document(prevDef)
			nestedCandidate := Document(candidateDef)
			problems = append(problems, compareFields(nestedPrev, nestedCandidate, path)...)
		}
	}

	// Additive changes are allowed as long as the field is not required immediately.
	for field := range candidate.Properties {
		if _, ok := previous.Properties[field]; ok {
			continue
		}
		if candidateRequired[field] {
			problems = append(problems, fmt.Sprintf("new field %s is required", joinPath(prefix, field)))
		}
	}

	sort.Strings(problems)
	return problems
}

func compareForwardCompatibleFields(future, current Document, prefix string) []string {
	futureRequired := setOf(future.Required)
	currentRequired := setOf(current.Required)
	var problems []string

	for field, futureDef := range future.Properties {
		path := joinPath(prefix, field)
		currentDef, ok := current.Properties[field]
		if !ok {
			if futureRequired[field] {
				problems = append(problems, fmt.Sprintf("missing required field %s", path))
			}
			continue
		}
		if futureDef.Type != currentDef.Type {
			problems = append(problems, fmt.Sprintf("field %s changed type from %s to %s", path, currentDef.Type, futureDef.Type))
			continue
		}
		if currentRequired[field] && !futureRequired[field] {
			problems = append(problems, fmt.Sprintf("field %s is no longer required", path))
		}
		if futureDef.Type == "object" {
			nestedFuture := Document(futureDef)
			nestedCurrent := Document(currentDef)
			problems = append(problems, compareForwardCompatibleFields(nestedFuture, nestedCurrent, path)...)
		}
	}

	sort.Strings(problems)
	return problems
}

func setOf(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func joinPath(prefix, field string) string {
	if prefix == "" {
		return field
	}
	return prefix + "." + field
}
