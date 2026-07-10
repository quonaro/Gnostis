package directory

import (
	"testing"

	"github.com/quonaro/gnostis/internal/config"
)

func TestShouldIndex(t *testing.T) {
	idx := config.Index{
		DefaultExtensions:      []string{".go", ".md"},
		DefaultExcludePatterns: []string{"vendor/**"},
	}

	d := FromConfig(idx, config.Directory{
		Path:          "/tmp/proj",
		Extensions:    []string{".go"},
		Exclude:       []string{"**/*_test.go"},
		MaxFileSizeMB: 1,
	})

	cases := []struct {
		rel  string
		size int64
		want bool
	}{
		{"main.go", 100, true},
		{"README.md", 100, false},
		{"main_test.go", 100, false},
		{"vendor/lib.go", 100, false},
		{"big.go", 2 * 1024 * 1024, false},
	}

	for _, tc := range cases {
		got := d.ShouldIndex(tc.rel, tc.size)
		if got != tc.want {
			t.Errorf("ShouldIndex(%q, %d) = %v, want %v", tc.rel, tc.size, got, tc.want)
		}
	}
}

func TestIncludeFilter(t *testing.T) {
	idx := config.Index{DefaultExtensions: []string{".go"}}
	d := FromConfig(idx, config.Directory{
		Path:    "/tmp/proj",
		Include: []string{"src/**"},
	})

	if d.ShouldIndex("main.go", 100) {
		t.Error("main.go should not match include filter")
	}
	if !d.ShouldIndex("src/app.go", 100) {
		t.Error("src/app.go should match include filter")
	}
}
