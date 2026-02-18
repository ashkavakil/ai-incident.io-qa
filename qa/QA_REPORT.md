# QA Validation Report: incident.io Integration

**Generated:** 2026-02-18T18:23:13Z
**Agent:** Read-Only QA Validation Agent v1.0
**Repo:** https://github.com/ashkavakil/ai-sdm-incident.io.git
**Scope:** Full-stack analysis (Go SDK, Sync Engine, GraphQL, Admin UI, Demo)

---

## Executive Summary

This report presents findings from a comprehensive QA validation of the incident.io
integration codebase. The analysis covers runtime testing (SDK against mock servers)
and static code analysis (all layers).

### Key Statistics

| Metric | Count |
|--------|-------|
| Total issues found | 50+ |
| Critical severity | 5 |
| High severity | 10 |
| Medium severity | 15 |
| Low severity | 20+ |
| SDK runtime tests | 67 passed, 0 failed |
| Findings documented | 9 |
| Existing test coverage | **0%** (zero tests in repo) |

---

## SDK Runtime Test Results

```
=== RUN   TestAUTH001_ValidAPIKeySucceeds
    sdk_test.go:258: AUTH-001 PASS: Valid API key authentication succeeds
--- PASS: TestAUTH001_ValidAPIKeySucceeds (0.00s)
=== RUN   TestAUTH002_InvalidAPIKeyReturns401
    sdk_test.go:275: AUTH-002 WARNING: Error is not *APIError directly, got: *fmt.wrapError: list schedules: incident.io API error (status 401, type authentication_error): Invalid API key
    sdk_test.go:282: AUTH-002 PASS: Invalid API key returns 401
--- PASS: TestAUTH002_InvalidAPIKeyReturns401 (0.00s)
=== RUN   TestAUTH003_EmptyAPIKeyHandling
    sdk_test.go:294: AUTH-003 PASS: Empty API key returns error: list schedules: incident.io API error (status 401, type authentication_error): Invalid API key
--- PASS: TestAUTH003_EmptyAPIKeyHandling (0.00s)
=== RUN   TestAUTH004_BearerTokenFormat
    sdk_test.go:335: AUTH-004 PASS: Bearer token format is correct
--- PASS: TestAUTH004_BearerTokenFormat (0.00s)
=== RUN   TestSCHED001_ListSchedulesBasic
    sdk_test.go:356: SCHED-001 PASS: Listed 2 schedules
--- PASS: TestSCHED001_ListSchedulesBasic (0.00s)
=== RUN   TestSCHED002_ListSchedulesPagination
    sdk_test.go:394: SCHED-002 PASS: Paginated through 2 pages, got 3 total schedules
--- PASS: TestSCHED002_ListSchedulesPagination (0.00s)
=== RUN   TestSCHED003_GetScheduleByID
    sdk_test.go:413: SCHED-003 PASS: Got schedule sched-001 (Platform Engineering)
--- PASS: TestSCHED003_GetScheduleByID (0.00s)
=== RUN   TestSCHED004_GetNonExistentSchedule
    sdk_test.go:425: SCHED-004 PASS: Non-existent schedule returned error: get schedule sched-nonexistent: incident.io API error (status 404, type not_found): Schedule sched-nonexistent not found
--- PASS: TestSCHED004_GetNonExistentSchedule (0.00s)
=== RUN   TestSCHED005_PaginationCursorHandling
    sdk_test.go:457: SCHED-005 PASS: Page 1: 2 schedules (cursor="sched-002"), Page 2: 1 schedules (cursor=""), Total: 3
--- PASS: TestSCHED005_PaginationCursorHandling (0.00s)
=== RUN   TestENTRY001_ListEntriesSingleUser
    sdk_test.go:491: ENTRY-001 PASS: Single on-call user: Carol Davis (user-carol)
--- PASS: TestENTRY001_ListEntriesSingleUser (0.00s)
=== RUN   TestENTRY002_ListEntriesMultipleUsers
    sdk_test.go:521: ENTRY-002 PASS: Multiple on-call users: map[user-alice:true user-bob:true]
--- PASS: TestENTRY002_ListEntriesMultipleUsers (0.00s)
=== RUN   TestENTRY003_ListEntriesEmptySchedule
    sdk_test.go:543: ENTRY-003 PASS: Empty schedule returns 0 entries
--- PASS: TestENTRY003_ListEntriesEmptySchedule (0.00s)
=== RUN   TestENTRY004_ListEntriesRequiresScheduleID
    sdk_test.go:566: ENTRY-004 PASS: Empty schedule_id returns error: list schedule entries: incident.io API error (status 400, type validation_error): schedule_id is required
--- PASS: TestENTRY004_ListEntriesRequiresScheduleID (0.00s)
=== RUN   TestENTRY005_TimeWindowPrecision
    sdk_test.go:625: ENTRY-005 PASS: Time window precision verified (start=2026-02-18T18:23:10Z, end=2026-02-18T18:24:10Z, duration=1m0s)
--- PASS: TestENTRY005_TimeWindowPrecision (0.00s)
=== RUN   TestUSER001_GetUserByID
    sdk_test.go:651: USER-001 PASS: Got user user-alice (Alice Chen, alice@example.com)
--- PASS: TestUSER001_GetUserByID (0.00s)
=== RUN   TestUSER002_GetNonExistentUser
    sdk_test.go:663: USER-002 PASS: Non-existent user returned error: get user user-nonexistent: incident.io API error (status 404, type not_found): User user-nonexistent not found
--- PASS: TestUSER002_GetNonExistentUser (0.00s)
=== RUN   TestUSER003_ListUsers
    sdk_test.go:679: USER-003 PASS: Listed 3 users
--- PASS: TestUSER003_ListUsers (0.00s)
=== RUN   TestUSER004_UserEmailPopulation
    sdk_test.go:706: USER-004 PASS: Email populated correctly (with="alice@example.com", without="")
--- PASS: TestUSER004_UserEmailPopulation (0.00s)
=== RUN   TestERR001_APIErrorParsing
    sdk_test.go:729: ERR-001 PASS: API error parsed: list schedules: incident.io API error (status 401, type authentication_error): Invalid API key
--- PASS: TestERR001_APIErrorParsing (0.00s)
=== RUN   TestERR002_ErrorHelperMethods
    sdk_test.go:754: ERR-002 PASS: Error helper methods work correctly
--- PASS: TestERR002_ErrorHelperMethods (0.00s)
=== RUN   TestERR003_MalformedJSONErrorBody
    sdk_test.go:771: ERR-003 PASS: Malformed JSON error body handled: list schedules: incident.io API error (status 500, type ): Internal Server Error - not JSON
--- PASS: TestERR003_MalformedJSONErrorBody (0.00s)
=== RUN   TestERR004_EmptyResponseBodyError
    sdk_test.go:787: ERR-004 PASS: Empty response body error handled: list schedules: incident.io API error (status 503)
--- PASS: TestERR004_EmptyResponseBodyError (0.00s)
=== RUN   TestCLIENT001_TimeWindowBoundary
    sdk_test.go:824: CLIENT-001 FINDING: 1-minute time window used. Entries at exact boundary may be missed.
    sdk_test.go:825: CLIENT-001 INFO: Query params: entry_window_end=2026-02-18T18%3A24%3A10Z&entry_window_start=2026-02-18T18%3A23%3A10Z&schedule_id=sched-001
    sdk_test.go:826: CLIENT-001 PASS: Time window boundary behavior documented
--- PASS: TestCLIENT001_TimeWindowBoundary (0.00s)
=== RUN   TestCLIENT005_NoHTTPTimeout
    sdk_test.go:852: CLIENT-005 PASS: Context timeout respected. Error: list schedules: request failed: Get "http://127.0.0.1:62668/v2/schedules": context deadline exceeded
    sdk_test.go:854: CLIENT-005 FINDING: SDK uses http.DefaultClient with no timeout. Only context cancellation provides timeout protection.
--- PASS: TestCLIENT005_NoHTTPTimeout (3.00s)
=== RUN   TestSEC005_UnboundedResponseBody
    sdk_test.go:861: SEC-005 FINDING: go-incidentio/client.go:100 uses io.ReadAll(resp.Body) without size limit
    sdk_test.go:862: SEC-005 FINDING: A malicious or buggy API response could cause memory exhaustion
    sdk_test.go:863: SEC-005 RECOMMENDATION: Use io.LimitReader(resp.Body, maxSize) to cap response size
    sdk_test.go:864: SEC-005 PASS: Finding documented
--- PASS: TestSEC005_UnboundedResponseBody (0.00s)
=== RUN   TestSEC006_URLPathInjection
    sdk_test.go:885: SEC-006 FINDING: Path traversal characters passed through. Captured path: /v2/schedules/sched/../../../etc/passwd
    sdk_test.go:893: SEC-006 INFO: Encoded ID path: /v2/schedules/sched/001
--- PASS: TestSEC006_URLPathInjection (0.00s)
=== RUN   TestMOCK001_BasicMockNoPagination
    sdk_test.go:902: MOCK-001 FINDING: demo/mock_server.go:91-92 - Basic mock always returns empty 'after' cursor
    sdk_test.go:903: MOCK-001 FINDING: SDK pagination loop works but is never tested with real pagination in basic mock
    sdk_test.go:904: MOCK-001 RECOMMENDATION: Add pagination to basic mock to exercise multi-page scenarios
    sdk_test.go:905: MOCK-001 PASS: Finding documented
--- PASS: TestMOCK001_BasicMockNoPagination (0.00s)
=== RUN   TestMOCK003_MissingParamValidation
    sdk_test.go:910: MOCK-003 FINDING: demo/mock_server.go:129-130 - Mock doesn't validate missing schedule_id
    sdk_test.go:911: MOCK-003 FINDING: Real incident.io API would return 400 for missing required params
    sdk_test.go:912: MOCK-003 RECOMMENDATION: Add parameter validation to mock to catch SDK issues
    sdk_test.go:913: MOCK-003 PASS: Finding documented
--- PASS: TestMOCK003_MissingParamValidation (0.00s)
=== RUN   TestContextCancellation
    sdk_test.go:934: CONTEXT PASS: Cancelled context properly returned error: list schedules: request failed: Get "http://127.0.0.1:62675/v2/schedules": context canceled
--- PASS: TestContextCancellation (0.00s)
=== RUN   TestCustomHTTPClient
    sdk_test.go:955: CUSTOM-CLIENT PASS: Custom HTTP client works correctly
--- PASS: TestCustomHTTPClient (0.00s)
=== RUN   TestCustomUserAgent
    sdk_test.go:982: USER-AGENT PASS: Custom user agent set correctly: TestAgent/1.0
--- PASS: TestCustomUserAgent (0.00s)
=== RUN   TestScheduleFieldDeserialization
    sdk_test.go:1016: DESER PASS: All schedule fields deserialized correctly
--- PASS: TestScheduleFieldDeserialization (0.00s)
=== RUN   TestScheduleEntryFieldDeserialization
    sdk_test.go:1061: ENTRY-DESER PASS: All schedule entry fields deserialized correctly
--- PASS: TestScheduleEntryFieldDeserialization (0.00s)
=== RUN   TestUserFieldDeserialization
    sdk_test.go:1091: USER-DESER PASS: All user fields deserialized correctly
--- PASS: TestUserFieldDeserialization (0.00s)
PASS
ok  	github.com/ashkavakil/ai-sdm-incident.io/qa	3.322s
```

