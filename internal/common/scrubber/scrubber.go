// Package scrubber removes sensitive data from strings before they reach logs.
// It handles both JSON key-value patterns and raw card number sequences.
package scrubber

import (
	"regexp"
	"strings"
)

// sensitiveKeys is the set of JSON key names whose values must be redacted.
var sensitiveKeys = []string{
	"password", "secret", "token", "cvv", "cvc",
	"card_number", "cardnumber", "card_no", "pan",
	"account_number", "account_no",
	"key_secret", "webhook_secret",
	"authorization", "api_key",
}

// sensitiveKeyRe matches "key": "value" or "key":"value" patterns in JSON.
var sensitiveKeyRe = buildKeyRe(sensitiveKeys)

// cardNumberRe matches 13-19 digit sequences that look like card numbers.
// It avoids matching short numeric IDs by requiring the sequence to be
// preceded and followed by non-digit characters (or string boundaries).
var cardNumberRe = regexp.MustCompile(`\b\d{13,19}\b`)

func buildKeyRe(keys []string) *regexp.Regexp {
	// Build: "(?i)("key1"|"key2"|...)\s*:\s*"[^"]*"
	escaped := make([]string, len(keys))
	for i, k := range keys {
		escaped[i] = regexp.QuoteMeta(`"` + k + `"`)
	}
	pattern := `(?i)(` + strings.Join(escaped, "|") + `)\s*:\s*"[^"]*"`
	return regexp.MustCompile(pattern)
}

// Scrub returns a copy of s with sensitive values replaced by [SCRUBBED].
// It is safe to call on arbitrary strings (JSON, form-encoded, plain text).
func Scrub(s string) string {
	if s == "" {
		return s
	}
	// Replace known-sensitive JSON key values.
	s = sensitiveKeyRe.ReplaceAllStringFunc(s, func(match string) string {
		// Keep the key name; replace only the value.
		colon := strings.Index(match, ":")
		if colon < 0 {
			return `[SCRUBBED]`
		}
		return match[:colon+1] + `"[SCRUBBED]"`
	})
	// Replace card number sequences.
	s = cardNumberRe.ReplaceAllString(s, "[CARD_SCRUBBED]")
	return s
}
