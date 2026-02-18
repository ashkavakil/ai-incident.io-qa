# Sprint 001: QA Validation Agent for incident.io Integration

## Overview

The incident.io integration is a three-layer system (Go SDK, sync engine, React admin UI) that syncs on-call schedules from incident.io into StrongDM groups. Despite being functionally complete, it has **zero test coverage** and numerous gaps in error handling, edge case protection, and resilience. This sprint builds a read-only QA validation agent that thoroughly audits the codebase without modifying any source files.

The QA agent operates in two modes: (1) **static analysis** - reading all source files and tracing code paths to find logic errors, missing validation, security issues, and design gaps; (2) **runtime validation** - launching the demo mock servers and testing the Go SDK client against them to verify API contract compliance, pagination behavior, error handling, and data consistency.

The agent produces a comprehensive, categorized issue report with exact file:line references, reproduction paths, severity ratings, and expected vs. actual behavior for every finding.

## Use Cases

1. **SDK API Contract Validation**: Test the `go-incidentio` SDK client against mock servers to verify correct HTTP request construction, authentication headers, pagination handling, error parsing, and JSON deserialization.

2. **Sync Logic Audit**: Trace every code path through `sync.go` to identify: silent error swallowing, race conditions, partial failure states, empty membership risks, and data consistency gaps.

3. **Configuration & Validation Review**: Verify that `config.go` and `validation.go` properly handle all edge cases: sync mode transitions, API key format validation, identity set lifecycle, and integration name uniqueness.

4. **GraphQL Resolver Analysis**: Audit all mutations/queries for: missing authorization checks, batch atomicity issues, async sync failure visibility, pagination correctness, and subscription reliability.

5. **Security Assessment**: Review API key storage, token caching, email-based user matching trust model, and log output for sensitive data exposure.

6. **UI Pattern Review**: Analyze React components for missing error states, loading indicators, sync status visibility, and form validation completeness.

## Architecture

```
                    QA Validation Agent
                    ==================
                           |
            +--------------+--------------+
            |                             |
    Static Analysis Engine         Runtime Test Engine
            |                             |
    +-------+-------+             +-------+-------+
    |       |       |             |       |       |
  Code    Error   Security     Mock     SDK     API
  Path    Audit   Scan       Server   Client  Contract
  Trace                     Launch    Tests    Tests
    |       |       |             |       |       |
    +-------+-------+             +-------+-------+
            |                             |
            +--------------+--------------+
                           |
                  Issue Report Generator
                  =====================
                           |
              +------------+------------+
              |            |            |
          CRITICAL      HIGH       MEDIUM/LOW
          Issues       Issues       Issues
              |            |            |
              +---- Each Issue --------+
              | - Description          |
              | - Severity             |
              | - File:Line            |
              | - Reproduction Path    |
              | - Expected Behavior    |
              | - Actual Behavior      |
              | - Error/Log Output     |
              +------------------------+
```

## Implementation Plan

### Phase 1: Test Infrastructure (~20%)

Build the QA agent's test harness using Go test files that live in a separate `qa/` directory (not in the repo). These tests use the existing mock servers for runtime validation and read source files for static analysis.

**Files:**
- `qa/go.mod` - Go module for QA tests (depends on `go-incidentio` SDK)
- `qa/sdk_test.go` - SDK client tests against mock servers
- `qa/static_analysis.go` - Code path analysis utilities
- `qa/report.go` - Issue report generation
- `qa/run_qa.sh` - Main entry point script

**Tasks:**
- [ ] Create `qa/` directory with Go module referencing `go-incidentio`
- [ ] Write mock server launcher that starts both basic and rich mocks
- [ ] Build issue report data structures (Issue, Severity, Category)
- [ ] Create report formatter (Markdown output)

### Phase 2: SDK Runtime Tests (~25%)

Test the Go SDK client against both mock servers to validate all API operations.

**Files:**
- `qa/sdk_test.go` - Comprehensive SDK tests

