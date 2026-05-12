# From requirements to models and code

A design guide for LLM-assisted processing of RFQ documents into simulatable models and executable code.

## Purpose and scope

RFQ (request for quotation) documents in our context are product requirements documents: a dense and noisy mix of requirements, features, product structure, and referenced norms. The customer expects a quotation and a per-requirement compliance assessment, fast and accurate.

This document describes an approach to use LLMs as part of a pipeline that turns these documents into:

- A faithful textual representation of the source (Markdown / TOON).
- Cleaned, structured data extracted from that representation.
- Simulatable models of the product and its test context.
- Executable code (e.g. state machines) for behaviors described in the requirements.
- A per-requirement gap and compliance report.

The pipeline is partly built (`pdf2mt`, `mt2data`). This document focuses on the parts that are not yet built: model construction and code generation from requirements.

## Guiding principle: LLM produces IR, deterministic tools produce artifacts

The LLM never writes a SPICE netlist, never writes Go code, never writes a KiCad schematic. The LLM produces a structured **intermediate representation** (IR). Deterministic, non-LLM tools transform the IR into the final artifact.

This separation matters because it isolates three concerns that current LLMs conflate:

- **Understanding the requirement** — LLMs are good at this.
- **Knowing the right topology / algorithm** — LLMs are unreliable at this.
- **Emitting valid syntax** — LLMs are okay but error-prone, and errors here are the most expensive to debug.

By externalizing topology knowledge into a curated library and externalizing syntax into a deterministic assembler, the LLM is asked to do only what it does well: classify, map, extract, and parameterize.

Consequences of this separation:

- LLM outputs are schema-validated. Schema violation → reject and retry once → fail loud.
- LLM calls are pure functions of (input, prompt version, library catalog version). Cacheable by hash.
- Reviews happen on IR, not on netlists or generated code. IR is short, named, typed, and traceable to source.
- The library and the IR schema are the artifacts that compound across projects. Prompts are cheap and replaceable.

## Pipeline overview

```
PDF ──pdf2mt──▶ Markdown/TOON ──mt2data──▶ structured data
                                                │
                              ┌─────────────────┼─────────────────┐
                              ▼                 ▼                 ▼
                       compliance map    behavioral extract   product structure
                          (LLM)               (LLM)               (LLM)
                              │                 │                 │
                              └─────────────────┼─────────────────┘
                                                ▼
                                              IR
                                                │
                              ┌─────────────────┼─────────────────┐
                              ▼                 ▼                 ▼
                          assembler         codegen           gap report
                       (deterministic)   (deterministic)   (deterministic)
                              │                 │                 │
                              ▼                 ▼                 ▼
                       SPICE / PySpice       Go / FSM          Markdown
```

Stages 1 and 2 (`pdf2mt`, `mt2data`) are existing and lossless: there is a ground truth, and outputs can be verified against the source.

Stages 3 onward are the focus of this document. They are where invention happens, and where the LLM/IR separation pays off.

## Stage 3a: compliance tests as compositions

### Idea

Most RFQ requirements are not directly simulatable. "Shall comply with ISO 16750-2 pulse 2a" is a *test setup plus pass criterion*, not a circuit. The model has to include the test harness.

Treat the model as a collection of **testbenches**, one per requirement (or a few per requirement). Each testbench is a composition of four kinds of building block:

- **Preconditions** — initial state of the DUT (powered, in nominal mode, at ambient temperature).
- **Stimuli** — what is applied to the DUT (a surge pulse, a load step, a CAN frame).
- **Loads / environment** — what the DUT drives or sits in (resistive load, supply harness, thermal boundary).
- **Criteria** — how the DUT is observed and what counts as pass (functional status A/B/C/D, voltage stays within band, no reset).

Each block is drawn from a library and instantiated with parameters. The composition wires them together against the DUT's typed ports.

### Library organization

The library is organized by **what the thing is**, not by which requirement uses it. Norms are the natural unit of reuse.

```
libraries/
  stimuli/
    iso16750_2/        # one file per norm + edition
      pulse_1.lib
      pulse_2a.lib
      pulse_5b.lib
    iec61000_4_5/
      surge_combination_wave.lib
    iso7637_2/
      ...
  loads/
    resistive.lib
    motor_dc.lib
    lamp_inrush.lib
  environment/
    supply_network_12v.lib
    harness_short.lib
  criteria/
    iso16750_1/
      status_a.lib
      status_c.lib
```

Each library entry has machine-readable metadata: what it models, parameter schema (types, ranges, defaults), port types, what it does *not* cover, norm version. The LLM sees only this metadata catalog. It never sees the implementation.

This means template implementations can be swapped (ngspice → Xyce, behavioral → transistor-level) without touching the LLM layer.

### IR schema for compliance tests

```yaml
test:
  requirement_id: REQ-042
  source_span: "section 4.3.2, sentence 1-3"
  dut: <ref to product structure>
  preconditions:
    - template: criteria/iso16750_1/operating_mode_nominal
      bind: {target: dut}
  stimuli:
    - id: stim1
      template: iso16750_2/pulse_2a
      version: "2012"
      params: {Us: 50, Ri: 2}
      bind: {output: dut.vbat}
  loads:
    - id: load1
      template: loads/resistive
      params: {R: 2.4}
      bind: {terminals: [dut.vout, gnd]}
  criteria:
    - id: crit1
      template: criteria/iso16750_1/status_A
      bind: {observe: dut.vout_logic}
  duration: 500ms
```

