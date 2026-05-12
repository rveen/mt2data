// Package schema defines the intermediate representation (IR) produced by mt2data.
package schema

import (
	"github.com/rveen/mt2data/internal/issues"
	"github.com/rveen/mt2data/internal/provenance"
)

// Document is the top-level IR for a processed MT document.
type Document struct {
	ID           string        `json:"id"`
	Kind         string        `json:"kind"` // rfq | norm | datasheet | other
	Metadata     DocumentMeta  `json:"metadata"`
	Clauses      []Clause      `json:"clauses,omitempty"`
	Components   []Component   `json:"components,omitempty"`
	Connections  []Connection  `json:"connections,omitempty"`
	Parameters   []Parameter   `json:"parameters,omitempty"`
	Requirements []Requirement `json:"requirements,omitempty"`
	Trees        []Tree        `json:"trees,omitempty"`
	References   []Reference   `json:"references,omitempty"`
	Issues       []issues.Issue `json:"issues,omitempty"`
}

// Component is a physical or logical product element identified from requirements.
type Component struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`        // module|sensor|actuator|interface|software|other
	Description string `json:"description,omitempty"`
}

// Connection is an interface or dependency between two components.
type Connection struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Interface   string `json:"interface,omitempty"`   // CAN|HVDC|HV-analog|LV-analog|digital-IO|thermal|etc.
	Description string `json:"description,omitempty"`
}

// DocumentMeta carries document-level metadata.
type DocumentMeta struct {
	Title      string `json:"title,omitempty"`
	Edition    string `json:"edition,omitempty"`
	Date       string `json:"date,omitempty"`
	SourceFile string `json:"source_file,omitempty"`
}

// Clause is one node in the recovered clause/section hierarchy.
type Clause struct {
	ID     string   `json:"id"`    // "4.6.3"
	Title  string   `json:"title"`
	Path   []string `json:"path"`  // ["4", "4.6", "4.6.3"]
	Blocks []string `json:"blocks,omitempty"` // block IDs attached to this clause
}

// Quantity is a parsed scalar value with a unit.
type Quantity struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
	Raw   string  `json:"raw,omitempty"`
}

// Condition is a qualifier propagated into a parameter row.
type Condition struct {
	Quantity string    `json:"quantity"`
	Value    *Quantity `json:"value,omitempty"`
	Raw      string    `json:"raw,omitempty"`
}

// Parameter is one row from a parameter/datasheet table.
type Parameter struct {
	ID         string               `json:"id"`
	Name       string               `json:"name"`
	Symbol     string               `json:"symbol,omitempty"`
	Min        *Quantity            `json:"min,omitempty"`
	Typ        *Quantity            `json:"typ,omitempty"`
	Max        *Quantity            `json:"max,omitempty"`
	Conditions []Condition          `json:"conditions,omitempty"`
	Source     provenance.Source    `json:"source"`
}

// Requirement is one extracted normative statement.
type Requirement struct {
	ID               string            `json:"id"`
	IDIsAuto         bool              `json:"id_is_auto,omitempty"`
	Text             string            `json:"text"`
	Title            string            `json:"title,omitempty"`
	Section          string            `json:"section,omitempty"`
	Item             string            `json:"item,omitempty"`
	Domain           string            `json:"domain,omitempty"`
	Verb             string            `json:"verb,omitempty"`
	Verification     string            `json:"verification,omitempty"`
	FunctionalSafety string            `json:"functional_safety,omitempty"`
	Compound         bool              `json:"compound,omitempty"`
	References       []Reference       `json:"references,omitempty"`
	Source           provenance.Source `json:"source"`
}

// TreeNode is one node in a data hierarchy tree.
type TreeNode struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Children []TreeNode `json:"children,omitempty"`
}

// Tree is a recovered data hierarchy (product structure, classification taxonomy, etc.).
type Tree struct {
	ID     string   `json:"id"`
	Source string   `json:"source_block,omitempty"`
	Root   TreeNode `json:"root"`
}

// RefKind classifies a resolved or unresolved reference.
type RefKind string

const (
	RefKindDocument  RefKind = "document"
	RefKindStandard  RefKind = "standard"
	RefKindSection   RefKind = "section"
	RefKindSignal    RefKind = "signal"
	RefKindUnknown   RefKind = "unknown"
)

// Reference is a cross-reference extracted from the document.
type Reference struct {
	From     string  `json:"from"`      // block ID of the referencing block
	Raw      string  `json:"raw"`       // original text token
	Kind     RefKind `json:"kind"`
	Norm     string  `json:"norm,omitempty"`
	Edition  string  `json:"edition,omitempty"`
	Clause   string  `json:"clause,omitempty"`
	Resolved bool    `json:"resolved"`
}
