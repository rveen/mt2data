package hierarchy

import (
	"fmt"
	"strings"

	"github.com/rveen/mt2data/internal/schema"
)

// ProductTree builds a schema.Tree that represents the product/component breakdown
// implied by the clause hierarchy, pruned to branches that contain at least one
// requirement in their subtree.
//
// Each node label is "<ID> <Title> [N]" where N is the total requirement count
// for that subtree.
func ProductTree(clauses []schema.Clause, reqs []schema.Requirement) *schema.Tree {
	if len(clauses) == 0 {
		return nil
	}

	// Count requirements per clause section.
	reqCount := make(map[string]int, len(reqs))
	for _, r := range reqs {
		if r.Section != "" {
			reqCount[r.Section]++
		}
	}

	// Build parent→children and id→clause maps.
	byID := make(map[string]*schema.Clause, len(clauses))
	children := make(map[string][]string, len(clauses))
	var roots []string

	for i := range clauses {
		c := &clauses[i]
		byID[c.ID] = c
		if len(c.Path) <= 1 {
			roots = append(roots, c.ID)
		} else {
			parentID := c.Path[len(c.Path)-2]
			children[parentID] = append(children[parentID], c.ID)
		}
	}

	// Recursively compute subtree requirement counts.
	var subtreeCount func(id string) int
	subtreeCount = func(id string) int {
		n := reqCount[id]
		for _, ch := range children[id] {
			n += subtreeCount(ch)
		}
		return n
	}

	// Build a TreeNode for a clause, pruning branches with zero subtree count.
	var buildNode func(id string) *schema.TreeNode
	buildNode = func(id string) *schema.TreeNode {
		total := subtreeCount(id)
		if total == 0 {
			return nil
		}
		c := byID[id]
		node := &schema.TreeNode{
			ID:    id,
			Label: fmt.Sprintf("%s %s [%d]", id, c.Title, total),
		}
		for _, chID := range children[id] {
			if ch := buildNode(chID); ch != nil {
				node.Children = append(node.Children, *ch)
			}
		}
		return node
	}

	// Build a synthetic root that holds all top-level sections.
	var topChildren []schema.TreeNode
	for _, rid := range roots {
		if n := buildNode(rid); n != nil {
			topChildren = append(topChildren, *n)
		}
	}
	if len(topChildren) == 0 {
		return nil
	}

	root := schema.TreeNode{
		ID:       "product_structure",
		Label:    "Product Structure",
		Children: topChildren,
	}
	return &schema.Tree{
		ID:     "product_structure",
		Source: "clause_hierarchy",
		Root:   root,
	}
}

// ProductTreeTOON renders the product structure tree as an indented plain-text outline
// wrapped in a TOON-style block header.
func ProductTreeTOON(tree *schema.Tree) string {
	if tree == nil {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "product_structure[1]{Component/Section}:\n")
	var walk func(node schema.TreeNode, depth int)
	walk = func(node schema.TreeNode, depth int) {
		if node.ID == "product_structure" {
			for _, ch := range node.Children {
				walk(ch, depth)
			}
			return
		}
		indent := strings.Repeat("  ", depth+1)
		sb.WriteString(indent)
		sb.WriteString(node.Label)
		sb.WriteByte('\n')
		for _, ch := range node.Children {
			walk(ch, depth+1)
		}
	}
	walk(tree.Root, 0)
	return sb.String()
}