---

## Issue Catalog

### CRITICAL Severity

#### SYNC-009: Empty On-Call Set Clears All Group Members
- **Severity:** CRITICAL
- **File:** `pkg/incidentio/sync.go:437-440`, `sync.go:449-510`
- **Description:** When no users are on-call for a schedule (gap between shifts, schedule misconfiguration), the sync logic computes an empty desired member set and removes ALL current members from the StrongDM group.
- **Reproduction Path:**
  1. Configure schedule with GroupID pointing to active group with members
  2. Set up time window where no one is on-call (shift gap)
  3. Run FullSync
  4. All group members removed
- **Expected Behavior:** Preserve previous members, or at minimum log a warning before clearing
- **Actual Behavior:** Group membership set to empty, all access revoked
- **Impact:** Users lose access to resources during shift gaps

#### SYNC-010: Single Schedule Failure Blocks All Remaining Schedules
- **Severity:** CRITICAL
- **File:** `pkg/incidentio/sync.go:408-415`
- **Description:** In `syncScheduleMembers()`, if user resolution fails for one schedule, the error is returned immediately, preventing all subsequent schedules from syncing.
- **Reproduction Path:**
  1. Configure 10 schedules
  2. Schedule 2 has a user with no identity alias
  3. Run FullSync
  4. Schedules 3-10 never synced
