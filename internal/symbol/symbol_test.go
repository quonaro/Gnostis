package symbol

import (
	"path/filepath"
	"testing"
)

func TestIndex_Lookup(t *testing.T) {
	idx := newTestIndex(t)
	idx.Add(Location{Symbol: "Foo", Path: "/foo.go", StartLine: 1})

	locs := idx.Lookup("Foo")
	if len(locs) != 1 || locs[0].Symbol != "Foo" {
		t.Errorf("Lookup(Foo) = %+v, want 1 Foo", locs)
	}
	if len(idx.Lookup("Bar")) != 0 {
		t.Errorf("Lookup(Bar) should be empty")
	}
}

func TestIndex_LookupCaseInsensitive(t *testing.T) {
	idx := newTestIndex(t)
	idx.Add(Location{Symbol: "FooBar", Path: "/foo.go", StartLine: 1})

	locs := idx.Lookup("foobar")
	if len(locs) != 1 {
		t.Errorf("Lookup(foobar) = %d, want 1", len(locs))
	}
}

func TestIndex_RemoveByPath(t *testing.T) {
	idx := newTestIndex(t)
	idx.Add(Location{Symbol: "Foo", Path: "/foo.go", StartLine: 1})
	idx.Add(Location{Symbol: "Bar", Path: "/bar.go", StartLine: 1})
	idx.RemoveByPath("/foo.go")

	if len(idx.Lookup("Foo")) != 0 {
		t.Errorf("Lookup(Foo) should be empty after removing /foo.go")
	}
	if len(idx.Lookup("Bar")) != 1 {
		t.Errorf("Lookup(Bar) = %d, want 1", len(idx.Lookup("Bar")))
	}
}

func TestIndex_SearchFuzzy(t *testing.T) {
	idx := newTestIndex(t)
	idx.Add(Location{Symbol: "FooBar", Path: "/foo.go", StartLine: 1})
	idx.Add(Location{Symbol: "BarBaz", Path: "/bar.go", StartLine: 1})
	idx.Add(Location{Symbol: "Qux", Path: "/qux.go", StartLine: 1})

	locs := idx.SearchFuzzy("Bar")
	if len(locs) != 2 {
		t.Errorf("SearchFuzzy(Bar) = %d, want 2", len(locs))
	}
}

func TestIndex_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "symbols.json")
	idx, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	idx.Add(Location{Symbol: "Foo", Path: "/foo.go", StartLine: 1})
	if err := idx.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	idx2, err := New(path)
	if err != nil {
		t.Fatalf("New reopen: %v", err)
	}
	locs := idx2.Lookup("Foo")
	if len(locs) != 1 || locs[0].Path != "/foo.go" {
		t.Errorf("Lookup after reopen = %+v, want /foo.go", locs)
	}
}

func TestIndex_AddChunks(t *testing.T) {
	idx := newTestIndex(t)
	idx.AddChunks([]Chunk{
		{Symbol: "Foo", Path: "/foo.go", StartLine: 1, EndLine: 5},
		{Symbol: "", Path: "/foo.go", StartLine: 6, EndLine: 7},
	})
	if len(idx.Lookup("Foo")) != 1 {
		t.Errorf("Lookup(Foo) = %d, want 1", len(idx.Lookup("Foo")))
	}
}

func TestIndex_Count(t *testing.T) {
	idx := newTestIndex(t)
	idx.Add(Location{Symbol: "Foo", Path: "/foo.go", StartLine: 1})
	idx.Add(Location{Symbol: "Bar", Path: "/bar.go", StartLine: 1})
	idx.Add(Location{Symbol: "Bar", Path: "/baz.go", StartLine: 1})
	if got := idx.Count(); got != 3 {
		t.Errorf("Count = %d, want 3", got)
	}
}

func newTestIndex(t *testing.T) *Index {
	t.Helper()
	idx, err := New(filepath.Join(t.TempDir(), "symbols.json"))
	if err != nil {
		t.Fatalf("new index: %v", err)
	}
	return idx
}
