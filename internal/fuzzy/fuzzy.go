// Package fuzzy provides a small, pure subsequence matcher used to filter and
// rank a candidate list (e.g. Clockify project names) against a typed query.
// It has no I/O so the ranking behavior can be exercised directly in tests.
package fuzzy

import (
	"sort"
	"strings"
)

// Match holds a candidate's original index and its match score against a query.
type Match struct {
	Index int
	Score int
}

// score reports whether query is a (case-insensitive) subsequence of candidate
// and, if so, a quality score where higher is better. Contiguous runs and an
// early first match are rewarded; an exact or prefix match scores highest. A
// non-subsequence returns ok=false.
func score(query, candidate string) (int, bool) {
	q := strings.ToLower(query)
	c := strings.ToLower(candidate)

	if q == "" {
		return 0, true
	}
	if q == c {
		return 1_000_000, true
	}

	const (
		matchBonus        = 10
		contiguousBonus   = 15
		prefixBonus       = 40
		leadingGapPenalty = 1
	)

	s := 0
	qi := 0
	prevMatched := -2
	firstMatch := -1
	for ci := 0; ci < len(c) && qi < len(q); ci++ {
		if c[ci] != q[qi] {
			continue
		}
		if firstMatch == -1 {
			firstMatch = ci
		}
		s += matchBonus
		if ci == prevMatched+1 {
			s += contiguousBonus
		}
		prevMatched = ci
		qi++
	}
	if qi != len(q) {
		return 0, false
	}

	if firstMatch == 0 {
		s += prefixBonus
	} else {
		s -= firstMatch * leadingGapPenalty
	}
	return s, true
}

// Rank returns the candidates matching query, best first. Ties preserve the
// original input order. An empty query keeps every candidate in input order.
func Rank(query string, candidates []string) []Match {
	matches := make([]Match, 0, len(candidates))
	for i, cand := range candidates {
		if sc, ok := score(query, cand); ok {
			matches = append(matches, Match{Index: i, Score: sc})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})
	return matches
}