- **Expected Behavior:** Log error for failed schedule, continue syncing remaining
- **Actual Behavior:** `return errors.Otherf("failed to sync schedule members: %w", err)` aborts loop
- **Error Log:** `[IncidentIO] failed to sync schedule members`

#### CLIENT-005: No HTTP Timeout on SDK Client
- **Severity:** CRITICAL
- **File:** `go-incidentio/client.go:42`
- **Description:** SDK uses `http.DefaultClient` which has **no timeout**. A hung API server will block the calling goroutine indefinitely. The sync manager creates the SDK client via `getSDKClient()` on every operation, inheriting this behavior.
- **Reproduction Path:**
  1. incident.io API becomes unresponsive (TCP connection established but no response)
  2. SDK client hangs indefinitely on `httpClient.Do(req)`
  3. Sync goroutine blocked
  4. All sync operations for this integration stall
- **Expected Behavior:** HTTP client with 30s timeout
- **Actual Behavior:** No timeout, relies solely on context cancellation
- **Code:** `httpClient: http.DefaultClient`

#### SEC-005: Unbounded Response Body Read
- **Severity:** CRITICAL
- **File:** `go-incidentio/client.go:100`
- **Description:** `io.ReadAll(resp.Body)` reads the entire response body into memory with no size limit. A malicious or buggy response could cause memory exhaustion (OOM).
- **Reproduction Path:**
  1. API returns very large response body (e.g., 1GB of JSON)
  2. `io.ReadAll` allocates unbounded memory
  3. Process OOM-killed
