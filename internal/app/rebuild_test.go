package app

import "testing"

func TestJobPrefix(t *testing.T) {
	cases := []struct {
		id       string
		expected string
	}{
		{"project:RuobrOld-1783920062303548722", "project:RuobrOld"},
		{"index-1783920062303548722", "index"},
		{"project:some-name-with-hyphens-123", "project:some-name-with-hyphens"},
		{"plain", "plain"},
	}

	for _, c := range cases {
		got := jobPrefix(c.id)
		if got != c.expected {
			t.Errorf("jobPrefix(%q) = %q, want %q", c.id, got, c.expected)
		}
	}
}
