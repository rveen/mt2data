# mt2data redesign

A design and handoff document for refactoring `mt2data` into a staged library plus thin CLI.

## Context

`mt2data` consumes the Markdown output of `pdf2mt` and produces a clean, structured dataset. It is used on two document classes:

- **RFQs** — product requirements documents from customers.
- **Referenced documents** — norms, standards, datasheets cited by the RFQ.

Both classes feed downstream stages (model construction, code extraction, compliance reporting). Those stages need a single, consistent IR regardless of source document class.

This document captures the redesign discussion. It is intended as input to a Claude Code session in the `mt2data` repository.

## Job description

`mt2data` is a **reduction**, not an interpretation. Every piece of output must be derivable from the input. Anything ambiguous is flagged, not guessed. Interpretation belongs to later LLM-driven stages.

Concretely, `mt2data` must:

- Recognize the document's structural elements (headings, paragraphs, tables, lists, figures, formulas, footnotes, cross-references) from the Markdown produced by `pdf2mt`.
- Recover semantic structure that the PDF threw away: clause containment, table-paragraph references, scope of "Note" / "EXAMPLE" blocks.
- Normalize content into a small set of typed data shapes.
- Carry provenance (page, span, original wording) on every output element.
- Flag, never invent.

## Three target shapes

Conflating these produces unusable output. Each has different normalization needs.

### Parameter tables

The datasheet shape: `quantity, symbol, min, typ, max, unit, conditions, notes`. Dense, regular. Conditions ("at Tj=25 °C, Vcc=3.3 V") are scattered: in row, in column header, in paragraph above, in footnote. The interesting work is **condition propagation** — pulling all qualifiers into each row so each row stands alone.

### Requirement tables

The RFQ shape: `id, text, category, applicability, verification method, references`. Rows are sentences or short paragraphs. The interesting work is **imperative extraction** ("shall", "must", "should") and **reference resolution** (in-line norm citations made explicit and structured).

### Data trees

Product hierarchies, document hierarchies, classification taxonomies. The interesting work is **containment recovery** — sometimes the tree is implicit in heading levels, sometimes in indented lists, sometimes in tables with parent/child columns, sometimes only in prose.

A document usually contains all three. A norm has parameter tables (limits) plus requirement tables (test conditions) plus a data tree (clause hierarchy). An RFQ has a product tree plus requirement tables plus performance-spec parameter tables.

## RFQs and norms share one schema

Downstream stages need to join them: "REQ-042 references ISO 16750-2 §4.6.3" must resolve to a structured pointer into the norm's IR. If the two are extracted to different shapes, manual stitching is unavoidable.

Design the schema for the **union** of what both contain, with optional fields rather than separate types. Norms have stronger clause numbering and weaker requirement IDs; RFQs have explicit requirement IDs and weaker clause structure. Both can carry both.

## Pipeline stages

Mostly deterministic. LLM only where rules genuinely fail.

### 1. Parse and segment

From the Markdown, build a tree of structural blocks: heading, paragraph, table, list, code, figure caption, note, example. Mechanical: a Markdown parser plus a small classifier for "Note:", "EXAMPLE:", "WARNING:" prefixes.

Output: annotated block tree with stable block IDs and source spans.

### 2. Recover hierarchy

Walk heading levels to build the clause/section tree. Attach blocks to the deepest enclosing heading. Resolve numbering ("4.6.3") into a tree path.

Mechanical for well-formed docs. Misformed docs (skipped heading levels, repeated numbers) are flagged, not "fixed".

### 3. Classify blocks

Each block gets a type: `prose`, `parameter_table`, `requirement_table`, `tree_table`, `note`, `example`, `formula`, `figure_ref`, `cross_ref`, `boilerplate`, `unknown`.

Heuristics first: header keywords, column patterns, regex on cell contents. The LLM is called only for blocks the heuristics can't classify confidently. Output records which classifier was used so heuristic vs LLM decisions are auditable.

### 4. Normalize tables

Highest-leverage stage. For each table:

- Detect orientation (rows-as-records, columns-as-records, matrix).
- Promote merged-cell headers to a flat header row.
- Detect and split combined columns ("min/typ/max" in one cell).
- Normalize units: parse "5 V", "5V", "5 volts" → `{value: 5, unit: "V"}`. Units stay symbolic; no conversions.
- Resolve footnote markers in cells to the footnote text.
- Propagate conditions from headers, captions, and surrounding paragraphs into per-row condition fields.
- Carry provenance: original cell text, page, table ID.

Output: a list of typed records, not a 2D grid.

### 5. Extract requirements from prose

