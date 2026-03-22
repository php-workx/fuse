package tui

import "testing"

// --- shortenToLastN tests ---

func TestShortenToLastN_DeepPath(t *testing.T) {
	got := shortenToLastN("/Users/runger/workspaces/fuse", 2)
	if got != ".../workspaces/fuse" {
		t.Errorf("deep path: got %q, want %q", got, ".../workspaces/fuse")
	}
}

func TestShortenToLastN_ShallowPath(t *testing.T) {
	// Exactly 2 components (after splitting): "workspaces" and "fuse".
	got := shortenToLastN("workspaces/fuse", 2)
	// len(parts) == 2, which is <= n, so returns original.
	if got != "workspaces/fuse" {
		t.Errorf("shallow path: got %q, want %q", got, "workspaces/fuse")
	}
}

func TestShortenToLastN_RootPath(t *testing.T) {
	// "/" splits into ["", ""], trailing empty removed -> [""].
	// len(parts) == 1 <= 2, returns original.
	got := shortenToLastN("/", 2)
	if got != "/" {
		t.Errorf("root path: got %q, want %q", got, "/")
	}
}

func TestShortenToLastN_Empty(t *testing.T) {
	got := shortenToLastN("", 2)
	if got != "(unknown)" {
		t.Errorf("empty path: got %q, want %q", got, "(unknown)")
	}
}

func TestShortenToLastN_SingleComponent(t *testing.T) {
	got := shortenToLastN("fuse", 2)
	if got != "fuse" {
		t.Errorf("single component: got %q, want %q", got, "fuse")
	}
}

func TestShortenToLastN_TrailingSlash(t *testing.T) {
	got := shortenToLastN("/Users/runger/workspaces/fuse/", 2)
	if got != ".../workspaces/fuse" {
		t.Errorf("trailing slash: got %q, want %q", got, ".../workspaces/fuse")
	}
}

// --- visibleLen tests ---

func TestVisibleLen_ASCII(t *testing.T) {
	got := visibleLen("hello world")
	if got != 11 {
		t.Errorf("ASCII: got %d, want 11", got)
	}
}

func TestVisibleLen_WithANSI(t *testing.T) {
	// ANSI codes should be stripped before counting.
	got := visibleLen("\x1b[31mhello\x1b[0m")
	if got != 5 {
		t.Errorf("ANSI: got %d, want 5", got)
	}
}

func TestVisibleLen_WithBlockChar(t *testing.T) {
	// Block characters like \u2588 are multi-byte but 1 rune each.
	got := visibleLen(string([]rune{'\u2588', '\u2588', '\u2588'}))
	if got != 3 {
		t.Errorf("block chars: got %d, want 3", got)
	}
}

func TestVisibleLen_Empty(t *testing.T) {
	got := visibleLen("")
	if got != 0 {
		t.Errorf("empty: got %d, want 0", got)
	}
}

// --- sortedCounts tests ---

func TestSortedCounts_DescendingByCount(t *testing.T) {
	m := map[string]int{
		"alpha": 3,
		"beta":  10,
		"gamma": 1,
	}
	got := sortedCounts(m)
	if len(got) != 3 {
		t.Fatalf("sortedCounts: got %d pairs, want 3", len(got))
	}
	if got[0].key != "beta" || got[0].count != 10 {
		t.Errorf("first: got %v, want {beta, 10}", got[0])
	}
	if got[1].key != "alpha" || got[1].count != 3 {
		t.Errorf("second: got %v, want {alpha, 3}", got[1])
	}
	if got[2].key != "gamma" || got[2].count != 1 {
		t.Errorf("third: got %v, want {gamma, 1}", got[2])
	}
}

func TestSortedCounts_AlphabeticalTiebreak(t *testing.T) {
	m := map[string]int{
		"cherry": 5,
		"apple":  5,
		"banana": 5,
	}
	got := sortedCounts(m)
	if len(got) != 3 {
		t.Fatalf("sortedCounts: got %d pairs, want 3", len(got))
	}
	// Same count, so alphabetical order: apple, banana, cherry.
	if got[0].key != "apple" {
		t.Errorf("tiebreak first: got %q, want apple", got[0].key)
	}
	if got[1].key != "banana" {
		t.Errorf("tiebreak second: got %q, want banana", got[1].key)
	}
	if got[2].key != "cherry" {
		t.Errorf("tiebreak third: got %q, want cherry", got[2].key)
	}
}

func TestSortedCounts_EmptyMap(t *testing.T) {
	got := sortedCounts(map[string]int{})
	if len(got) != 0 {
		t.Errorf("empty map: got %d pairs, want 0", len(got))
	}
}

func TestSortedCounts_NilMap(t *testing.T) {
	got := sortedCounts(nil)
	if len(got) != 0 {
		t.Errorf("nil map: got %d pairs, want 0", len(got))
	}
}