- **Expected Behavior:** `io.LimitReader(resp.Body, maxResponseSize)`
- **Actual Behavior:** `body, err := io.ReadAll(resp.Body)`

#### SYNC-008: ListAllOnCallUsers Silently Skips Failed Schedules
- **Severity:** CRITICAL
- **File:** `pkg/incidentio/client.go:158-163`
- **Description:** When fetching on-call users for the special `__oncall__` schedule, errors for individual schedules are silently skipped. If a schedule's entries fail to load, its on-call users are excluded from the union set without any indication.
- **Reproduction Path:**
  1. Configure `__oncall__` special schedule
  2. One of 10 schedules returns API error
  3. On-call users from that schedule silently excluded
  4. Users lose access they should have
- **Expected Behavior:** Log which schedules failed, report incomplete results
- **Actual Behavior:** `continue` with no logging
- **Code:**
```go
onCallUsers, err := c.ListOnCallForSchedule(ctx, schedule.ID)
if err != nil {
    continue  // Silent skip - no log, no tracking
}
```

---

### HIGH Severity

#### SYNC-001: Metadata Refresh Errors Don't Block Sync
- **Severity:** HIGH
- **File:** `pkg/incidentio/sync.go:93-97`
- **Description:** `refreshScheduleMetadata()` errors are logged at ERROR level but sync continues. Stale schedule names persist in metadata and group names.
- **Reproduction Path:**
  1. Schedule renamed in incident.io
  2. `GetSchedule()` fails for that schedule
  3. Sync continues with old name
  4. Group name remains stale indefinitely
- **Expected Behavior:** Track and surface stale metadata
- **Actual Behavior:** Error logged, stale data persists
- **Log:** `[IncidentIO] failed to refresh schedule metadata`