**Test Cases:**
- [ ] **AUTH-001**: Valid API key authentication succeeds
- [ ] **AUTH-002**: Invalid API key returns 401 with proper error type
- [ ] **AUTH-003**: Empty API key handling
- [ ] **AUTH-004**: Malformed bearer token handling
- [ ] **SCHED-001**: List all schedules (basic mock: 4 expected)
- [ ] **SCHED-002**: List all schedules with pagination (rich mock: 156 across multiple pages)
- [ ] **SCHED-003**: Get single schedule by ID
- [ ] **SCHED-004**: Get non-existent schedule returns 404
- [ ] **SCHED-005**: Pagination cursor-based traversal complete coverage
- [ ] **ENTRY-001**: List on-call entries for schedule with single user
- [ ] **ENTRY-002**: List on-call entries for schedule with multiple users
- [ ] **ENTRY-003**: List on-call entries for non-existent schedule
- [ ] **ENTRY-004**: Empty time window handling
- [ ] **ENTRY-005**: Time window boundary precision (1-minute window)
- [ ] **USER-001**: Get user by ID returns correct fields
- [ ] **USER-002**: Get non-existent user returns 404
- [ ] **USER-003**: List users with pagination
- [ ] **USER-004**: User email field population verification
- [ ] **ERR-001**: APIError parsing from JSON response
- [ ] **ERR-002**: IsNotFound(), IsUnauthorized(), IsRateLimited() helpers
- [ ] **ERR-003**: Malformed JSON error body handling
- [ ] **ERR-004**: Empty response body error handling

### Phase 3: Static Analysis - Sync Logic (~25%)

Deep code path analysis of the sync engine (`pkg/incidentio/sync.go`, `client.go`, `config.go`).

**Issues to Investigate:**

**Sync Orchestration (`sync.go:82-134` - FullSync):**
- [ ] **SYNC-001**: `refreshScheduleMetadata()` errors are logged but don't block sync. If the incident.io API is partially down, stale metadata persists while sync continues with potentially wrong group names.
  - File: `sync.go:93-97`
  - Reproduction: API returns error for GetSchedule but succeeds for ListOnCallForSchedule
  - Expected: Sync should either retry or propagate the error
  - Actual: Error logged, sync continues with stale names

- [ ] **SYNC-002**: `removeDeletedSchedules()` errors are logged but don't block sync. Deleted schedules accumulate in metadata.
  - File: `sync.go:99-103`
  - Reproduction: ListSchedules fails during removeDeletedSchedules
  - Expected: Error should block sync or be tracked
  - Actual: Error logged, stale schedules remain

- [ ] **SYNC-003**: `CleanupOrphanedGroups()` errors are logged but don't block sync. Orphaned groups accumulate.
  - File: `sync.go:109-113`

- [ ] **SYNC-004**: `updateLastSyncedAt()` errors are logged but don't block sync. LastSyncedAt is updated even if earlier steps had errors.
  - File: `sync.go:119-123`
  - Impact: UI shows "Last synced: just now" even when sync was partially broken
  - Actual: updateLastSyncedAt only runs if prior hard-error steps succeeded, but soft-error steps (metadata refresh, delete cleanup, orphan cleanup) may have failed silently

**Schedule Group Management (`sync.go:232-310` - SyncScheduleGroups):**
- [ ] **SYNC-005**: Group name update race condition. If an admin manually renames a group, sync will overwrite it on next run without warning.
  - File: `sync.go:265-270`
  - Reproduction: Admin renames group "Infra On-Call" to "Infra Critical". Schedule name unchanged. Next sync: no overwrite (good). Schedule name changes in incident.io: overwrite occurs (potentially bad).

- [ ] **SYNC-006**: Empty schedule name validation returns error, blocking entire sync for all schedules.
  - File: `sync.go:249-251`
  - Reproduction: One schedule has empty name in metadata
  - Expected: Skip that schedule, sync others
  - Actual: Returns error, aborts entire SyncScheduleGroups

- [ ] **SYNC-007**: Group recreation when linked group deleted. The code handles this (line 256-262), but doesn't log which admin deleted it or why, making audit difficult.

**On-Call User Sync (`sync.go:357-419` - syncScheduleMembers):**
- [ ] **SYNC-008**: `__oncall__` schedule calls `ListAllOnCallUsers()` which silently skips failed schedules.
  - File: `pkg/incidentio/client.go:158-163`
  - Reproduction: 1 of 156 schedules returns error
  - Expected: Error reported, on-call set may be incomplete
  - Actual: Users from failed schedule silently excluded

- [ ] **SYNC-009**: Empty on-call set syncs to group. If no users are on-call for a schedule, the group membership is cleared to zero members.
  - File: `sync.go:437-440`, `sync.go:449-510`
  - Reproduction: Schedule has no one on-call at current moment (e.g., gap between shifts)
  - Expected: Preserve previous members or flag warning
  - Actual: All members removed from group

- [ ] **SYNC-010**: Single schedule member sync failure aborts remaining schedules.
  - File: `sync.go:408-415`
  - Reproduction: User resolution fails for schedule 2 of 10
  - Expected: Continue syncing remaining schedules
  - Actual: Returns error, schedules 3-10 not synced