Three things to notice:

- Every test points back to a `requirement_id` and `source_span`. Traceability is built in.
- `version: "2012"` on the stimulus pins the norm edition. When the norm is updated, you add a new template version, re-run the mapper, and the IR diff shows exactly which tests changed.
- Bindings reference DUT ports by name. Port type compatibility is checked by the assembler, not the LLM.

### What the mapper LLM does

For each classified requirement, the mapper is asked three smaller questions instead of one big one:

1. Which norm and which pulse / test? → pick stimulus template.
2. What load condition does the requirement imply? → pick load template.
3. What pass criterion applies? → pick criterion template.

Each decision is against a smaller catalog and is independently reviewable. If a decision can't be made, the IR records it as `unmapped` with a reason — no silent fallback.

### Catalog scaling

N stimuli + M loads + K criteria yields N+M+K templates, not N×M×K composites. A new automotive RFQ that uses ISO 16750-2 reuses the entire stimulus library for free. Customer-specific complexity lives in DUT structure and load templates.

## Stage 3b: behavioral models from legal jargon

### The problem

Customers sometimes describe a state machine in legal/contractual prose. The result is under-specified: transitions, defaults, and edge cases the author considered "obvious" are simply omitted. There is no oracle that tells you whether the extracted machine is correct.

### Goal shift

Instead of "produce the correct state machine," the goal is "produce a state machine **plus** an explicit list of every assumption and gap, traceable to source text." The LLM's job is to make the implicit explicit, not to guess well.

### Two-pass extraction

Single-pass extraction is unreliable because the LLM invents state names and event names that *sound* plausible. Two passes fix this.

**Pass 1 — vocabulary extraction.** Extract only the nouns: list of states, list of events, list of variables, list of timing constants. No transitions. Each entry carries a source span. LLMs are reliable at this.

A human review happens here, before any logic is inferred. Ambiguities like "the spec calls it ARMED in section 3.1 and ENABLED in section 3.4 — same state?" are caught cheaply at this stage. Catching them after a 40-transition machine has been built is not cheap.

**Pass 2 — transition extraction.** With the approved vocabulary as a hard constraint, extract transitions. The LLM can only emit transitions whose `from`, `to`, `trigger`, and variables come from the approved vocabulary. Anything that does not fit becomes a flagged gap, not a silent invention.

This constraint is the main lever for reliability.

### IR schema for state machines

```yaml
state_machine:
  requirement_id: REQ-073
  source_span: "section 5.1"
  states: [IDLE, ARMED, ACTIVE, FAULT]
  events: [power_on, button_press, timeout_5s, over_temp]
  variables:
    - {name: supply_voltage, type: float, unit: V}
  initial_state: IDLE
  transitions:
    - from: IDLE
      to: ARMED
      trigger: button_press
      guard: supply_voltage > 10
      action: start_timer(5s)
      source_span: "section 5.1, sentence 2"
      confidence: high
    - ...
  gaps:
    - kind: underspecified
      where: "(ACTIVE, over_temp)"
      note: "spec does not say what happens"
    - kind: conflict
      where: "(ARMED, button_press)"
      note: "section 5.1 says transition to ACTIVE, section 5.4 says transition to IDLE"
    - kind: untestable
      where: "section 5.2, sentence 1"
      note: "'shall respond promptly' — not quantified"
```

### Gaps are first-class deliverables

The gap list is not an error log, it is the **primary output for customer dialogue**. Categories:

- **Underspecified** — what happens on (state, event) is not in the spec.
- **Conflict** — two passages imply different targets for the same (state, event).
- **Dangling** — an event is mentioned but never triggered, or never handled.
- **Untestable** — phrases that have no quantitative meaning ("promptly", "as needed").
- **Implicit default** — assumptions the LLM had to make to produce a runnable machine.

The gap list, with paragraph references, becomes a structured set of clarifying questions to send the customer alongside the quotation. This is faster and more accurate than the alternative (guess, build, demo, "no, not like that").

### Validation without a ground truth

Several proxies, none requiring an LLM:

- **Source coverage.** Every imperative ("shall", "must", "when", "if") in the requirement section maps to at least one IR element, or appears on the gap list as "not modeled". A simple script flags unmapped imperatives. This catches silent dropping of requirements.
- **Reachability analysis.** Graph checks on the IR: unreachable states, dead-end states, events never consumed, guards that can never be true.
- **Round-trip paraphrase.** Regenerate prose from the IR and compare to source. Useful for human spot-checks; a second LLM can do a first pass.
- **Property checks.** Customer-stated invariants ("never enter ACTIVE without first passing through ARMED") are encoded as temporal logic and model-checked against the IR with NuSMV / SPIN. These tools handle small FSMs instantly.

### Code generation

From the approved IR, generation is a deterministic template substitution. No LLM involvement.

