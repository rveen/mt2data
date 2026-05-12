// Package classify implements Stage 3: heuristic block type assignment.
//
// Each block receives a semantic type beyond the structural type assigned by
// the parser. The classifier records which classifier produced the decision so
// that LLM-fallback contributions are auditable.
package classify

import (
	"regexp"
	"strings"

	"github.com/rveen/mt2data/internal/parse"
)

// SemanticType is the semantic category assigned to a block.
type SemanticType string

const (
	Prose            SemanticType = "prose"
	ParameterTable   SemanticType = "parameter_table"
	RequirementTable SemanticType = "requirement_table"
	TreeTable        SemanticType = "tree_table"
	SignalTable      SemanticType = "signal_table"
	Note             SemanticType = "note"
	Example          SemanticType = "example"
	Formula          SemanticType = "formula"
	FigureRef        SemanticType = "figure_ref"
	CrossRef         SemanticType = "cross_ref"
	Boilerplate      SemanticType = "boilerplate"
	LayoutTable      SemanticType = "layout_table"
	Unknown          SemanticType = "unknown"
)

// ClassifierKind records how a decision was made.
type ClassifierKind string

const (
	ClassifierHeuristic ClassifierKind = "heuristic"
	ClassifierLLM       ClassifierKind = "llm"
)

// Decision records the semantic type and classifier for one block.
type Decision struct {
	BlockID    string
	Semantic   SemanticType
	Classifier ClassifierKind
}

// -- heuristics for TOON table column header keywords --

var paramColKeywords = []string{
	"min", "typ", "max", "typical", "minimum", "maximum",
	"unit", "symbol", "value", "att", "attenuation",
	"frequency", "tolerance", "accuracy", "resolution",
}

var reqColKeywords = []string{
	"requirement", "shall", "category", "applicability",
	"verification", "reference", "status", "priority",
}

var treeColKeywords = []string{
	"milestone", "payment", "acceptance", "phase", "stage",
	"item", "variant", "part", "deliverable",
}

var signalColKeywords = []string{
	"signal", "range", "resolution", "description", "basic value",
}

// boilerplatePatterns are substrings indicating non-content paragraphs.
var boilerplatePatterns = []string{
	"this page intentionally",
	"all rights reserved",
	"copyright ©",
	"proprietary and confidential",
	"intentionally left blank",
}

var reFormula = regexp.MustCompile(`[A-Za-z]\s*[=<>≤≥]\s*[\d.]+|[A-Za-z]\s*²|∑|√|∫`)

// Classify assigns a semantic type to every block in the document.
func Classify(doc *parse.Document) []Decision {
	var decisions []Decision
	for _, blk := range doc.Blocks {
		sem, cls := classifyBlock(blk, doc)
		decisions = append(decisions, Decision{
			BlockID:    blk.ID,
			Semantic:   sem,
			Classifier: cls,
		})
	}
	return decisions
}

func classifyBlock(blk parse.Block, doc *parse.Document) (SemanticType, ClassifierKind) {
	switch blk.Type {
	case parse.TypeToonTable:
		tt := doc.ToonTables[blk.ID]
		if tt == nil {
			return Unknown, ClassifierHeuristic
		}
		return classifyToonTable(tt), ClassifierHeuristic

	case parse.TypeNote:
		return Note, ClassifierHeuristic

	case parse.TypeHint:
		return Note, ClassifierHeuristic

	case parse.TypeWarning:
		return Note, ClassifierHeuristic

	case parse.TypeFigure:
		return FigureRef, ClassifierHeuristic

	case parse.TypeSignalBlock:
		return SignalTable, ClassifierHeuristic

	case parse.TypeASIL, parse.TypeReqID, parse.TypeHeading, parse.TypePlainHead:
		// Structural metadata — not content to classify semantically
		return Boilerplate, ClassifierHeuristic

	case parse.TypeList:
		// Lists may be requirements (if they contain shall/must) or prose
		lower := strings.ToLower(blk.Raw)
		if containsImperative(lower) {
			return Prose, ClassifierHeuristic // treated as prose for extraction
		}
		return Prose, ClassifierHeuristic

	case parse.TypeParagraph:
		lower := strings.ToLower(blk.Raw)
		for _, bp := range boilerplatePatterns {
			if strings.Contains(lower, bp) {
				return Boilerplate, ClassifierHeuristic
			}
		}
		if reFormula.MatchString(blk.Raw) && !containsImperative(lower) {
			return Formula, ClassifierHeuristic
		}
		return Prose, ClassifierHeuristic

	default:
		return Unknown, ClassifierHeuristic
	}
}

func classifyToonTable(tt *parse.ToonTable) SemanticType {
	cols := strings.ToLower(strings.Join(tt.Columns, " "))

	// Named table patterns take priority
	name := strings.ToLower(tt.Name)
	if name == "plan" || name == "schedule" || name == "variants" {
		return TreeTable
	}
	if name == "filter" || name == "limits" || name == "spec" {
		return ParameterTable
	}

	score := func(keywords []string) int {
		n := 0
		for _, kw := range keywords {
			if strings.Contains(cols, kw) {
				n++
			}
		}
		return n
	}

	paramScore := score(paramColKeywords)
	reqScore := score(reqColKeywords)
	treeScore := score(treeColKeywords)
	signalScore := score(signalColKeywords)

	max := paramScore
	winner := ParameterTable
	if reqScore > max {
		max = reqScore
		winner = RequirementTable
	}
	if treeScore > max {
		max = treeScore
		winner = TreeTable
	}
	if signalScore > max {
		max = signalScore
		winner = SignalTable
	}

	// Layout table heuristic: very few cells, no header pattern
	if len(tt.Rows) == 0 {
		return LayoutTable
	}
	if max == 0 {
		return Unknown
	}
	return winner
}

var imperativeWords = []string{"shall", "must", "should", "may not", "shall not", "must not"}

func containsImperative(lower string) bool {
	for _, w := range imperativeWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}
