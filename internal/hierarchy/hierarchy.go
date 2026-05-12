// Package hierarchy implements Stage 2: clause/section tree recovery from parsed blocks.
package hierarchy

import (
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/parse"
	"github.com/rveen/mt2data/internal/schema"
)

// reSectionNum extracts a leading section number from heading text.
var reSectionNum = regexp.MustCompile(`^(\d+(?:\.\d+)*)(?:\s|$)`)

// Result holds the recovered clause tree and any attached block assignments.
type Result struct {
	Clauses    []schema.Clause
	BlockClause map[string]string // block ID → clause ID
}

// Build walks the parsed block list, builds the clause tree, and attaches
// non-heading blocks to the deepest enclosing clause.
func Build(doc *parse.Document, rep *issues.Reporter) *Result {
	res := &Result{
		BlockClause: make(map[string]string),
	}

	// Stack tracks open clauses by heading level.
	// stack[i] is the clause ID at heading level i+1.
	var stack []string // clause IDs; index 0 = level 1
	var prevLevel int

	clauseMap := make(map[string]*schema.Clause) // clause ID → pointer for block attachment

	for _, blk := range doc.Blocks {
		if blk.Type == parse.TypeHeading {
			level := blk.Level
			title, secID := extractHeadingParts(blk.Raw)

			// If section ID not in heading text, use HeadID from parentheses (fallback)
			if secID == "" {
				secID = deriveID(title, stack, level)
			}

			// Detect skipped heading levels
			if prevLevel > 0 && level > prevLevel+1 {
				rep.Add(issues.KindSkippedHeading,
					blk.ID,
					"heading jumps from level "+levelStr(prevLevel)+" to "+levelStr(level))
			}
			prevLevel = level

			// Trim stack to parent level
			if level <= len(stack) {
				stack = stack[:level-1]
			}

			// Build path
			path := make([]string, len(stack)+1)
			copy(path, stack)
			path[len(stack)] = secID

			c := schema.Clause{
				ID:    secID,
				Title: title,
				Path:  path,
			}
			res.Clauses = append(res.Clauses, c)
			clauseMap[secID] = &res.Clauses[len(res.Clauses)-1]

			stack = append(stack, secID)
			continue
		}

		if blk.Type == parse.TypePlainHead {
			rep.Add(issues.KindPlainHeading, blk.ID, "plain heading: "+strings.TrimSpace(blk.Raw))
			continue
		}

		// Attach non-heading block to the current deepest clause
		if len(stack) > 0 {
			clauseID := stack[len(stack)-1]
			res.BlockClause[blk.ID] = clauseID
			if c, ok := clauseMap[clauseID]; ok {
				c.Blocks = append(c.Blocks, blk.ID)
			}
		}
	}

	return res
}

// extractHeadingParts returns (title, sectionNumber) from a heading line.
// The heading line may be "## 1.1 Title (ID)" or "# Title".
func extractHeadingParts(raw string) (title, secNum string) {
	// Strip leading # chars
	text := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(raw), "#"))

	// Extract leading section number
	if m := reSectionNum.FindStringSubmatch(text); m != nil {
		secNum = m[1]
		text = strings.TrimSpace(text[len(m[0]):])
	}

	// Strip trailing parenthesised ID from title display
	reTrailingID := regexp.MustCompile(`\s*\([A-Z0-9][A-Z0-9\-]{3,}\)\s*$|\s*\(\d{5,}\)\s*$`)
	title = reTrailingID.ReplaceAllString(text, "")
	if title == "" {
		title = text
	}
	return title, secNum
}

// deriveID constructs a section ID when none is in the heading text.
// Uses parent stack + a positional suffix when no number is available.
func deriveID(title string, stack []string, _ int) string {
	if len(stack) > 0 {
		parent := stack[len(stack)-1]
		// If parent looks numeric, append a sub-number placeholder
		if reSectionNum.MatchString(parent) {
			return parent + ".x"
		}
	}
	// Fallback: slugify the title
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	if len(slug) > 30 {
		slug = slug[:30]
	}
	return slug
}

func levelStr(l int) string {
	switch l {
	case 1:
		return "H1"
	case 2:
		return "H2"
	case 3:
		return "H3"
	case 4:
		return "H4"
	case 5:
		return "H5"
	case 6:
		return "H6"
	default:
		return "H?"
	}
}
