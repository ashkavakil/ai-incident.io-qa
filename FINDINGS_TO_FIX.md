# Actionable Findings to Fix

**Generated:** 2026-02-18
**Confidence:** Issues marked [PROVEN] were verified via runtime tests. Issues marked [STATIC] were found via code review only.

---

## CRITICAL - Fix Immediately

### 1. [PROVEN] SEC-005: Unbounded Response Body Read — OOM Risk
- **File:** `go-incidentio/client.go:100`
- **Error:** `io.ReadAll(resp.Body)` reads entire response with no size limit
- **Proven by:** `TestEDGE_ReadAllVsLimitReader` — confirmed 5MB read unbounded
- **Impact:** Malicious or buggy API response causes process OOM kill
- **Fix:**
```go
// Replace line 100:
body, err := io.ReadAll(resp.Body)
// With:
body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024)) // 10MB max
```

### 2. [PROVEN] CLIENT-005: No HTTP Timeout on SDK Client
- **File:** `go-incidentio/client.go:42`
- **Error:** Uses `http.DefaultClient` which has zero timeout
- **Proven by:** `TestCLIENT005_NoHTTPTimeout` — 3s slow server blocks until context cancels
- **Impact:** Hung API server blocks goroutine indefinitely. Sync stalls.
- **Fix:**
```go
// Replace line 42:
httpClient: http.DefaultClient,
// With:
httpClient: &http.Client{Timeout: 30 * time.Second},
```

### 3. [PROVEN] SEC-006: URL Path Injection via Special Characters in IDs
- **File:** `go-incidentio/schedule.go:64`, `go-incidentio/user.go:41`
- **Error:** IDs passed directly to `fmt.Sprintf("/v2/schedules/%s", id)` without URL encoding
- **Proven by:** `TestSEC006_URLPathInjection` — path traversal `../../../etc/passwd` passes through
- **Proven by:** `TestEDGE_SpecialCharsInScheduleID` — `?` in ID leaks into query string
- **Reproduction:**
```
ID = "sched?id=evil"  → server receives path="/v2/schedules/sched" query="id=evil"
ID = "sched/../../../etc/passwd" → server receives path="/v2/schedules/sched/../../../etc/passwd"
```
- **Fix:**
```go
// Replace schedule.go:64:
body, err := c.get(ctx, fmt.Sprintf("/v2/schedules/%s", id), nil)
// With:
body, err := c.get(ctx, fmt.Sprintf("/v2/schedules/%s", url.PathEscape(id)), nil)

// Same fix needed in user.go:41
```

### 4. [STATIC] SYNC-009: Empty On-Call Set Clears All Group Members
- **File:** `pkg/incidentio/sync.go:437-440` → `syncGroupMembership()` at line 449-510
- **Error:** When no users are on-call (shift gap), `desiredMembers` is empty, so ALL current members are removed from the StrongDM group
- **Impact:** Users lose access to resources during shift gaps
- **Reproduction path:**
  1. Configure a schedule linked to a group with 5 members
  2. Reach a time window where no one is on-call (gap between shifts)
  3. FullSync runs (periodic or manual)
  4. `onCallUserIDs` is empty → `matchedUsers` is empty → `desiredMembers` is empty
  5. `syncGroupMembership()` computes `toRemove = all current members`
  6. All 5 members removed from group
- **Fix:** Add empty-set guard before syncing:
```go
// In syncSingleScheduleMembers(), after resolveUsersToSDMAccounts():
if len(matchedUsers) == 0 && onCallUserIDs.Len() > 0 {
    log.Warn(ctx, "[IncidentIO] all on-call users unresolvable, preserving current membership",
        log.String("schedule_name", schedule.Name))
    return nil
}
if onCallUserIDs.Len() == 0 {
    log.Info(ctx, "[IncidentIO] no on-call users for schedule, preserving current membership",
        log.String("schedule_name", schedule.Name))
    return nil
}
```

