package idgen

import (
	"strings"
	"testing"
)

func TestNewWithPrefix(t *testing.T) {
	id := New("order")
	if !strings.HasPrefix(id, "order_") {
		t.Fatalf("expected prefix order_, got %s", id)
	}
}

func TestNewWithoutPrefix(t *testing.T) {
	id := New("")
	if strings.Contains(id, "_") {
		t.Fatalf("expected raw KSUID without underscore, got %s", id)
	}
}