**User Resolution (`sync.go:512-711`):**
- [ ] **SYNC-011**: In EMAIL mode, individual user fetch failures skip the user silently.
  - File: `sync.go:668-674`
  - Impact: User may be on-call but not granted access

- [ ] **SYNC-012**: Cache write failures for email mappings are non-blocking (best-effort).
  - File: `sync.go:700-707`
  - Impact: Repeated API calls for same user on every sync cycle

- [ ] **SYNC-013**: In IDENTITY_SET mode, missing alias logs warning but doesn't track unmatched users.
  - File: `sync.go:567-572`

**Client Layer (`pkg/incidentio/client.go`):**
- [ ] **CLIENT-001**: `ListOnCallForSchedule` uses 1-minute time window (now to now+1min). If entries are exactly at boundary, may miss them.
  - File: `pkg/incidentio/client.go:112-117`

- [ ] **CLIENT-002**: `ListOnCallsForSchedules` fails entirely if any single schedule fails.
  - File: `pkg/incidentio/client.go:139-147`

- [ ] **CLIENT-003**: `ListAllOnCallUsers` silently continues on per-schedule errors.
  - File: `pkg/incidentio/client.go:158-163`
  - Inconsistency: `ListOnCallsForSchedules` fails-fast, but `ListAllOnCallUsers` continues-on-error. No documented rationale.

- [ ] **CLIENT-004**: Rate limiter burst of 600 with 20 req/sec limit. Burst of 600 allows 600 requests instantly before rate limiting kicks in. With multiple integrations sharing a 1200 req/min account limit, this could cause rate limiting.
  - File: `pkg/incidentio/client.go:33`

- [ ] **CLIENT-005**: No HTTP timeout configured on SDK client (`go-incidentio/client.go` uses `http.DefaultClient` which has no timeout).
  - File: `go-incidentio/client.go:42`

### Phase 4: Static Analysis - Config, Validation, Security (~15%)

**Configuration (`config.go`):**
- [ ] **CONFIG-001**: `diffIncidentIOConfig` clears IdentitySetID when switching from IDENTITY_SET to EMAIL (line 85) but doesn't validate that existing cached user mappings are still valid.
  - File: `config.go:82-96`

- [ ] **CONFIG-002**: `RemoveIncidentIOSchedule` runs `FullSync` synchronously after removing schedules. If sync fails, the schedule removal is already persisted but cleanup didn't happen.
  - File: `config.go:226-237`
  - Reproduction: Remove schedule, FullSync fails
  - Expected: Schedule removed AND groups cleaned up atomically
  - Actual: Schedule removed, orphaned group remains until next periodic sync