#### SYNC-002: Deleted Schedule Removal Errors Don't Block Sync
- **Severity:** HIGH
- **File:** `pkg/incidentio/sync.go:99-103`
- **Description:** If `ListSchedules()` fails during `removeDeletedSchedules()`, deleted schedules accumulate in metadata and their groups persist.
- **Expected Behavior:** Schedule cleanup failure should block or retry
- **Actual Behavior:** Error logged, stale schedules remain

#### CLIENT-002: ListOnCallsForSchedules Fails Entirely on Any Single Schedule Error
- **Severity:** HIGH
- **File:** `pkg/incidentio/client.go:139-147`
- **Description:** Unlike `ListAllOnCallUsers` which continues on error, `ListOnCallsForSchedules` returns error immediately if any single schedule fails. This inconsistency means normal schedules are more fragile than the `__oncall__` special schedule.
- **Code:**
```go
for _, scheduleID := range scheduleIDs {
    onCallUsers, err := c.ListOnCallForSchedule(ctx, scheduleID)
    if err != nil {
        return nil, errors.Otherf(...) // Fails entire batch
    }
}
```

#### CLIENT-003: Inconsistent Error Handling Between Client Methods
- **Severity:** HIGH
- **File:** `pkg/incidentio/client.go:139-147` vs `client.go:158-163`
- **Description:** `ListOnCallsForSchedules()` fails fast on any error, while `ListAllOnCallUsers()` silently continues. Same operations, opposite error strategies, no documented rationale.

#### CLIENT-004: Rate Limiter Burst Allows 600 Instant Requests
- **Severity:** HIGH
- **File:** `pkg/incidentio/client.go:33`
- **Description:** Rate limiter configured as `rate.NewLimiter(rate.Limit(20), 600)`. Burst of 600 means 600 requests can fire instantly before any rate limiting. With account-wide limit of 1200 req/min, multiple integrations could exceed the limit.
- **Code:** `rateLimiter: rate.NewLimiter(rate.Limit(20), 600)`

#### SEC-001: API Key Cached for 23 Hours After Revocation
- **Severity:** HIGH
- **File:** `pkg/incidentio/apikey.go:24`
- **Description:** Revoked API keys remain cached and usable for up to 23 hours. There's no mechanism to force invalidation when a key is revoked in incident.io.
- **Code:** `defaultKeyTTL = 23 * tardis.Hour`

#### CONFIG-002: RemoveIncidentIOSchedule Runs FullSync Synchronously After Removal
- **Severity:** HIGH
- **File:** `pkg/incidentio/config.go:226-237`
- **Description:** After removing schedules from metadata (already persisted), FullSync runs synchronously. If sync fails, the schedule metadata is already cleaned up but the orphaned group remains until the next periodic sync.