Paragraphs with imperatives ("shall", "must", "is required to") become requirement records even when not in a table. Each gets:

- An ID. Synthesized if absent (e.g. `REQ-AUTO-0042`), flagged as auto.
- The verbatim sentence.
- The enclosing clause.
- Any in-line references.

### 6. Resolve references

"ISO 16750-2:2012, §4.6.3" → `{norm: "ISO 16750-2", edition: "2012", clause: "4.6.3"}`. "See Table 7" → pointer to local table ID. Unresolved references stay unresolved and are flagged.

Cross-document resolution happens later, when the norm's IR is also available. `mt2data` only produces the structured pointer.

### 7. Deduplicate and merge

A parameter that appears in a table and is restated in prose is one parameter, not two. Same for requirements repeated across sections.

Merging is conservative: only when normalized fields match exactly, including conditions. Near-matches are flagged for review, not merged.

### 8. Validate and report

Schema validation on the output. Coverage check: every input block is either classified into a typed output, marked as boilerplate, or flagged as unknown. The unknown rate is a quality metric.

## Output schema

```yaml
document:
  id: ...
  kind: rfq | norm | datasheet | other
  metadata: {title, edition, date, source_file, ...}

  clauses:                    # recovered hierarchy
    - id: "4.6.3"
      title: "Pulse 2a"
      path: ["4", "4.6", "4.6.3"]
      blocks: [block_id, ...]

  parameters:                 # parameter table rows, flattened
    - id: ...
      name: "Test pulse amplitude"
      symbol: "Us"
      min: {value: ..., unit: "V"}
      typ: ...
      max: {value: 50, unit: "V"}
      conditions:
        - {quantity: "Ri", value: 2, unit: "Ohm"}
      source: {clause: "4.6.3", block_id: ..., page: 17}

  requirements:               # requirement records
    - id: REQ-042
      text: "The DUT shall withstand pulse 2a..."
      category: ...
      verification: ...
      references:
        - {norm: "ISO 16750-2", clause: "4.6.3"}
      source: {...}

  trees:                      # data trees
    - id: product_structure
      root: ...
      nodes: [...]

  references:                 # all cross-refs, resolved or not
    - {from: ..., to: ..., resolved: true | false}

  issues:                     # everything flagged for review
    - {kind: "ambiguous_unit", where: ..., note: ...}
```

Provenance (`source`) on every record. IDs stable across re-runs (hash of normalized content + clause path) so diffs across RFQ revisions are meaningful.

## Where the LLM helps inside `mt2data`

Narrowly. Each LLM use is logged so its contribution to errors is measurable.

- **Block classification fallback** when heuristics return low confidence. Single-block input, single-label output, schema-validated.
- **Header disambiguation** in tables with merged or multi-row headers that defeated the parser. Validated against cell counts.
- **Condition extraction** from a paragraph preceding a parameter table. Output: list of `{quantity, value, unit}`. Validated by units parser.
- **Imperative extraction** in prose that doesn't use standard "shall" verbs but carries requirement force. Use sparingly; over-extraction pollutes the requirements list.

Everywhere else, deterministic. The LLM is a fallback, not the default.

## Failure modes to design against

- **Silent over-merging.** Two parameters with the same name from different conditions get collapsed. Mitigation: merge only on full-key match including conditions; otherwise emit both and flag.
- **Condition leakage.** A condition from clause 4.6.3 gets attached to a parameter in clause 4.7.1 because they are textually adjacent. Mitigation: condition propagation respects clause boundaries.
- **Boilerplate masquerading as content.** Norm preambles, copyright statements, "this page intentionally left blank". Build a small allow-list of known patterns and classify as `boilerplate`.
- **Tables that aren't tables.** Layout tables used for visual formatting. Heuristic: low cell count, no header row, mostly empty. Classify as `layout_table`, drop.
- **Numbering collisions.** "§4.6.3" in the RFQ and "§4.6.3" in the referenced norm are different. Always namespace by document ID.

## Refactoring approach

The existing `mt2data` is a single command-line tool. Reuse what is relevant, delete what is not, and start mostly from scratch toward this design.

Order of work:

1. **Lock the output schema.** Define the target schema as Go types. Generate JSON Schema for validation. Validate current `mt2data` output against it. The gaps tell you what to work on.
2. **Add provenance everywhere.** If current output doesn't carry source spans, retrofit this first. Everything else assumes it.
3. **Separate the three target shapes.** Parameters, requirements, trees in distinct outputs. Each has different normalization needs.
4. **Build the table normalizer.** Highest-leverage component. Multi-row headers, footnote resolution, condition propagation, unit parsing.
5. **Build the issues channel.** A flagged-for-review output, reviewed routinely. This is the signal that the tool is improving.
6. **Bring in the LLM only after** heuristics plateau. Measure first.