### 5. [STATIC] SYNC-010: Single Schedule Failure Blocks All Remaining Schedules
- **File:** `pkg/incidentio/sync.go:408-415`
- **Error:** `syncScheduleMembers()` returns immediately on first error
- **Code that fails:**
```go
err := s.syncSingleScheduleMembers(ctx, schedule, onCallUserIDs)
if err != nil {
    log.Error(ctx, "[IncidentIO] failed to sync schedule members", ...)
    return errors.Otherf("failed to sync schedule members: %w", err)  // ← ABORTS LOOP
}
```
- **Impact:** If schedule 2 of 10 has an unresolvable user, schedules 3-10 never sync
- **Fix:** Collect errors, continue loop:
```go
var syncErrors []error
for i := range metadata.Schedules {
    // ...
    err := s.syncSingleScheduleMembers(ctx, schedule, onCallUserIDs)
    if err != nil {
        log.Error(ctx, "[IncidentIO] failed to sync schedule members", ...)
        syncErrors = append(syncErrors, fmt.Errorf("schedule %s: %w", schedule.Name, err))
        continue  // ← CONTINUE instead of return
    }
}
if len(syncErrors) > 0 {
    return fmt.Errorf("failed to sync %d schedule(s): %w", len(syncErrors), errors.Join(syncErrors...))
}
```

### 6. [STATIC] SYNC-008: ListAllOnCallUsers Silently Skips Failed Schedules
- **File:** `pkg/incidentio/client.go:158-163`
- **Error:** When fetching on-call users for `__oncall__` schedule, per-schedule errors silently `continue` with no logging
- **Code:**
```go
onCallUsers, err := c.ListOnCallForSchedule(ctx, schedule.ID)
if err != nil {
    continue  // ← SILENT skip, no log, no tracking
}
```
- **Impact:** Users from failed schedules silently excluded from access
- **Fix:**
```go
onCallUsers, err := c.ListOnCallForSchedule(ctx, schedule.ID)
if err != nil {
    log.Warn(ctx, "[IncidentIO] failed to fetch on-call for schedule, skipping",
        log.String("schedule_id", schedule.ID), log.Err(err))
    continue
}
```

---

## HIGH - Fix Soon

### 7. [PROVEN] EDGE-HTML: Raw HTML Leaked into Error Messages
- **File:** `go-incidentio/errors.go:77`
- **Error:** When API returns HTML (502 from nginx/LB), raw HTML stored in `APIError.Message`
- **Proven by:** `TestEDGE_HTMLErrorResponse` — error contains `<html><body><h1>502 Bad Gateway</h1>`
- **Impact:** HTML could be displayed to end users in admin UI
- **Fix:** Sanitize non-JSON error bodies:
```go
// In newAPIError(), after JSON parse failure:
if len(body) > 200 {
    apiErr.Message = string(body[:200]) + "... (truncated)"
} else {
    apiErr.Message = string(body)
}
```

### 8. [PROVEN] EDGE-REDIRECT: Auth Header Leaked on HTTP Redirects
- **File:** `go-incidentio/client.go:42`
- **Error:** `http.DefaultClient` follows redirects by default. Authorization header sent to redirect target.
- **Proven by:** `TestEDGE_HTTP301Redirect` — SDK follows redirect with auth header
- **Impact:** If incident.io API redirects to a different host, API key is sent to that host
- **Fix:**
```go
httpClient: &http.Client{
    Timeout: 30 * time.Second,
    CheckRedirect: func(req *http.Request, via []*http.Request) error {
        return http.ErrUseLastResponse // Don't follow redirects
    },
},
```

### 9. [PROVEN] EDGE-PAGINATION: No Protection Against Infinite Pagination Loop
- **File:** `go-incidentio/schedule.go:40-60` (ListSchedulesWithContext) and `pkg/incidentio/client.go:56-73` (ListSchedules wrapper)
- **Error:** Pagination loop has no max iteration guard. Server returning same cursor forever causes infinite loop.
- **Proven by:** `TestEDGE_PaginationInfiniteLoop` — SDK loops until context timeout
- **Fix:** Add max page limit in `pkg/incidentio/client.go:ListSchedules()`:
```go
const maxPages = 100
for page := 0; page < maxPages; page++ {
    // ... existing pagination loop
}
if page >= maxPages {
    return nil, errors.Otherf("pagination exceeded %d pages", maxPages)
}
```

