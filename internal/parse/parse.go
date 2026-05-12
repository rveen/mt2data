// Package parse implements Stage 1: Markdown+MT → typed block tree.
//
// MT format specifics handled:
//   - Numeric requirement IDs on standalone lines (e.g. "1867448", "R-869098", "STM1207583")
//   - ASIL markers on standalone lines: "ASIL = B", "ASIL = B(D)", "ASIL = D", "ASIL = QM"
//   - TOON tables: name[...]{col1|col2}:  followed by indented rows
//   - Headings with parenthesised IDs: "### 1.1.1 Title (1867456)"
//   - Plain section-number headings (no # prefix): "2.1.4.1.3.9.2 Title (ID)"
//   - Note: / Hint: / Warning: prefixes on paragraphs
//   - Figure placeholders: "(figure)", "[image]", "Figure :" lines
//   - Signal block lines: "[PDU_Xxx]", "[CMM_Xxx]"
package parse

import (
	"crypto/sha1"
	"fmt"
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/provenance"
)

// BlockType categorises a parsed block.
type BlockType string

const (
	TypeHeading     BlockType = "heading"
	TypeReqID       BlockType = "req_id"
	TypeASIL        BlockType = "asil"
	TypeParagraph   BlockType = "paragraph"
	TypeToonTable   BlockType = "toon_table"
	TypeList        BlockType = "list"
	TypeNote        BlockType = "note"
	TypeHint        BlockType = "hint"
	TypeWarning     BlockType = "warning"
	TypeFigure      BlockType = "figure"
	TypeSignalBlock BlockType = "signal_block"
	TypePlainHead   BlockType = "plain_heading"
	TypeBlank       BlockType = "blank"
)

// Block is one structural unit of a parsed MT document.
type Block struct {
	ID       string            // stable content hash
	Type     BlockType
	Level    int               // heading level (1-6); 0 for non-headings
	Raw      string            // original text (all lines joined with \n)
	HeadID   string            // ID extracted from heading parentheses, if any
	ASIL     string            // ASIL level if Type==TypeASIL ("B", "B(D)", "D", "QM")
	ReqDocID string            // requirement DB ID if Type==TypeReqID
	Source   provenance.Source
}

// ToonTable is a parsed TOON inline table.
type ToonTable struct {
	Block
	Name    string
	Columns []string
	Rows    [][]string
}

// Document is the parsed block tree for one MT file.
type Document struct {
	Blocks     []Block
	ToonTables map[string]*ToonTable // keyed by block ID
}

// ----- compiled regexps -----

var (
	reHeading    = regexp.MustCompile(`^(#{1,6})\s+(.+)`)
	reHeadingID  = regexp.MustCompile(`\(([A-Z]?[A-Z0-9\-]{3,}|\d{5,})\)\s*$`)
	reReqID      = regexp.MustCompile(`^(R-\d+|STM\d+|\d{5,})\s*$`)
	reASIL       = regexp.MustCompile(`^ASIL\s*=\s*([A-Z()/QM]+)\s*$`)
	reToonHeader = regexp.MustCompile(`^(\w+)\[([^\]]*)\]\{([^}]+)\}:\s*$`)
	reListItem   = regexp.MustCompile(`^(\s*[-*]|\s*\d+\.)`)
	rePlainHead  = regexp.MustCompile(`^(\d+(?:\.\d+){1,8})\s+(.+)`)
	reSignalLine = regexp.MustCompile(`^\[(PDU|CMM|CIC|CMM|ECU)[_A-Za-z0-9]*\]`)
	reFigure     = regexp.MustCompile(`(?i)\(figure\)|\[image\]|^figure\s*:|^figure\s+:`)
)

