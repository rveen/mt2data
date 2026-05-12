// Package issues provides an append-only channel for flagged review items.
package issues

// Kind values for Issue.
const (
	KindUnknownBlock   = "unknown_block"
	KindUnresolvedRef  = "unresolved_ref"
	KindAmbiguousUnit  = "ambiguous_unit"
	KindDuplicateID    = "duplicate_id"
	KindNearDuplicate  = "near_duplicate"
	KindMissingField   = "missing_field"
	KindSkippedHeading = "skipped_heading"
	KindPlainHeading   = "plain_heading"
)

// Issue records one flagged item for human review.
type Issue struct {
	Kind  string `json:"kind"`
	Where string `json:"where"`
	Note  string `json:"note"`
}

// Reporter accumulates issues from all pipeline stages.
type Reporter struct {
	items []Issue
}

// Add appends an issue.
func (r *Reporter) Add(kind, where, note string) {
	r.items = append(r.items, Issue{Kind: kind, Where: where, Note: note})
}

// All returns a copy of all accumulated issues.
func (r *Reporter) All() []Issue {
	out := make([]Issue, len(r.items))
	copy(out, r.items)
	return out
}
