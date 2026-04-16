# mt2data

`mt2data` is a command-line tool written in Go. It takes a Markdown+TOON (MT) document produced
by `pdf2mt` and extracts a structured requirements table from it, with the aid of the Claude API.

## Context: the requirements analysis workflow

`mt2data` is step (2) in a larger workflow:

1. **pdf2mt** — converts a PDF requirements document to MT format (1:1 fidelity, LLM-optimized).
2. **mt2data** (this tool) — extracts a requirements table from the MT document.
3. **Synthesis** — human-led distillation of the requirements into a canonical, non-redundant set.
4. **Inconsistency detection** — pairwise comparison of requirements within domain clusters.

The MT document produced by `pdf2mt` is **read-only** after step (1). All downstream work is keyed
to requirement IDs produced in step (2), not to raw clause references.

## Input format

MT documents are Markdown files with TOON tables embedded. The input is a single MT file, typically
a converted automotive or systems engineering norm (e.g. ISO 26262, ASPICE, OEM requirement catalogue)
or a customers requirements document for a specific product. The MT format is specified in the pdf2mt
project (../pdf2mt).

There are no clause anchors in the input file. Requirements are not explicitly marked; they must be
inferred by the presence of modal verbs (MUST, SHALL, SHOULD, MAY, MUST NOT, SHALL NOT, SHOULD NOT)
or equivalent normative language in the text.

Each norm version is treated as a fixed input. Version tracking is a document management concern,
not a requirements analysis concern.

## Output: requirements table

The output is a Markdown table (or TOON table) with one row per requirement. The table is an
**index into the MT document** — not a replacement for it.

### Columns

| Column           | Description |
|------------------|-------------|
| `title`          | Summary of the requirement (generated). This is the sole description of the requirement in the table. |
| `id`             | Unique requirement ID, assigned sequentially within the document. Format: integer, starting at 1. |
| `section`        | Section number in the source MT document (e.g. `4.3.2.1`), inferred from Markdown headings. |
| `item`           | The element that is the object or target of the requirement, extracted from the text (e.g. `vehicle`, `electronic module`). |
| `domain`         | One or more of: `system`, `hardware`, `software`, `test`. |
| `verb`           | Modal verb: `MUST`, `SHOULD`, `MAY`, `MUST NOT`, `SHOULD NOT`. |
| `verification`   | Implied or stated verification method: `T` (test), `A` (analysis), `I` (inspection), `D` (demonstration). |
| `compound`       | `yes` if this row was split from a compound requirement; `no` otherwise. |

### Domain taxonomy

- `system` — not yet allocated to hardware or software; or a cross-cutting constraint.
- `hardware` — allocated to or constrained by hardware implementation.
- `software` — allocated to or constrained by software implementation.
- `test` — a requirement on the test process, test coverage, or test evidence.

When a requirement spans multiple domains, list all that apply (comma-separated).

### Verb normalization

The source document may use MUST, SHOULD, MUST NOT, SHOULD NOT, MAY, SHALL, SHALL NOT or domain-specific
variants. Normalize these verbs to: MUST (equivalent to SHALL, REQUIRED), SHOULD (equivalent to RECOMMENDED),
MUST NOT (equivalent to SHALL NOT), SHOULD NOT, or MAY (equivalent to OPTIONAL), in accordance with
RFC2119 (https://www.rfc-editor.org/rfc/rfc2119.txt).

### Atomicity

Compound requirements are split into atomic requirements. Each atomic requirement gets its own row
with a unique sequential `id` and inherits the `section` value of the originating compound requirement.

## Architecture

- Input: single MT file (path argument).
- LLM call: Claude API (`claude-sonnet-4-20250514`), one call per clause block or per page chunk
  (TBD based on context window experiments).
- Output: Markdown table written to stdout or to a file (flag).
- No database, no persistent state. The tool is stateless and idempotent.

Implementation language: Go.

## Prompt design principles

- The extraction prompt must supply the column schema and domain taxonomy explicitly.
- The verb normalization rules (per RFC2119) must be included in the prompt.
- The LLM is instructed to output JSON (one object per requirement), not Markdown, to make
  parsing deterministic. The Go layer assembles the final Markdown table.
- The prompt must instruct the LLM to split compound requirements into atomic requirements, each as a separate JSON object, all sharing the same `section` value.
- The `title` field must be a self-contained summary of the requirement, including any condition ("when X" / "if Y") scope.

## Relation to pdf2mt

- `pdf2mt` repo: https://github.com/rveen/pdf2mt (locally ../pdf2mt)
- MT format is defined there.
- `mt2data` does not depend on `pdf2mt` at build time; it only consumes its output format.