// Parse reads text (contents of an MT file) and returns a Document.
func Parse(text string) *Document {
	lines := strings.Split(text, "\n")
	doc := &Document{
		ToonTables: make(map[string]*ToonTable),
	}

	i := 0
	for i < len(lines) {
		line := lines[i]

		// ---- Blank line: skip
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}

		// ---- Markdown heading
		if m := reHeading.FindStringSubmatch(line); m != nil {
			blk := Block{
				Type:   TypeHeading,
				Level:  len(m[1]),
				Raw:    line,
				Source: provenance.Source{LineStart: i + 1, LineEnd: i + 1},
			}
			// Extract parenthesised ID from heading text
			title := strings.TrimSpace(m[2])
			if id := reHeadingID.FindStringSubmatch(title); id != nil {
				blk.HeadID = id[1]
				// Strip the "(ID)" from the title stored in Raw
				// (Raw stays as-is; HeadID is extracted)
			}
			blk.ID = blockID(blk.Type, i, line)
			doc.Blocks = append(doc.Blocks, blk)
			i++
			continue
		}

		// ---- ASIL marker
		if m := reASIL.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			blk := Block{
				Type:   TypeASIL,
				ASIL:   m[1],
				Raw:    line,
				Source: provenance.Source{LineStart: i + 1, LineEnd: i + 1},
			}
			blk.ID = blockID(blk.Type, i, line)
			doc.Blocks = append(doc.Blocks, blk)
			i++
			continue
		}

		// ---- Standalone requirement DB ID
		if m := reReqID.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			blk := Block{
				Type:     TypeReqID,
				ReqDocID: m[1],
				Raw:      line,
				Source:   provenance.Source{LineStart: i + 1, LineEnd: i + 1},
			}
			blk.ID = blockID(blk.Type, i, line)
			doc.Blocks = append(doc.Blocks, blk)
			i++
			continue
		}

		// ---- TOON table header
		if m := reToonHeader.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			start := i
			tt := &ToonTable{
				Block: Block{
					Type:   TypeToonTable,
					Source: provenance.Source{LineStart: i + 1},
				},
				Name: m[1],
			}
			// Parse columns — may be separated by | or ,
			colStr := m[3]
			if strings.Contains(colStr, "|") {
				tt.Columns = splitTrimmed(colStr, "|")
			} else {
				tt.Columns = splitTrimmed(colStr, ",")
			}
			i++ // advance past header line
			// Collect indented rows
			for i < len(lines) {
				rowLine := lines[i]
				if strings.TrimSpace(rowLine) == "" {
					i++
					continue
				}
				// Rows are indented (at least 2 spaces or a tab)
				if !strings.HasPrefix(rowLine, "  ") && !strings.HasPrefix(rowLine, "\t") {
					break
				}
				row := strings.TrimSpace(rowLine)
				var cells []string
				if strings.Contains(row, "|") {
					cells = splitTrimmed(row, "|")
				} else {
					cells = splitTrimmed(row, ",")
				}
				tt.Rows = append(tt.Rows, cells)
				i++
			}
			tt.Raw = strings.Join(lines[start:i], "\n")
			tt.Source.LineEnd = i
			tt.Block.ID = blockID(TypeToonTable, start, tt.Raw)
			doc.Blocks = append(doc.Blocks, tt.Block)
			doc.ToonTables[tt.Block.ID] = tt
			continue
		}

		// ---- Figure placeholder
		if reFigure.MatchString(strings.TrimSpace(line)) {
			blk := Block{
				Type:   TypeFigure,
				Raw:    line,
				Source: provenance.Source{LineStart: i + 1, LineEnd: i + 1},
			}
			blk.ID = blockID(blk.Type, i, line)
			doc.Blocks = append(doc.Blocks, blk)
			i++
			continue
		}

		// ---- Signal block line (single-line signal definition)
		if reSignalLine.MatchString(strings.TrimSpace(line)) {
			// Collect consecutive signal lines
			start := i
			var sb strings.Builder
			sb.WriteString(line)
			i++
			for i < len(lines) {
				next := strings.TrimSpace(lines[i])
				if next == "" || reSignalLine.MatchString(next) || strings.HasPrefix(next, "[") {
					if next == "" {
						break
					}
					sb.WriteByte('\n')
					sb.WriteString(lines[i])
					i++
				} else {
					break
				}
			}
			raw := sb.String()
			blk := Block{
				Type:   TypeSignalBlock,
				Raw:    raw,
				Source: provenance.Source{LineStart: start + 1, LineEnd: i},
			}
			blk.ID = blockID(blk.Type, start, raw)
			doc.Blocks = append(doc.Blocks, blk)
			continue
		}

		// ---- List
		if reListItem.MatchString(line) {
			start := i
			var sb strings.Builder
			sb.WriteString(line)
			i++
			for i < len(lines) {
				next := lines[i]
				if strings.TrimSpace(next) == "" {
					break
				}
				// Continuation: indented or next list item
				if reListItem.MatchString(next) || strings.HasPrefix(next, "  ") || strings.HasPrefix(next, "\t") {
					sb.WriteByte('\n')
					sb.WriteString(next)
					i++
				} else {
					break
				}
			}
			raw := sb.String()
			blk := Block{
				Type:   TypeList,
				Raw:    raw,
				Source: provenance.Source{LineStart: start + 1, LineEnd: i},
			}
			blk.ID = blockID(blk.Type, start, raw)
			doc.Blocks = append(doc.Blocks, blk)
			continue
		}

		// ---- Paragraph (collect until blank line or block-starting line)
		start := i
		var sb strings.Builder
		sb.WriteString(line)
		i++
		for i < len(lines) {
			next := lines[i]
			if strings.TrimSpace(next) == "" {
				break
			}
			// Stop if next line would start a new structural block
			if isBlockStart(next) {
				break
			}
			sb.WriteByte('\n')
			sb.WriteString(next)
			i++
		}
		raw := sb.String()
		trimmed := strings.TrimSpace(raw)
		btype := classifyParagraph(trimmed)
		blk := Block{
			Type:   btype,
			Raw:    raw,
			Source: provenance.Source{LineStart: start + 1, LineEnd: i, OrigText: trimmed},
		}
		// Check for plain heading (bare section number)
		if btype == TypeParagraph {
			if m := rePlainHead.FindStringSubmatch(trimmed); m != nil {
				// Only treat as plain heading if it's a single line
				if !strings.Contains(trimmed, "\n") {
					blk.Type = TypePlainHead
					blk.Level = 0
				}
			}
		}
		blk.ID = blockID(blk.Type, start, raw)
		doc.Blocks = append(doc.Blocks, blk)
	}

	return doc
}

// isBlockStart returns true if line would start a structural block (heading, ASIL, req ID, etc.)
func isBlockStart(line string) bool {
	t := strings.TrimSpace(line)
	return reHeading.MatchString(line) ||
		reASIL.MatchString(t) ||
		reReqID.MatchString(t) ||
		reToonHeader.MatchString(t) ||
		reListItem.MatchString(line)
}

// classifyParagraph returns the block type for a prose paragraph based on prefix.
func classifyParagraph(text string) BlockType {
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "note:") || strings.HasPrefix(lower, "note ") {
		return TypeNote
	}
	if strings.HasPrefix(lower, "hint:") || strings.HasPrefix(lower, "hint ") {
		return TypeHint
	}
	if strings.HasPrefix(lower, "warning:") || strings.HasPrefix(lower, "caution:") {
		return TypeWarning
	}
	return TypeParagraph
}

// blockID returns a stable identifier for a block based on type, line position, and content.
func blockID(t BlockType, lineStart int, content string) string {
	h := sha1.New()
	fmt.Fprintf(h, "%s:%d:%s", t, lineStart, content)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// splitTrimmed splits s by sep and trims whitespace from each element.
func splitTrimmed(s, sep string) []string {
	parts := strings.Split(s, sep)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
