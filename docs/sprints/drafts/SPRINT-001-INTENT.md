# Sprint 001 Intent: QA Validation Agent for incident.io Integration

## Seed

Build a QA agent that validates the incident.io capability. This agent can read the code but cannot make any changes to the code base. No cheating allowed and should thoroughly test the capability and find all the gaps and issues. All found issues should be detailed out with the reproduction path along with all the errors encountered and any logs. Make sure the tests continue to run. Again this validation QA agent cannot make any changes to code.

Repo: https://github.com/ashkavakil/ai-sdm-incident.io.git

## Context

### Orientation Summary
- **Project State**: The incident.io integration is a three-layer system (Go SDK, GraphQL API, React Admin UI) for syncing on-call schedules from incident.io into StrongDM groups. The codebase is functionally complete but has zero test coverage and significant gaps in error handling and edge-case protection.
- **Architecture**: Go backend with standalone SDK (`go-incidentio/`), integration layer (`pkg/incidentio/`), GraphQL resolvers (`pkg/graphql/`), and React admin UI (`adminui/`). A demo with mock servers exists in `demo/`.
- **Key Concern**: No existing tests, no CLAUDE.md, no sprint history. This is a greenfield QA effort.
- **Critical Systems**: Sync logic (`sync.go`), user resolution (EMAIL vs IDENTITY_SET modes), API key caching, periodic sync, and GraphQL mutation/query handlers.
- **Demo Infrastructure**: Two mock servers (basic: 4 schedules/4 users, rich: 156 schedules/60 users) available for testing without real API access.

## Recent Sprint Context

No prior sprints exist. This is Sprint 001 - the first structured development effort on this codebase.

## Relevant Codebase Areas

### Go Backend SDK (`go-incidentio/`)
| File | Purpose |
|------|---------|
| `client.go` | HTTP client, auth, request execution |
| `incidentio.go` | SDK types, version constants |
| `schedule.go` | Schedule list/get with pagination |
| `user.go` | User list/get with pagination |
| `schedule_entry.go` | On-call entries within time windows |
| `errors.go` | APIError types, helper methods |

### Integration Layer (`pkg/incidentio/`)
| File | Purpose |
|------|---------|
| `client.go` | Rate-limited SDK wrapper |
| `sync.go` | Full sync orchestration (critical) |
| `config.go` | Integration config CRUD |
| `validation.go` | Input validation |
| `apikey.go` | API key validation & caching |
| `token.go` | Token cache implementation |
| `constants.go` | User agent string |
| `periodic/periodic.go` | Scheduled sync job |

### Models & GraphQL (`pkg/`)
| File | Purpose |
|------|---------|
| `models/incidentio_types.go` | Data types, sync modes |
| `graphql/graph/incidentio.resolvers.go` | GraphQL mutations/queries |
| `graphql/graph/incidentio.helpers.go` | Model conversion helpers |
| `graphql/graph/incidentio.graphqls` | GraphQL schema |
| `tags/incidentio.go` | Filter tag parsing |

### Admin UI (`adminui/src/features/incidentIo/`)
| File | Purpose |
|------|---------|
| `CreateIncidentIoIntegrationDrawer.tsx` | Integration creation flow |
| `IncidentIoIntegrationFields.tsx` | Reusable form fields |
| `IncidentIoIntegrationDetailsPage/` | Details page with tabs |
| `graphql/queries.graphql` | GraphQL queries |
| `graphql/mutations.graphql` | GraphQL mutations |

### Demo (`demo/`)
| File | Purpose |
|------|---------|
| `main.go` | Demo entry point |
| `mock_server.go` | Basic mock (4 schedules, 4 users) |
| `rich_mock_server.go` | Rich mock (156 schedules, 60 users) |

## Constraints

- **Read-only**: The QA agent MUST NOT modify any source code in the repo
- **Non-destructive**: Tests must be runnable repeatedly without side effects
- **Comprehensive**: Must cover all layers (SDK, sync, config, GraphQL, UI patterns)
- **Reproducible**: Every issue must include exact reproduction steps
- **Evidence-based**: Issues must include actual errors, logs, or code references
- **Continuous**: Tests should be designed to run continuously/repeatedly

## Success Criteria

1. A working QA validation agent that can be invoked to audit the incident.io codebase
2. Comprehensive issue catalog covering all severity tiers (Critical/High/Medium/Low)
3. Each issue includes: description, severity, affected files/lines, reproduction path, expected vs actual behavior
4. Test scripts that exercise the demo mock servers and validate behavior
5. Static analysis findings covering error handling, security, data consistency
6. Architecture review covering design gaps and missing patterns
7. All validation is read-only - zero code modifications

## Verification Strategy

- **Static Analysis**: Read and analyze all source files for patterns, anti-patterns, and gaps
- **Mock Server Testing**: Run the demo mock servers and validate SDK behavior against them
- **Code Path Analysis**: Trace every code path through sync logic for error handling completeness
- **API Contract Verification**: Validate that SDK assumptions match incident.io API documentation
- **Security Audit**: Review authentication, authorization, secret handling patterns
- **UI Pattern Review**: Analyze React components for missing states, error handling, accessibility
- **Differential Analysis**: Compare EMAIL vs IDENTITY_SET sync modes for consistency

## Uncertainty Assessment

- **Correctness uncertainty**: Medium - No tests exist; must validate logic by code reading and mock testing
- **Scope uncertainty**: Low - The codebase is bounded and fully contained in the repo
- **Architecture uncertainty**: Low - Architecture is clear from code exploration

## Open Questions

1. Should the QA agent produce a single comprehensive report or categorized reports per layer?
2. What severity threshold matters most - should we focus on critical/high issues or be exhaustive?
3. Should the QA agent also validate the demo mock servers for correctness against the incident.io API spec?
4. Is there a specific incident.io API version or documentation to validate against?
5. Should the QA agent attempt to compile and run the Go code, or is pure static analysis sufficient?
