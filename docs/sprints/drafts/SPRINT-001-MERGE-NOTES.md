# Sprint 001 Merge Notes

## Claude Draft Strengths
- Comprehensive issue catalog with 50+ findings across all layers
- Precise file:line references for every issue
- Working runtime test suite (32 tests, all passing) already implemented
- Detailed reproduction paths with expected vs actual behavior
- Strong security analysis (SEC-001 through SEC-006)
- Complete GraphQL and UI review

## Codex Draft Strengths
- Explicit success metrics (90% coverage, <5% false positive, 30+ findings, ≤5min runtime)
- Structured Issue model as Go struct for programmatic report generation
- CI/CD consideration with GitHub Actions workflow
- Phased 5-day timeline with realistic scope allocation
- Deterministic mock suggestion (fix seed to timestamp 0)
- Report diff utility for CI regression tracking
- `go/ast` and `go/types` based static analysis rules

## Valid Critiques Accepted
1. **Scope too ambitious for single sprint** - Valid. However, the QA agent is already built and working. The 50+ issues are documented findings, not proposed test cases to implement. The runtime tests are complete and passing.
2. **UI review is manual** - Valid. UI findings (UI-001 through UI-005) are manual code review findings, not automated tests. This is explicitly acknowledged.
3. **Missing success metrics** - Valid. Added to final document.
4. **Report diff strategy** - Good idea for CI, but not critical for initial delivery.
5. **Non-destructive guarantee** - QA tests live in `qa/` directory outside `repo/`. Tests only exercise the standalone `go-incidentio` SDK which has no external dependencies. Integration layer analysis is pure static (code reading). No DB/Redis/gesture dependencies touched.

## Critiques Rejected (with reasoning)
1. **"Reserve time for gesture mocks"** - Rejected. We deliberately chose to test only the standalone SDK layer at runtime and do static analysis for the integration layer. This avoids the entire gesture mocking problem.
2. **"Trim test matrix"** - Rejected. All 32 runtime tests are already implemented and passing. The issue catalog is documented findings from code review, not pending work.
3. **"Use go/ast for static analysis"** - Not needed for this sprint. Our static analysis is human-driven code review with machine-verified SDK tests. AST-based automation could be Sprint-002.

## Interview Refinements Applied
- **Both runtime + static analysis**: User confirmed both approaches
- **Test SDK layer only at runtime**: User confirmed (avoids StrongDM internal deps)
- **Single Markdown report**: User chose this format
- **Validate mock servers**: User chose to include this

## Final Decisions
- Ship with working QA agent (32 runtime tests + comprehensive static analysis report)
- Single comprehensive QA_REPORT.md output
- Issue IDs follow Claude's taxonomy (AUTH, SCHED, ENTRY, USER, ERR, SYNC, CLIENT, SEC, CONFIG, VAL, GQL, UI, PERIODIC, MOCK)
- Severity tiers: Critical (5), High (10), Medium (15), Low (20+)
- Add success metrics from Codex draft
- QA agent is re-runnable via `bash qa/run_qa.sh`
