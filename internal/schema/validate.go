package schema

import (
	"encoding/json"
	"fmt"
	"os"
)

// ValidationError records one schema violation.
type ValidationError struct {
	Field   string
	Where   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Field, e.Where, e.Message)
}

// Validate checks that all required fields are populated in doc.
// It returns a list of validation errors (empty means valid).
func Validate(doc *Document) []ValidationError {
	var errs []ValidationError

	for i, r := range doc.Requirements {
		where := fmt.Sprintf("requirements[%d]", i)
		if r.ID == "" {
			errs = append(errs, ValidationError{"id", where, "missing"})
		}
		if r.Text == "" {
			errs = append(errs, ValidationError{"text", where, "missing"})
		}
		if r.Source.BlockID == "" {
			errs = append(errs, ValidationError{"source.block_id", where, "missing"})
		}
	}

	for i, p := range doc.Parameters {
		where := fmt.Sprintf("parameters[%d]", i)
		if p.Name == "" {
			errs = append(errs, ValidationError{"name", where, "missing"})
		}
		if p.Source.BlockID == "" {
			errs = append(errs, ValidationError{"source.block_id", where, "missing"})
		}
	}

	// Duplicate ID check
	seen := make(map[string]bool)
	for i, r := range doc.Requirements {
		if r.ID != "" {
			if seen[r.ID] {
				errs = append(errs, ValidationError{
					"id",
					fmt.Sprintf("requirements[%d]", i),
					fmt.Sprintf("duplicate ID %q", r.ID),
				})
			}
			seen[r.ID] = true
		}
	}

	return errs
}

// LoadAndValidate reads a JSON file into a Document and validates it.
// Reports validation errors to stderr and returns the document.
func LoadAndValidate(path string) (*Document, []ValidationError, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}
	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	errs := Validate(&doc)
	return &doc, errs, nil
}