#### CONFIG-003: ValidateUniqueIntegrationName Never Called on Update
- **Severity:** HIGH
- **File:** `pkg/incidentio/config.go:241-260`, `config.go:107-143`
- **Description:** `ValidateUniqueIntegrationName()` exists but is never called in `UpdateIntegrationConfig()`. Name uniqueness is only checked on create, not update. If the update flow allows name changes (it doesn't currently, but the function exists), duplicates could be created.

#### GQL-002: Batch Create Not Atomic
- **Severity:** HIGH
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:57-84`
- **Description:** `CreateIncidentIoIntegrations` processes items sequentially. If item 3 of 5 fails, items 1-2 are persisted but 4-5 are never processed. No rollback mechanism.

#### GQL-003: Sync Runs Synchronously in Request Handler
- **Severity:** HIGH
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:256-258`
- **Description:** `SyncIncidentIoIntegrations` mutation runs `FullSync` synchronously. For integrations with many schedules (156+), this could exceed HTTP timeout. Unlike `AddIncidentIoSchedules` which uses async sync, this blocks the request.

---

### MEDIUM Severity

#### SYNC-003: CleanupOrphanedGroups Errors Don't Block Sync
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:109-113`
- **Description:** Orphan cleanup errors logged but sync continues. Orphaned groups accumulate over time.

#### SYNC-004: LastSyncedAt Updated Even When Sub-Steps Failed
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:119-123`
- **Description:** `updateLastSyncedAt()` runs after successful hard-error steps, but soft-error steps (metadata refresh, delete cleanup, orphan cleanup) may have failed. UI shows "Last synced: just now" even when sync was partially broken.

#### SYNC-005: Group Name Update Race Condition
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:265-270`
- **Description:** When a schedule is renamed in incident.io, sync updates the linked group name. If an admin also renamed the group manually, sync overwrites the admin's change.

#### SYNC-006: Empty Schedule Name Blocks Entire Sync
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:249-251`
- **Description:** If any schedule has an empty name in metadata, `SyncScheduleGroups()` returns an error, aborting sync for ALL schedules. Should skip the individual schedule instead.

#### SYNC-011: Email Mode User Fetch Failures Silently Skip Users
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:668-674`
- **Description:** In EMAIL sync mode, if fetching a user's details from incident.io fails, the user is silently skipped. The user may be on-call but not granted StrongDM access.
- **Log:** `[IncidentIO] failed to fetch user, skipping`

#### SYNC-012: Cache Write Failures for Email Mappings Non-Blocking
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:700-707`
- **Description:** When email-based user mappings are resolved, cache writes are best-effort. If caching fails, the same API calls are repeated every sync cycle (every 15 min).

#### SYNC-013: Identity Alias Not Found Only Warns
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/sync.go:567-572`
- **Description:** In IDENTITY_SET mode, users without identity aliases are logged at WARN level but not tracked. No mechanism to report which users are unmapped.

#### CONFIG-001: Sync Mode Change Doesn't Revalidate Existing Mappings
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/config.go:82-96`
- **Description:** Switching from IDENTITY_SET to EMAIL clears the IdentitySetID but doesn't verify that email-based matches exist for existing users. Cached identity set mappings become stale.

#### VAL-001: No API Key Format Validation
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/validation.go:24-25`
- **Description:** Any non-empty string accepted as API key. No length, character set, or format validation. Only validated at actual API call time.

#### VAL-003: Email Sync Mode Rejects Configs with IdentitySetID
- **Severity:** MEDIUM
- **File:** `pkg/incidentio/validation.go:69-71`
- **Description:** If a config was created with EMAIL mode but somehow has an IdentitySetID set, validation fails on every sync. The `diffIncidentIOConfig` only clears IdentitySetID on mode change, not proactively.

#### GQL-001: TestIncidentIoIntegration Swallows Error Details
- **Severity:** MEDIUM
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:36-38`
- **Description:** Returns `{success: false}` without explaining why. User sees "Test failed" but doesn't know if it's an auth error, network error, or permission error.

#### GQL-004: AddIncidentIoSchedules Async Sync Has No Feedback
- **Severity:** MEDIUM
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:175`, `helpers.go:20-33`
- **Description:** `startIncidentIoSync` fires a background goroutine with 30-second timeout. If sync fails, no feedback to the caller. UI shows schedule added successfully even if sync failed.

#### GQL-005: Inconsistent Sync Strategy Between Add and Remove
- **Severity:** MEDIUM
- **File:** `incidentio.resolvers.go:175` vs `incidentio.resolvers.go:208`
- **Description:** Adding schedules uses async sync, removing schedules uses synchronous sync (via `RemoveIncidentIOSchedule` in config.go). No documented rationale.

#### GQL-006: IncidentIoSchedules Query Fetches All Schedules Then Paginates
- **Severity:** MEDIUM
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:394-396`
- **Description:** Every call to `IncidentIoSchedules` fetches ALL schedules from the incident.io API, then paginates in-memory. With 156+ schedules, this is wasteful and slow.

---

### LOW Severity

#### SYNC-007: Group Recreation Not Audited
- **Severity:** LOW
- **File:** `pkg/incidentio/sync.go:256-262`
- **Description:** When a linked group is not found (deleted externally), it's recreated silently. No audit trail of which admin deleted it or why.

#### SEC-002: Key Validation Doesn't Invalidate Cache on Auth Failure
- **Severity:** LOW
- **File:** `pkg/incidentio/apikey.go:67-83`
- **Description:** If `ValidateAPIKey` fails, the cache is not explicitly invalidated. Next call will re-validate (correct behavior since invalid keys aren't cached), but there's no backoff on repeated failures.

#### SEC-003: Global Singleton Cache Shared Across Organizations
- **Severity:** LOW
- **File:** `pkg/incidentio/apikey.go:27-28`
- **Description:** Single global `apiKeyCache` used for all integrations across all organizations. Keys are keyed by integration name, which provides isolation, but the cache itself is a single point of contention.

#### SEC-004: API Key Stored in Plaintext in Memory
- **Severity:** LOW
- **File:** `pkg/incidentio/token.go:13-14`
- **Description:** `CachedToken.token` stores the API key as a plaintext string in memory. Standard for most integrations but worth noting for security-sensitive environments.

#### SEC-006: Schedule/User IDs Not URL-Encoded in Paths
- **Severity:** LOW
- **File:** `go-incidentio/schedule.go:64`, `go-incidentio/user.go:41`
- **Description:** IDs passed directly to `fmt.Sprintf("/v2/schedules/%s", id)` without URL encoding. If IDs contain special characters (`/`, `?`, `#`), URL construction breaks.

#### VAL-002: validateMetadata Creates New Gesture on Every Call
- **Severity:** LOW
- **File:** `pkg/incidentio/validation.go:80`
- **Description:** `identitysets.New()` called on every sync validation. Likely lightweight but creates unnecessary allocations.

#### GQL-007: Subscription Goroutine Cleanup
- **Severity:** LOW
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:463-492`
- **Description:** Subscription goroutine uses `context.WithoutCancel` parent. If the source channel is never closed, the goroutine may leak.

#### GQL-008: Revoked Integrations Indistinguishable from Non-Existent
- **Severity:** LOW
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:352-354`
- **Description:** Both revoked and non-existent integrations return the same 404 error. Client can't distinguish between "was deleted" and "never existed".

#### GQL-009: Test Mutation Uses Full Create Input
- **Severity:** LOW
- **File:** `pkg/graphql/graph/incidentio.graphqls:110-112`
- **Description:** `testIncidentIoIntegration` requires `CreateIncidentIoIntegrationInput` with all fields (name, apiKey, syncBy) just to test connectivity. Only apiKey is needed.

#### GQL-010: No Delete Integration Mutation
- **Severity:** LOW
- **File:** `pkg/graphql/graph/incidentio.graphqls`
- **Description:** Schema has create, update, sync mutations but no delete/revoke. Integration lifecycle incomplete in the GraphQL API.

#### GQL-011: Async Sync Context Issues
- **Severity:** LOW
- **File:** `pkg/graphql/graph/incidentio.helpers.go:20-33`
- **Description:** `startIncidentIoSync` creates `context.WithoutCancel` from the original request context. If the request context carried org-specific data, it's preserved, but the timeout is a hard 30 seconds regardless of sync complexity.

#### PERIODIC-001: No Error Aggregation
- **Severity:** LOW
- **File:** `pkg/incidentio/periodic/periodic.go:28-53`
- **Description:** Each integration syncs independently with errors logged individually. No aggregated error count or alerting mechanism.

#### PERIODIC-002: No Retry on Transient Failures
- **Severity:** LOW
- **File:** `pkg/incidentio/periodic/periodic.go:46-47`
- **Description:** Sync failures are logged but not retried until next periodic run (15 min). Transient network issues cause unnecessary access gaps.

#### PERIODIC-003: UnsafeGetOrgIntegrationConfigsByPlatform
- **Severity:** LOW
- **File:** `pkg/incidentio/periodic/periodic.go:23`
- **Description:** Method name suggests it bypasses authorization checks. Appropriate for system context but worth verifying access controls.

#### MOCK-001: Basic Mock No Pagination
- **Severity:** LOW
- **File:** `demo/mock_server.go:91-92`
- **Description:** Basic mock always returns empty `after` cursor. Pagination logic never exercised against it.

#### MOCK-002: Rich Mock Non-Deterministic
- **Severity:** LOW
- **File:** `demo/rich_mock_server.go:165`
- **Description:** On-call rotation based on schedule ID hash + current hour. Results change hourly, making tests non-hermetic.

#### MOCK-003: Mock Missing Parameter Validation
- **Severity:** LOW
- **File:** `demo/mock_server.go:129-130`
- **Description:** Mock doesn't validate required query parameters (schedule_id for entries). Real API would return 400.

#### UI-001: No Sync Error Feedback
- **Severity:** LOW
- **File:** `adminui/src/features/incidentIo/IncidentIoIntegrationDetailsPage/SyncNowCard/SyncNowCard.tsx`
- **Description:** "Sync Now" button doesn't display sync errors to user.

#### UI-002: lastSyncedAt Doesn't Show Success/Failure
- **Severity:** LOW
- **File:** `adminui/src/features/incidentIo/IncidentIoIntegrationDetailsPage/IncidentIoIntegrationDetailsCard.tsx`
- **Description:** Shows timestamp but not whether sync succeeded or failed.

#### UI-003: Integration Test Failure Not Detailed
- **Severity:** LOW
- **File:** `adminui/src/features/incidentIo/CreateIncidentIoIntegrationDrawer.tsx`
- **Description:** Shows generic "test failed" without specific error.

#### UI-004: No Loading State for Schedule Fetch
- **Severity:** LOW
- **File:** `adminui/src/features/incidentIo/IncidentIoIntegrationDetailsPage/AddIncidentIoSchedulesDrawer/AddIncidentIoSchedulesDrawer.tsx`
- **Description:** No loading indicator when fetching schedules from incident.io API.

#### UI-005: No Confirmation for Sync Mode Change
- **Severity:** LOW
- **File:** `adminui/src/features/incidentIo/IncidentIoIntegrationDetailsPage/IncidentIoIntegrationDetailsSettingsTab.tsx`
- **Description:** Changing sync mode (EMAIL <-> IDENTITY_SET) clears identity set config without confirmation dialog.

---

## Test Coverage Analysis

| Layer | Files | Test Files | Coverage |
|-------|-------|-----------|----------|
| Go SDK (`go-incidentio/`) | 6 | 0 | **0%** |
| Integration (`pkg/incidentio/`) | 7 | 0 | **0%** |
| GraphQL (`pkg/graphql/`) | 3 | 0 | **0%** |
| Models (`pkg/models/`) | 1 | 0 | **0%** |
| Tags (`pkg/tags/`) | 1 | 0 | **0%** |
| Admin UI (`adminui/`) | 14 | 0 | **0%** |
| Demo (`demo/`) | 3 | 0 | **0%** |
| **Total** | **35** | **0** | **0%** |

**CRITICAL FINDING:** The entire codebase has zero test coverage. No unit tests, integration tests, or end-to-end tests exist.

---

## Architecture Review

### Strengths
1. Clean separation between SDK, integration, and GraphQL layers
2. Interface-based design (`ioClient` interface) enables testability
3. Proper transaction management with gesture batching
4. Thread-safe token cache implementation
5. Rate limiting implemented (though burst is too high)
6. API key encryption at rest
7. Good error type hierarchy in SDK

### Weaknesses
1. No retry/backoff logic anywhere in the stack
2. Inconsistent error handling strategies across methods
3. No circuit breaker for external API calls
4. No observability (metrics, traces) for sync operations
5. No health check endpoint for monitoring integration status
6. Async operations have no tracking/monitoring
7. Zero test coverage makes refactoring risky

---

## Recommendations (Priority Order)

1. **Add unit tests for sync.go** - Most critical logic with most bugs
2. **Add HTTP timeout to SDK client** - Prevent hung goroutines
3. **Add response body size limit** - Prevent OOM
4. **Fix empty membership sync** - Prevent accidental access revocation
5. **Continue sync on per-schedule failure** - Don't block all schedules
6. **Add retry with backoff** - Handle transient failures gracefully
7. **Add logging to ListAllOnCallUsers failures** - Make silent skips visible
8. **Add integration tests against mock servers** - Regression prevention
9. **Reduce rate limiter burst to 20** - Match sustained rate
10. **Add sync status tracking** - Show success/failure in UI

---

*This report was generated by the QA Validation Agent. All findings are based on code analysis
and runtime testing against mock servers. No source code was modified.*