## Library + CLI split

Even if the first cut is small, put business logic in an importable library and keep only argument parsing and I/O in the CLI. Avoids a painful split later.

Suggested layout:

```
mt2data/
  cmd/
    mt2data/         # thin CLI entry point: flags, I/O, exit codes
      main.go
  internal/          # or a public package if external use is anticipated
    parse/           # stage 1: Markdown → block tree
    hierarchy/       # stage 2: clause/section tree recovery
    classify/        # stage 3: block classification (heuristics + LLM fallback)
    tables/          # stage 4: table normalization
    requirements/    # stage 5: imperative extraction from prose
    refs/            # stage 6: reference resolution
    merge/           # stage 7: deduplication and merging
    schema/          # output types, JSON Schema generation, validation
    provenance/      # source span types, helpers
    issues/          # issue types and reporter
  testdata/          # sample RFQ and norm Markdown files, expected outputs
  docs/
    design/          # this document and the larger pipeline doc
```

Each stage reads and writes typed IR. Stages are independently testable.

---

## Instructions for Claude Code

This document is meant to be dropped into the `mt2data` repository (e.g. `docs/design/mt2data-redesign.md`) and used as context for a Claude Code session.

### Recommended setup before starting

1. Place this document at `docs/design/mt2data-redesign.md`.
2. Place the larger pipeline document (`from-requirements-to-models-and-code.md`) at `docs/design/from-requirements-to-models-and-code.md` so the broader context is available.
3. Create or update `CLAUDE.md` at the repo root with a short orientation:
   - What `mt2data` does today (one paragraph).
   - What it is being refactored toward: staged library plus thin CLI, the output schema, the principle "deterministic by default, LLM as narrow fallback".
   - Pointers to the two design documents above.
   - Code conventions: Go, prefer clear over clever, no premature abstractions.
4. Make sure a few real input samples are checked into `testdata/` (one RFQ Markdown, one norm Markdown). The redesign is hard to validate without them.

### Suggested first task

Bounded, foundational, unblocks everything else:

> Define the output schema (`internal/schema/`) as Go types covering `document`, `clauses`, `parameters`, `requirements`, `trees`, `references`, and `issues` per `docs/design/mt2data-redesign.md`. Generate a JSON Schema from the Go types. Write a small validator command or test that loads current `mt2data` output and reports which fields are missing or non-conforming. Do not change extraction logic yet — only define and validate.

This produces a concrete artifact (the schema), a measurement (the gap against current output), and no risky changes to working code.

### Subsequent tasks, in order

1. **Provenance retrofit.** Add `source` (page, span, block ID) to every output record. Plumb it through whatever extraction code is kept from the current `mt2data`.
2. **Stage 1 — parse and segment.** Markdown → block tree with stable block IDs. New code, isolated from existing extraction.
3. **Stage 2 — hierarchy recovery.** Clause tree from heading levels. Attach blocks. Detect and flag malformed numbering.
4. **Stage 3 — block classification (heuristics only).** Implement the heuristic classifier. Defer the LLM fallback.
5. **Stage 4 — table normalizer.** Start with single-row headers, then multi-row, then condition propagation. Unit parser as a separate sub-package.
6. **Stage 5 — requirement extraction from prose.** Sentence segmentation plus imperative detection.
7. **Stage 6 — reference resolution.** Norm citations and intra-document refs.
8. **Stage 7 — merge and deduplicate.**
9. **Stage 8 — validation and issues report.**
10. **LLM fallback,** narrowly scoped per stage 3 / 4 needs, only after heuristics plateau and a measured baseline exists.

### Working principles for the session

- **One stage at a time.** Each stage is a separate package, with its own tests against `testdata/`. Resist cross-cutting refactors.
- **Tests before generalization.** Every new stage gets a golden-file test on at least one real sample before adding features.
- **Delete aggressively.** Existing code that doesn't fit the staged design should be removed, not adapted. The design assumes mostly-from-scratch.
- **Issues are output, not errors.** Anything ambiguous goes to the `issues` channel. The pipeline does not fail on ambiguity; it reports.
- **No LLM calls in early stages.** Build and ship the deterministic baseline first. LLM integration is a measured addition, not a starting point.
- **Provenance is non-negotiable.** Any new output record without a `source` field is a bug.

### Stopping points

After each stage lands: run the validator on real samples, inspect the `issues` output, commit, update `CLAUDE.md` with current status. Don't chain stages in one session — the schema and the early stages will reveal problems that change later assumptions.
