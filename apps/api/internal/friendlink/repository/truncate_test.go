package repository

import (
	"strings"
	"testing"
)

func TestTruncateReasonKeepsUTF8(t *testing.T) {
	got := truncateReason(strings.Repeat("超", maxFailReasonLen+50))
	if len([]rune(got)) != maxFailReasonLen || strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("truncation broke a rune boundary: %d runes", len([]rune(got)))
	}
	if truncateReason("HTTP 502") != "HTTP 502" {
		t.Fatal("a short reason must stay untouched")
	}
}