### 10. [PROVEN] EDGE-429: No Retry on Rate Limit (429)
- **File:** `go-incidentio/client.go:82-110` (do method)
- **Error:** SDK returns 429 error immediately without checking `Retry-After` header or retrying
- **Proven by:** `TestEDGE_HTTP429RateLimit` — error returned, no retry attempted
- **Impact:** Transient rate limits cause sync failures. Must wait for next 15-min cycle.
- **Fix:** Add retry logic in `do()` method for 429 responses

### 11. [STATIC] CLIENT-003: Inconsistent Error Handling Between Client Methods
- **File:** `pkg/incidentio/client.go:139-147` vs `client.go:158-163`
- **Error:** `ListOnCallsForSchedules()` fails entire batch on any error, but `ListAllOnCallUsers()` silently continues. Same operation, opposite strategies.
- **Impact:** Normal schedules (using `ListOnCallsForSchedules`) are more fragile than `__oncall__` (using `ListAllOnCallUsers`)
- **Fix:** Make both methods consistent — either both continue-on-error with logging, or both fail-fast

### 12. [STATIC] CLIENT-004: Rate Limiter Burst Too High
- **File:** `pkg/incidentio/client.go:33`
- **Error:** `rate.NewLimiter(rate.Limit(20), 600)` — burst of 600 allows 600 instant requests
- **Impact:** Multiple integrations can exceed 1200 req/min account limit
- **Fix:** `rate.NewLimiter(rate.Limit(20), 20)` — burst matches sustained rate

### 13. [STATIC] CONFIG-002: RemoveIncidentIOSchedule Has Non-Atomic Behavior
- **File:** `pkg/incidentio/config.go:226-237`
- **Error:** Schedule metadata removed and persisted first, then FullSync runs. If sync fails, orphaned groups remain.
- **Impact:** Orphaned StrongDM groups persist until next periodic sync (up to 15 min)
- **Fix:** Run cleanup in same transaction, or defer sync failure to caller

### 14. [STATIC] GQL-002: Batch Create Not Atomic
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:57-84`
- **Error:** `CreateIncidentIoIntegrations` processes items sequentially. Item 3 failure leaves items 1-2 persisted, 4-5 unprocessed.
- **Impact:** Partial state on batch failure
- **Fix:** Wrap in transaction and rollback on any failure

### 15. [STATIC] GQL-003: SyncIncidentIoIntegrations Runs Synchronously
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:256-258`
- **Error:** `FullSync` runs synchronously in HTTP request handler. 156+ schedules could exceed HTTP timeout.
- **Impact:** GraphQL mutation timeout on large integrations
- **Fix:** Use async pattern like `startIncidentIoSync` (already used by AddIncidentIoSchedules)

---

## MEDIUM - Fix When Convenient

### 16. [STATIC] SYNC-001: Stale Schedule Names After Metadata Refresh Failure
- **File:** `pkg/incidentio/sync.go:93-97`
- **Error:** `refreshScheduleMetadata()` errors logged but sync continues with stale names

### 17. [STATIC] SYNC-006: Empty Schedule Name Blocks Entire Sync
- **File:** `pkg/incidentio/sync.go:249-251`
- **Error:** One schedule with empty name aborts `SyncScheduleGroups()` for ALL schedules
- **Fix:** Skip that schedule, log warning, continue

### 18. [STATIC] SEC-001: API Key Cached 23 Hours After Revocation
- **File:** `pkg/incidentio/apikey.go:24`
- **Error:** Revoked keys remain usable for up to 23 hours
- **Fix:** Invalidate cache on 401 response from any API call

### 19. [STATIC] VAL-001: No API Key Format Validation
- **File:** `pkg/incidentio/validation.go:24-25`
- **Error:** Any non-empty string accepted as API key

