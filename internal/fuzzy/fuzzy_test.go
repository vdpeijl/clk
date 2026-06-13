package fuzzy

import "testing"

func TestRankFiltersNonSubsequences(t *testing.T) {
	got := Rank("clk", []string{"clockify", "Internal Tools", "clk-core", "website"})
	for _, m := range got {
		if m.Index == 3 { // "website" has no c/l/k subsequence
			t.Errorf("website should not match query clk")
		}
	}
}

func TestRankExactMatchWinsOverSubsequence(t *testing.T) {
	got := Rank("api", []string{"Mapping API", "api"})
	if len(got) < 2 {
		t.Fatalf("expected both candidates to match, got %d", len(got))
	}
	if got[0].Index != 1 {
		t.Errorf("exact match should rank first, got index %d", got[0].Index)
	}
}

func TestRankPrefixBeatsLaterMatch(t *testing.T) {
	got := Rank("int", []string{"My Integration", "Internal"})
	if len(got) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(got))
	}
	if got[0].Index != 1 {
		t.Errorf("prefix match (Internal) should rank first, got index %d", got[0].Index)
	}
}

func TestRankCaseInsensitive(t *testing.T) {
	got := Rank("ACME", []string{"acme corp"})
	if len(got) != 1 {
		t.Fatalf("expected case-insensitive match, got %d", len(got))
	}
}

func TestRankEmptyQueryKeepsAllInOrder(t *testing.T) {
	in := []string{"a", "b", "c"}
	got := Rank("", in)
	if len(got) != 3 {
		t.Fatalf("empty query should keep all, got %d", len(got))
	}
	for i, m := range got {
		if m.Index != i {
			t.Errorf("order not preserved: position %d has index %d", i, m.Index)
		}
	}
}

func TestRankNoMatches(t *testing.T) {
	got := Rank("zzz", []string{"alpha", "beta"})
	if len(got) != 0 {
		t.Errorf("expected no matches, got %d", len(got))
	}
}
