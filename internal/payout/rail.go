package payout

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

const (
	railSignatureHeader = "X-PayGate-Rail-Signature"
	railTimestampHeader = "X-PayGate-Rail-Timestamp"
)

func SignRailPayload(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp))
	_, _ = mac.Write([]byte("."))
	_, _ = mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func VerifyRailPayload(secret, timestamp string, body []byte, signature string) bool {
	if secret == "" || timestamp == "" || signature == "" {
		return false
	}
	expected := SignRailPayload(secret, timestamp, body)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

func eventTypeForRailStatus(status RailCallbackStatus) string {
	return fmt.Sprintf("payout.rail_%s", status)
}
