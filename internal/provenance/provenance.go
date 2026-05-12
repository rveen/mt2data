// Package provenance defines source-span types carried on every IR record.
package provenance

// Source locates an extracted record back to the input document.
type Source struct {
	BlockID   string `json:"block_id"`
	Clause    string `json:"clause,omitempty"`
	LineStart int    `json:"line_start"`
	LineEnd   int    `json:"line_end"`
	OrigText  string `json:"orig_text,omitempty"`
}
