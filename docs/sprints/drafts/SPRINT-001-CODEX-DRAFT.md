# Sprint 001 – Codex Draft

## Objective

Design and bootstrap a **read-only, repeatable QA validation agent** that audits the complete incident.io integration (Go SDK → Sync Engine → GraphQL → React UI) and produces a detailed, evidence-backed issue report without modifying production source code.
# Sprint 001 – Codex Draft

## Objective

Design and bootstrap a **read-only, repeatable QA validation agent** that audits the complete incident.io integration (Go SDK → Sync Engine → GraphQL → React UI) and produces a detailed, evidence-backed issue report without modifying production source code.

## Guiding Principles

- **Non-destructive** – never write to the repo or external systems.
- **Evidence first** – every finding references code, logs, or reproducible steps.
- **Layer-aware** – test each layer individually **and** end-to-end.
- **Continuous** – runnable in CI on every commit/PR.
- **Human-readable** – Markdown report grouped by severity and category.

## Deliverables

- `qa/` directory containing agent source, tests and helpers (outside `repo/`).
- Go test suite that launches mock servers, exercises SDK & sync engine, and performs static analysis.
- Static-analysis helpers using `go/ast`, `go/types`, and regex heuristics.
- Markdown issue report `qa/report/incidentio-qa-<date>.md`.
- (Stretch) GitHub Actions workflow `.github/workflows/qa.yml`.

## Architecture Overview

```
          +----------------------+
          |   qa/validation.go   |
          +----+-----------+-----+
               |           |
        +------+----+  +---+------+
        | Static    |  | Runtime  |
        | Analysis  |  | Testing  |
        +--+--------+  +--+-------+
           |              |
   +-------v------+  +----v-------+
   | Rule Pack    |  | Mock Svrs  |
   +-------+------+  +----+-------+
           \            /
            \   +------+-------+
             +->|  Issue Model |
                +------+-------+
                       |
                +------+-------+
                | Markdown Out |
                +--------------+
```

## Timeline & Milestones (5-day sprint)

### Day 0-0.5 – Environment

- Create `qa/go.mod` aligned with root Go version.
- Vendor `golang.org/x/tools/go/packages` for analysis.

### Day 0.5-1.5 – Runtime Harness

1. `qa/mock/launcher.go` – start `demo/mock_server.go` and `demo/rich_mock_server.go` on random free ports.
2. `qa/sdk_runtime_test.go` – table-driven tests for all SDK endpoints.
3. Helper assertions for HTTP status, pagination and JSON decoding.

### Day 1.5-3 – Sync Engine Tests

1. In-memory SQLite + stubbed gesture interfaces.
2. Drive `SyncManager.FullSync()` with controlled incident.io client fakes.
3. Validate group lifecycle, member sync, error branches and mode parity.

### Day 3-4 – Static Analysis

1. Build rule pack (`qa/static/rules.go`):
   - Errors logged but not returned.
   - Ignored `json.Unmarshal` errors.
   - Panic without recover or return.
   - TODO/FIXME comments.
2. Parse `repo/...` via `packages.Load` (NeedTypes, NeedSyntax).
3. Emit issues into shared model.

### Day 4-4.5 – Report Generator

1. Sort issues Severity ▸ Layer ▸ ID.
2. Render Markdown summary & detail sections.
3. Save to `qa/report/<timestamp>.md` (git-ignored).

### Day 4.5-5 – Developer Experience

- README in `qa/` with quick-start instructions.
- Makefile targets `qa` and `qa-watch`.
- (Stretch) GitHub Actions job matrix (fast vs slow suites).

## Severity & Category Matrix

| Severity | Definition | Icon |
|----------|------------|------|
| Critical | Data loss / security exploit / test crash | 🔴 |
| High     | Incorrect sync behavior / auth failure    | 🟠 |
| Medium   | Partial mismatch, degraded UX            | 🟡 |
| Low      | Cosmetic, logging, docs                  | ⚪️ |

Categories: **SDK**, **Sync**, **GraphQL**, **UI**, **Security**, **Infra**.

## Issue Struct (`qa/issue.go`)

```go
type Issue struct {
    ID       string   // e.g. SYNC-006
    Severity Severity
    Layer    string   // "Sync"
    Title    string
    Location string   // file:line or component
    Repro    string   // step-by-step
    Expected string
    Actual   string
    Evidence []string // logs, errors, code snippets
}
```

## Risks & Mitigations

- **Parsing performance** – cache `packages.Load` results.
- **Flaky ports** – choose free port via `net.Listen` then reuse.
- **CI runtime** – split fast (SDK) and slow (sync) test jobs.

## Success Metrics

- ≥90 % SDK endpoint coverage across both mocks.
- <5 % false-positive rate on static rules.
- ≥30 unique findings spanning all layers.
- Total runtime ≤5 min on medium GitHub runner.

## Stretch Goals

- Visual regression of React states via Storybook snapshot.
- Fuzzing SDK JSON decoders.
- Mutation testing for sync rules.

---

_Author: Codex QA squad – 2026-02-18_

