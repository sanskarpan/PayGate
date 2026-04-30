package idgen

import (
	"fmt"

	"github.com/segmentio/ksuid"
)

// New returns a time-sortable prefixed KSUID (e.g. order_xxx).
func New(prefix string) string {
	if prefix == "" {
		return ksuid.New().String()
	}
	return fmt.Sprintf("%s_%s", prefix, ksuid.New().String())
}
