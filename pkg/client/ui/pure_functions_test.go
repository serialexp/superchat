package ui

import (
	"strings"
	"testing"
)

// Test pure functions (no dependencies on Model state)

func TestIsDeletedMessageContent(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "deleted message",
			content:  "[deleted by author]",
			expected: true,
		},
		{
			name:     "deleted by moderator",
			content:  "[deleted by moderator]",
			expected: true,
		},
		{
			name:     "normal message",
			content:  "This is a normal message",
			expected: false,
		},
		{
			name:     "message mentioning deletion",
			content:  "I will delete this later",
			expected: false,
		},
		{
			name:     "empty message",
			content:  "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDeletedMessageContent(tt.content)
			if result != tt.expected {
				t.Errorf("isDeletedMessageContent(%q) = %v, want %v", tt.content, result, tt.expected)
			}
		})
	}
}

func TestMaxMin(t *testing.T) {
	tests := []struct {
		name string
		a    int
		b    int
		max  int
		min  int
	}{
		{"both positive", 5, 3, 5, 3},
		{"both negative", -5, -3, -3, -5},
		{"mixed", -5, 3, 3, -5},
		{"equal", 5, 5, 5, 5},
		{"zero", 0, 5, 5, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			maxResult := max(tt.a, tt.b)
			if maxResult != tt.max {
				t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, maxResult, tt.max)
			}

			minResult := min(tt.a, tt.b)
			if minResult != tt.min {
				t.Errorf("min(%d, %d) = %d, want %d", tt.a, tt.b, minResult, tt.min)
			}
		})
	}
}

func TestMergeOverlay(t *testing.T) {
	base := "line1\nline2\nline3\nline4"
	overlay := "\n\nOVERLAY\n"

	result := mergeOverlay(base, overlay)

	lines := strings.Split(result, "\n")
	if len(lines) != 4 {
		t.Errorf("mergeOverlay() returned %d lines, want 4", len(lines))
	}

	if lines[0] != "line1" {
		t.Errorf("line 0 = %q, want %q", lines[0], "line1")
	}

	if lines[2] != "OVERLAY" {
		t.Errorf("line 2 = %q, want %q", lines[2], "OVERLAY")
	}
}