- [ ] **CONFIG-003**: `ValidateUniqueIntegrationName` is defined but never called during Update operations. Name change during update could create duplicates.
  - File: `config.go:241-260`, `config.go:107-143` (UpdateIntegrationConfig doesn't call it)

**Validation (`validation.go`):**
- [ ] **VAL-001**: No API key format validation. Any non-empty string accepted.
  - File: `validation.go:24-25`

- [ ] **VAL-002**: `validateMetadata` creates a new `identitysets.New()` gesture on every call. No caching or connection reuse.
  - File: `validation.go:80`

- [ ] **VAL-003**: Email sync mode rejects configs with a set IdentitySetID (`validation.go:69-71`), but `diffIncidentIOConfig` only clears it on mode *change*. If config was created with EMAIL mode and IdentitySetID somehow set, validation would fail on every sync.

**API Key Provider (`apikey.go`):**
- [ ] **SEC-001**: API key cached for 23 hours. A revoked key continues granting access for up to 23 hours.
  - File: `apikey.go:24`

- [ ] **SEC-002**: Key validation doesn't invalidate on auth failure. If `ValidateAPIKey` fails, the cache is not updated to prevent retries. Next call will re-validate.
  - File: `apikey.go:67-83` - Cache only set on success, not on explicit failure

- [ ] **SEC-003**: Global singleton `apiKeyCache` shared across all integrations. No isolation between organizations.
  - File: `apikey.go:27-28`

- [ ] **SEC-004**: API key stored in plaintext in memory (CachedToken.token field).
  - File: `token.go:13-14`

**SDK Client (`go-incidentio/client.go`):**
- [ ] **SEC-005**: No response body size limit. `io.ReadAll(resp.Body)` on line 100 could consume unbounded memory on malicious response.
  - File: `go-incidentio/client.go:100`

- [ ] **SEC-006**: Schedule/User IDs used directly in URL paths without encoding. If an ID contains special characters, URL construction could break.
  - File: `go-incidentio/schedule.go:64`, `go-incidentio/user.go:41`

### Phase 5: Static Analysis - GraphQL & UI (~10%)

**GraphQL Resolvers (`incidentio.resolvers.go`):**
- [ ] **GQL-001**: `TestIncidentIoIntegration` swallows error details. Returns `{success: false}` without explaining why the test failed.
  - File: `incidentio.resolvers.go:36-38`

- [ ] **GQL-002**: `CreateIncidentIoIntegrations` batch not atomic. If item 3 of 5 fails, items 1-2 are persisted but 4-5 are not processed.
  - File: `incidentio.resolvers.go:57-84`

- [ ] **GQL-003**: `SyncIncidentIoIntegrations` runs synchronously in the request handler. Large schedules could cause HTTP timeout.
  - File: `incidentio.resolvers.go:256-258`

- [ ] **GQL-004**: `AddIncidentIoSchedules` triggers async sync via `startIncidentIoSync` with 30-second timeout. No feedback to caller if sync fails.
  - File: `incidentio.resolvers.go:175`, `incidentio.helpers.go:20-33`

- [ ] **GQL-005**: `RemoveIncidentIoSchedules` doesn't trigger async sync. It calls `RemoveIncidentIOSchedule` in config.go which runs sync synchronously.
  - File: `incidentio.resolvers.go:208`, `config.go:226-237`
  - Inconsistency: Add uses async sync, Remove uses synchronous sync

- [ ] **GQL-006**: `IncidentIoSchedules` query fetches ALL schedules from incident.io API on every call, then paginates in-memory. With 156+ schedules, this is wasteful.
  - File: `incidentio.resolvers.go:394-396`

- [ ] **GQL-007**: Subscription resolver uses `context.WithoutCancel` but the goroutine has no cleanup mechanism if the parent context is long-cancelled.
  - File: `incidentio.resolvers.go:463-492`

- [ ] **GQL-008**: `IncidentIoIntegration` query returns 404 for revoked integrations without distinguishing from non-existent ones.
  - File: `incidentio.resolvers.go:352-354`

**GraphQL Schema (`incidentio.graphqls`):**
- [ ] **GQL-009**: `testIncidentIoIntegration` uses `CreateIncidentIoIntegrationInput` as input type, requiring all create fields (name, apiKey, syncBy) just to test a connection. Should have a simpler input type.
  - File: `incidentio.graphqls:110-112`

- [ ] **GQL-010**: No `deleteIncidentIoIntegration` mutation in the schema. Integrations can only be revoked, not deleted.

**Helpers (`incidentio.helpers.go`):**
- [ ] **GQL-011**: `startIncidentIoSync` creates a background goroutine with `context.WithoutCancel`. If the original request context has org-specific data, this could cause stale context issues.
  - File: `incidentio.helpers.go:20-33`

**UI Analysis (static review of React components):**
- [ ] **UI-001**: No sync error feedback. `SyncNowCard` shows "Sync Now" button but doesn't display sync errors.
- [ ] **UI-002**: `lastSyncedAt` displayed as timestamp but doesn't indicate success/failure of that sync.
- [ ] **UI-003**: `CreateIncidentIoIntegrationDrawer` tests connection before create, but doesn't show what specific error occurred on failure.
- [ ] **UI-004**: No loading state when fetching schedules from incident.io in `AddIncidentIoSchedulesDrawer`.
- [ ] **UI-005**: No confirmation dialog when changing sync mode (EMAIL <-> IDENTITY_SET), which clears identity set configuration.

### Phase 6: Report Generation & Periodic Sync Review (~5%)

**Periodic Sync (`periodic/periodic.go`):**
- [ ] **PERIODIC-001**: No error aggregation. Each integration syncs independently, but total failure count is not reported.
  - File: `periodic/periodic.go:28-53`

- [ ] **PERIODIC-002**: Context cancellation check only happens at `errors.Is(err, context.Canceled)`. If sync is slow, it blocks the job scheduler.
  - File: `periodic/periodic.go:46-47`

- [ ] **PERIODIC-003**: `UnsafeGetOrgIntegrationConfigsByPlatform` - the "Unsafe" prefix suggests this bypasses authorization. Appropriate for system context but worth noting.

**Demo Mock Servers:**
- [ ] **MOCK-001**: Basic mock doesn't implement pagination (always returns empty `after` cursor). SDK pagination loop works but isn't tested with real pagination.
  - File: `demo/mock_server.go:91-92`

- [ ] **MOCK-002**: Rich mock's `getOnCallMembers` uses `rand.New(rand.NewSource(...))` with deterministic seed based on schedule ID and hour. Not cryptographically random, but that's fine for mock - however, it means tests aren't hermetic (results change hourly).
  - File: `demo/rich_mock_server.go:165`

- [ ] **MOCK-003**: Neither mock validates required query parameters (e.g., `schedule_id` for entries endpoint). Real API would return 400.
  - File: `demo/mock_server.go:129-130` (doesn't validate missing schedule_id)

## Files Summary

| File | Action | Purpose |
|------|--------|---------|
| `qa/go.mod` | Create | QA module definition |
| `qa/sdk_test.go` | Create | SDK runtime tests against mock servers |
| `qa/static_analysis_report.go` | Create | Static analysis findings catalog |
| `qa/report.go` | Create | Issue report formatter |
| `qa/run_qa.sh` | Create | Main QA entry point |
| `qa/README.md` | Create | QA agent documentation |

**Source files analyzed (read-only, NOT modified):**

| File | Analysis Findings |
|------|-------------------|
| `go-incidentio/client.go` | SEC-005, SEC-006, CLIENT-005 |
| `go-incidentio/errors.go` | ERR-001 through ERR-004 |
| `go-incidentio/schedule.go` | SCHED tests |
| `go-incidentio/schedule_entry.go` | ENTRY tests, CLIENT-001 |
| `go-incidentio/user.go` | USER tests |
| `go-incidentio/incidentio.go` | (clean) |
| `pkg/incidentio/client.go` | CLIENT-001 through CLIENT-005 |
| `pkg/incidentio/sync.go` | SYNC-001 through SYNC-013 |
| `pkg/incidentio/config.go` | CONFIG-001 through CONFIG-003 |
| `pkg/incidentio/validation.go` | VAL-001 through VAL-003 |
| `pkg/incidentio/apikey.go` | SEC-001 through SEC-003 |
| `pkg/incidentio/token.go` | SEC-004 |
| `pkg/incidentio/periodic/periodic.go` | PERIODIC-001 through PERIODIC-003 |
| `pkg/models/incidentio_types.go` | (clean) |
| `pkg/graphql/graph/incidentio.resolvers.go` | GQL-001 through GQL-008 |
| `pkg/graphql/graph/incidentio.helpers.go` | GQL-011 |
| `pkg/graphql/graph/incidentio.graphqls` | GQL-009, GQL-010 |
| `pkg/tags/incidentio.go` | (clean) |
| `adminui/` (all components) | UI-001 through UI-005 |
| `demo/mock_server.go` | MOCK-001, MOCK-003 |
| `demo/rich_mock_server.go` | MOCK-002 |

## Definition of Done

- [ ] All 50+ identified issues cataloged with severity, file:line, reproduction path
- [ ] SDK runtime tests pass against both mock servers
- [ ] Static analysis findings verified against actual code
- [ ] Issue report generated in Markdown format
- [ ] QA agent is re-runnable (idempotent, no side effects)
- [ ] Zero modifications to source repo
- [ ] Issues prioritized by severity tier (Critical/High/Medium/Low)

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Mock servers don't match real incident.io API behavior | Medium | High | Cross-reference with incident.io API docs |
| SDK tests can't compile due to missing StrongDM internal deps | High | Medium | Test only the standalone `go-incidentio` package which has no internal deps |
| False positives in static analysis | Medium | Low | Each issue verified against actual code with specific line references |
| Some edge cases only manifest at runtime with real API | Medium | Medium | Document as "requires live API testing" in report |
| Hourly mock rotation changes test results | Low | Low | Use fixed seed or snapshot approach for deterministic tests |

## Security Considerations

- QA agent must not expose API keys or credentials in reports
- Mock server API key (`demo-api-key-12345`) is already public in source
- No actual incident.io API calls made during QA validation
- All tests run against local mock servers only

## Dependencies

- Go 1.21+ (for running SDK tests)
- Access to demo mock servers (included in repo)
- No external dependencies beyond what's in `go-incidentio/go.mod`

## Open Questions

1. Should the QA agent also validate the incident.io API documentation against the SDK implementation?
2. Should the agent flag the complete absence of unit tests as a Critical issue or track it separately?
3. Is the 23-hour API key cache TTL a deliberate product decision or an arbitrary choice from copying PagerDuty patterns?
4. Should the periodic sync interval (15 min per UI) be validated against the 23-hour key TTL?
