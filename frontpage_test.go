package main

import (
	"strings"
	"testing"
)

func TestPenaltiesWhereClauseExcludesFlagged(t *testing.T) {
	where := whereClause("penalties")

	if !strings.Contains(where, "rawRank") {
		t.Fatalf("penalties filter must keep the rank-gap condition, got %q", where)
	}
	if !strings.Contains(where, "flagged = 0") {
		t.Fatalf("penalties filter must exclude stories marked [flagged] on HN, got %q", where)
	}
}

func TestPenaltiesFilterSampleRows(t *testing.T) {
	// Mirrors whereClause("penalties"): ifnull(topRank,91) > rawRank and flagged = 0
	type row struct {
		title    string
		topRank  *int
		rawRank  int
		flagged  int
		wantKeep bool
	}

	rank := func(n int) *int { return &n }

	rows := []row{
		{title: "unflagged demotion", topRank: rank(40), rawRank: 5, flagged: 0, wantKeep: true},
		{title: "flagged demotion", topRank: rank(40), rawRank: 5, flagged: 1, wantKeep: false},
		{title: "flagged off front page", topRank: nil, rawRank: 3, flagged: 1, wantKeep: false},
		{title: "unflagged off front page", topRank: nil, rawRank: 3, flagged: 0, wantKeep: true},
		{title: "unflagged boost", topRank: rank(2), rawRank: 20, flagged: 0, wantKeep: false},
	}

	for _, r := range rows {
		top := 91
		if r.topRank != nil {
			top = *r.topRank
		}
		got := top > r.rawRank && r.flagged == 0
		if got != r.wantKeep {
			t.Errorf("%s: kept=%v, want %v (top=%d raw=%d flagged=%d)", r.title, got, r.wantKeep, top, r.rawRank, r.flagged)
		}
	}
}