Outputs from the same IR:

- Idiomatic Go for the firmware team (switch on state, switch on event, call action).
- Graphviz / PlantUML diagram for the compliance document.
- Test harness with one test per transition.
- Markdown traceability table mapping every transition back to its source span.

The traceability table is the artifact most likely to be read by the customer.

## Stage 3c: product structure

The product itself is modeled as a hierarchical block diagram with typed ports, derived from the RFQ's product structure section. The LLM extracts blocks and interconnections; a reviewer confirms.

Each block initially is a black box with declared port types and parametric limits taken from the requirements. Simulation-ready implementations are filled in later, manually or from previous projects.

The product structure provides the DUT references that compliance tests bind to.

## Stage 4: deterministic assembly and codegen

The assembler is a non-LLM tool that:

- Reads the IR.
- Resolves template references against the library.
- Type-checks all bindings (port types, parameter ranges).
- Emits the final artifact: ngspice deck, PySpice script, KiCad schematic for documentation, Go source for FSMs, etc.
- Reports any error loudly with location info — never produces a partial or "best effort" artifact.

This is plain code generation from a typed IR. Boring, reliable, testable.

## Stage 5: gap and compliance report

Per-requirement table:

| Requirement | Classified | Mapped to IR | Simulatable | Result |
|---|---|---|---|---|
| REQ-042 | ✓ | ✓ | ✓ | pass |
| REQ-073 | ✓ | ✓ (FSM) | ✓ | pass + 3 open questions |
| REQ-091 | ✓ | ✗ | n/a | unmapped — no template for "EMC radiated emissions per CISPR 25 class 5" |
| REQ-104 | ✗ | n/a | n/a | unclassifiable — text refers to a deleted section |

The "unmapped" and "unclassifiable" buckets are exactly what the engineer needs to focus on. They also feed back into the library (add the missing template) and into the customer dialogue (request clarification).

## Top-level IR shape

```yaml
rfq:
  metadata:
    customer: ...
    rfq_id: ...
    source_documents: [...]
  product_structure: <block diagram>
  compliance_tests:
    - <test composition>
    - ...
  behavioral_models:
    - <state machine>
    - ...
  gaps:
    - <gap entries from all stages>
```

Behavioral models can be referenced by compliance tests (`while in ACTIVE state, apply pulse 2a`), which closes the loop between the analog and digital sides.

## Construction-time only — no LLM at simulation time

The LLM is involved only in IR construction. Once the IR is approved, runs are deterministic, reproducible, diffable, and CI-able.

Implications:

- Failures are construction failures, surfaced in the gap report. No half-broken simulations.
- Caching is straightforward: LLM calls are pure functions of (input, prompt version, catalog version).
- Re-running an RFQ after a library update only re-invokes the LLM for items whose mapping might have changed.
- Compliance audits are answerable: "why did you say compliant?" → point to the IR, the template version, the LLM output that produced it.

A runtime advisor (LLM proposing fixes when a sim fails) can be added later. Inverting that order is painful.

## What to build first

Resist generalization until one narrow loop closes end-to-end.

**For compliance tests:** ISO 16750-2. Small number of pulses, well-defined, automotive RFQs use it constantly.

1. Stimulus library for ISO 16750-2 (all pulses, parameterized, versioned).
2. Two or three load templates.
3. Two or three criterion templates (status A, status C).
4. IR schema with composition.
5. ngspice assembler.
6. Mapper prompt that produces compositions.
7. End-to-end run on five real requirements from a past RFQ.

**For behavioral models:** one real customer state machine, in legalese, end to end.

1. Vocabulary extractor.
2. Transition extractor with vocabulary constraint.
3. Gap report generator.
4. Go codegen.
5. Graphviz codegen.
6. Traceability table.

Flat FSMs only at first. No hierarchical states, parallel regions, or history pseudostates.

## Open questions to decide before generalizing

- **Composition expressiveness.** The four-slot model (preconditions / stimuli / loads / criteria) covers automotive transient compliance well. Does it cover EMC, mechanical, environmental tests? Likely needs at least an extension for swept stimuli (frequency sweeps for radiated immunity).
- **Cosimulation.** Should behavioral FSMs run inside the same simulation as the SPICE testbenches (Verilog-A, Python driving ngspice), or only as standalone Go for firmware? Same IR either way; very different assembler.
- **IR storage and versioning.** Git-friendly YAML is the obvious choice. Decide early on stable IDs (requirement IDs, template IDs) so diffs across RFQ revisions are meaningful.
- **Catalog metadata language.** Plain YAML with a JSON schema is enough to start. Avoid building a DSL until pain demands it.

## Summary

- LLMs classify, map, and extract. Deterministic tools assemble and generate.
- The IR is the contract. Schema-validated, reviewable, traceable, versioned.
- Libraries are organized by norm and by physical role, not by requirement. They compound across projects.
- Compliance tests are compositions of preconditions, stimuli, loads, and criteria.
- Behavioral models are extracted in two passes with a frozen vocabulary, and gaps are a first-class output.
- Code generation is deterministic template substitution — boring on purpose.
- Build one narrow vertical slice end-to-end before generalizing.