### 20. [STATIC] GQL-001: TestIncidentIoIntegration Swallows Error Details
- **File:** `pkg/graphql/graph/incidentio.resolvers.go:36-38`
- **Error:** Returns `{success: false}` without error message
- **Fix:** Return error details in response

### 21. [PROVEN] EDGE-EMPTY-ID: SDK Accepts Empty Schedule/User IDs
- **File:** `go-incidentio/schedule.go:63`, `go-incidentio/user.go:40`
- **Proven by:** `TestEDGE_EmptyScheduleID` — empty ID creates path `/v2/schedules/` which matches list endpoint
- **Fix:** Validate non-empty ID before making request

### 22. [PROVEN] EDGE-EMPTY-JSON: SDK Accepts Empty JSON Object
- **File:** `go-incidentio/schedule.go:54-58`
- **Proven by:** `TestEDGE_EmptyJSONObject` — `{}` response returns nil schedules without error
- **Fix:** Check for missing `schedules` key in response

---

## ZERO TEST COVERAGE - Needs Tests Written

These critical files have **0% runtime test coverage** because they depend on internal StrongDM packages. Unit tests need to be written inside the StrongDM monorepo.

### sync.go (711 lines — most critical file)
Needs tests for:
- [ ] `FullSync()` complete flow
- [ ] `refreshScheduleMetadata()` — API error handling
- [ ] `removeDeletedSchedules()` — schedule deletion edge cases
- [ ] `SyncScheduleGroups()` — group creation, recreation, renaming
- [ ] `CleanupOrphanedGroups()` — orphan detection
- [ ] `syncScheduleMembers()` — per-schedule error handling
- [ ] `syncSingleScheduleMembers()` — empty member set
- [ ] `syncGroupMembership()` — add/remove member logic
- [ ] `resolveUsersToSDMAccounts()` — EMAIL vs IDENTITY_SET modes
- [ ] `resolveUsersByEmail()` — cache hit/miss paths
- [ ] `resolveUsersByIdentityAlias()` — alias lookup

### client.go wrapper (180 lines)
Needs tests for:
- [ ] `ListOnCallForSchedule()` — 1-minute time window
- [ ] `ListOnCallsForSchedules()` — batch error handling
- [ ] `ListAllOnCallUsers()` — silent error skipping

### config.go (261 lines)
Needs tests for:
- [ ] `CreateIntegrationConfig()` — duplicate name check
- [ ] `UpdateIntegrationConfig()` — sync mode transitions
- [ ] `diffIncidentIOConfig()` — identity set clearing
- [ ] `AddIncidentIOSchedule()` — duplicate schedule prevention
- [ ] `RemoveIncidentIOSchedule()` — orphan cleanup

### validation.go (93 lines)
Needs tests for:
- [ ] `validateCreateConfig()` — all validation rules
- [ ] `validateSyncBy()` — enum validation
- [ ] `validateMetadata()` — identity set existence check

### GraphQL resolvers (499 lines)
Needs tests for:
- [ ] All mutations with valid/invalid inputs
- [ ] Batch operations with partial failures
- [ ] Async sync error handling

---

## Coverage Summary

| Layer | Lines | Runtime Coverage | Issues Found |
|-------|-------|-----------------|-------------|
| `go-incidentio/` SDK | 459 | **93.3%** | 6 proven bugs |
| `pkg/incidentio/` sync | 1,507 | **0%** | 13 static findings |
| `pkg/graphql/` resolvers | 751 | **0%** | 11 static findings |
| `pkg/models/` types | 61 | **0%** | 0 (type defs only) |
| `pkg/tags/` filters | 64 | **0%** | 0 (clean) |
| `adminui/` React UI | 1,379 | **0%** | 5 UI gaps |
| `demo/` mock servers | 663 | **0%** | 3 mock issues |
| **Total** | **4,884** | **~9%** | **38 actionable** |

**Bottom line:** The SDK layer is well-tested (93.3%), but the critical sync engine (1,507 lines, the most complex and bug-prone code) has zero runtime test coverage. You should prioritize writing unit tests for `sync.go` inside the StrongDM monorepo using mocked interfaces.
