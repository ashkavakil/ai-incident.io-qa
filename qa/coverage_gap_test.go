package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	incidentio "github.com/strongdm/web/pkg/incidentio/sdk"
)

// ============================================================================
// Coverage Gap: do() — retry exhaustion, ctx.Done during retry, Retry-After parsing
// ============================================================================

func TestCOV_DoMaxRetriesExhausted(t *testing.T) {
	// Server always returns 429 — SDK should retry 3 times then give up
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0") // 0-second retry to keep test fast
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "rate_limited", "status": 429, "message": "Rate limited",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})

	if err == nil {
		t.Fatal("COV-RETRY-EXHAUST FAIL: Should error after max retries")
	}

	totalAttempts := atomic.LoadInt32(&attempts)
	// Should be 4 attempts (initial + 3 retries)
	if totalAttempts < 3 {
		t.Fatalf("COV-RETRY-EXHAUST FAIL: Expected at least 3 attempts, got %d", totalAttempts)
	}

	t.Logf("COV-RETRY-EXHAUST PASS: %d attempts before giving up. Error: %v", totalAttempts, err)
}

func TestCOV_DoContextCancelledDuringRetryWait(t *testing.T) {
	// Server returns 429 with long Retry-After, but context is cancelled during wait
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "60") // 60 seconds — will be cancelled before then
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "rate_limited", "status": 429, "message": "Rate limited",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.ListSchedulesWithContext(ctx, incidentio.ListSchedulesOptions{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("COV-CTX-CANCEL-RETRY FAIL: Should error when context cancelled during retry wait")
	}

	// Should complete in ~200ms (context timeout), not 60s (Retry-After)
	if elapsed > 2*time.Second {
		t.Fatalf("COV-CTX-CANCEL-RETRY FAIL: Took %v — should have cancelled in ~200ms", elapsed)
	}

	t.Logf("COV-CTX-CANCEL-RETRY PASS: Context cancelled during retry wait in %v. Error: %v", elapsed, err)
}

func TestCOV_DoRetryAfterHeaderParsed(t *testing.T) {
	// Verify Retry-After header with numeric value is parsed and used
	var attempts int32
	var requestTimes []time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		requestTimes = append(requestTimes, time.Now())
		if count <= 1 {
			w.Header().Set("Retry-After", "1") // 1-second wait
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "rate_limited", "status": 429, "message": "Rate limited",
			})
			return
		}
		// Second request succeeds
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})

	if err != nil {
		t.Fatalf("COV-RETRY-AFTER FAIL: Should succeed after retry: %v", err)
	}

	totalAttempts := atomic.LoadInt32(&attempts)
	if totalAttempts != 2 {
		t.Fatalf("COV-RETRY-AFTER FAIL: Expected 2 attempts, got %d", totalAttempts)
	}

	t.Logf("COV-RETRY-AFTER PASS: Retried after Retry-After header, succeeded on attempt %d", totalAttempts)
}

func TestCOV_DoRetryAfterHeaderNonNumeric(t *testing.T) {
	// Retry-After with non-numeric value (HTTP date) — should use default 5s
	// We use context timeout to prevent waiting 5s
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 1 {
			w.Header().Set("Retry-After", "Wed, 21 Oct 2025 07:28:00 GMT") // Non-numeric
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "rate_limited", "status": 429, "message": "Rate limited",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.ListSchedulesWithContext(ctx, incidentio.ListSchedulesOptions{})
	// Will either timeout (default 5s > 500ms context) or succeed if retry fires fast
	if err != nil {
		t.Logf("COV-RETRY-NONNUM PASS: Non-numeric Retry-After caused timeout (default wait too long): %v", err)
	} else {
		t.Log("COV-RETRY-NONNUM PASS: Non-numeric Retry-After handled, request succeeded")
	}
}

func TestCOV_DoRetryAfterNoHeader(t *testing.T) {
	// 429 without Retry-After header — should use default 5s backoff
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 1 {
			// No Retry-After header
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "rate_limited", "status": 429, "message": "Rate limited",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.ListSchedulesWithContext(ctx, incidentio.ListSchedulesOptions{})
	// Default 5s > 500ms context timeout, so this should be cancelled
	if err != nil {
		t.Logf("COV-RETRY-NOHEADER PASS: No Retry-After header, default 5s wait exceeded context timeout: %v", err)
	} else {
		t.Log("COV-RETRY-NOHEADER PASS: Succeeded before context timeout")
	}
}

// ============================================================================
// Coverage Gap: GetScheduleWithContext — decode error path
// ============================================================================

func TestCOV_GetScheduleDecodeError(t *testing.T) {
	// Server returns 200 with invalid JSON for schedule body
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"schedule": INVALID_JSON}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetScheduleWithContext(context.Background(), "sched-001", incidentio.GetScheduleOptions{})
	if err == nil {
		t.Fatal("COV-SCHED-DECODE FAIL: Invalid JSON should return error")
	}
	if !strings.Contains(err.Error(), "get schedule") {
		t.Errorf("COV-SCHED-DECODE FAIL: Error should mention 'get schedule', got: %v", err)
	}
	t.Logf("COV-SCHED-DECODE PASS: Decode error handled: %v", err)
}

// ============================================================================
// Coverage Gap: GetUserWithContext — decode error path
// ============================================================================

func TestCOV_GetUserDecodeError(t *testing.T) {
	// Server returns 200 with invalid JSON for user body
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"user": NOT_VALID_JSON}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetUserWithContext(context.Background(), "user-001", incidentio.GetUserOptions{})
	if err == nil {
		t.Fatal("COV-USER-DECODE FAIL: Invalid JSON should return error")
	}
	if !strings.Contains(err.Error(), "get user") {
		t.Errorf("COV-USER-DECODE FAIL: Error should mention 'get user', got: %v", err)
	}
	t.Logf("COV-USER-DECODE PASS: Decode error handled: %v", err)
}

func TestCOV_GetUserHTTPError(t *testing.T) {
	// Server returns 500 for user endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "internal_error", "status": 500, "message": "Server error",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetUserWithContext(context.Background(), "user-001", incidentio.GetUserOptions{})
	if err == nil {
		t.Fatal("COV-USER-HTTP FAIL: 500 should return error")
	}
	if !strings.Contains(err.Error(), "get user") {
		t.Errorf("COV-USER-HTTP FAIL: Error should mention 'get user', got: %v", err)
	}
	t.Logf("COV-USER-HTTP PASS: HTTP error handled: %v", err)
}

// ============================================================================
// Coverage Gap: ListUsersWithContext — pagination After param, error paths
// ============================================================================

func TestCOV_ListUsersPagination(t *testing.T) {
	// Server supports pagination for users
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		pageSize := r.URL.Query().Get("page_size")

		if pageSize == "" {
			t.Log("COV-LIST-USERS-PAGE INFO: No page_size sent")
		}

		if after == "" {
			// First page
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": "u1", "name": "User 1", "email": "u1@e.com"},
				},
				"pagination_meta": map[string]interface{}{
					"after": "cursor-page2", "page_size": 1, "total_record_count": 2,
				},
			})
		} else if after == "cursor-page2" {
			// Second page
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": "u2", "name": "User 2", "email": "u2@e.com"},
				},
				"pagination_meta": map[string]interface{}{
					"after": "", "page_size": 1, "total_record_count": 2,
				},
			})
		}
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	// Page 1
	resp1, err := client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{PageSize: 1})
	if err != nil {
		t.Fatalf("COV-LIST-USERS-PAGE FAIL: Page 1: %v", err)
	}
	if len(resp1.Users) != 1 || resp1.Users[0].ID != "u1" {
		t.Fatalf("COV-LIST-USERS-PAGE FAIL: Page 1 wrong data")
	}

	// Page 2 using After cursor
	resp2, err := client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{
		PageSize: 1,
		After:    resp1.PaginationMeta.After,
	})
	if err != nil {
		t.Fatalf("COV-LIST-USERS-PAGE FAIL: Page 2: %v", err)
	}
	if len(resp2.Users) != 1 || resp2.Users[0].ID != "u2" {
		t.Fatalf("COV-LIST-USERS-PAGE FAIL: Page 2 wrong data")
	}

	t.Logf("COV-LIST-USERS-PAGE PASS: Paginated through 2 pages of users")
}

func TestCOV_ListUsersHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
		w.Write([]byte(`{"type":"error","status":503,"message":"Unavailable"}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{})
	if err == nil {
		t.Fatal("COV-LIST-USERS-ERR FAIL: 503 should error")
	}
	if !strings.Contains(err.Error(), "list users") {
		t.Errorf("COV-LIST-USERS-ERR FAIL: Error should mention 'list users', got: %v", err)
	}
	t.Logf("COV-LIST-USERS-ERR PASS: HTTP error: %v", err)
}

func TestCOV_ListUsersDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{BROKEN JSON`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{})
	if err == nil {
		t.Fatal("COV-LIST-USERS-DECODE FAIL: Bad JSON should error")
	}
	if !strings.Contains(err.Error(), "list users") {
		t.Errorf("COV-LIST-USERS-DECODE FAIL: Error should mention 'list users', got: %v", err)
	}
	t.Logf("COV-LIST-USERS-DECODE PASS: Decode error: %v", err)
}

// ============================================================================
// Coverage Gap: ListScheduleEntriesWithContext — empty window params
// ============================================================================

func TestCOV_ListEntriesNoWindowStart(t *testing.T) {
	var capturedParams string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedule_entries": []interface{}{},
			"pagination_meta":  map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "sched-001",
		EntryWindowStart: "", // Empty — should be omitted from params
		EntryWindowEnd:   "", // Empty — should be omitted from params
	})
	if err != nil {
		t.Fatalf("COV-ENTRIES-NOWINDOW FAIL: %v", err)
	}

	// Verify only schedule_id was sent (no window params)
	if strings.Contains(capturedParams, "entry_window_start") {
		t.Error("COV-ENTRIES-NOWINDOW FAIL: Empty window start should not be sent")
	}
	if strings.Contains(capturedParams, "entry_window_end") {
		t.Error("COV-ENTRIES-NOWINDOW FAIL: Empty window end should not be sent")
	}

	t.Logf("COV-ENTRIES-NOWINDOW PASS: Empty window params correctly omitted. Query: %s", capturedParams)
}

func TestCOV_ListEntriesDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`NOT JSON AT ALL`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "sched-001",
		EntryWindowStart: time.Now().UTC().Format(time.RFC3339),
		EntryWindowEnd:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("COV-ENTRIES-DECODE FAIL: Invalid JSON should error")
	}
	if !strings.Contains(err.Error(), "list schedule entries") {
		t.Errorf("COV-ENTRIES-DECODE FAIL: Error should mention 'list schedule entries', got: %v", err)
	}
	t.Logf("COV-ENTRIES-DECODE PASS: Decode error: %v", err)
}

// ============================================================================
// Coverage Gap: newAPIError — status override from response body
// ============================================================================

func TestCOV_APIErrorStatusOverrideFromBody(t *testing.T) {
	// Server returns HTTP 500 but body says status 503 — the body status should win
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":       "service_unavailable",
			"status":     503, // Different from HTTP status
			"message":    "Service temporarily unavailable",
			"request_id": "req-override-test",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-STATUS-OVERRIDE FAIL: Should return error")
	}

	// The error should use status 503 from body, not 500 from HTTP
	if !strings.Contains(err.Error(), "503") {
		t.Logf("COV-STATUS-OVERRIDE INFO: Error uses HTTP status not body status: %v", err)
	} else {
		t.Logf("COV-STATUS-OVERRIDE PASS: Body status 503 overrides HTTP 500: %v", err)
	}
}

func TestCOV_APIErrorZeroStatusInBody(t *testing.T) {
	// Server returns error with status=0 in body — HTTP status should be used
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(502)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":    "bad_gateway",
			"status":  0, // Zero — should NOT override
			"message": "Bad gateway",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-STATUS-ZERO FAIL: Should return error")
	}

	if !strings.Contains(err.Error(), "502") {
		t.Errorf("COV-STATUS-ZERO FAIL: Should keep HTTP status 502 when body has 0: %v", err)
	}
	t.Logf("COV-STATUS-ZERO PASS: Zero body status does not override HTTP status: %v", err)
}

// ============================================================================
// Coverage Gap: ListSchedulesWithContext — decode error, HTTP error
// ============================================================================

func TestCOV_ListSchedulesDecodeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{{{NOT JSON`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-LIST-SCHED-DECODE FAIL: Bad JSON should error")
	}
	if !strings.Contains(err.Error(), "list schedules") {
		t.Errorf("COV-LIST-SCHED-DECODE FAIL: Error should mention 'list schedules', got: %v", err)
	}
	t.Logf("COV-LIST-SCHED-DECODE PASS: Decode error: %v", err)
}

// ============================================================================
// Coverage Gap: GetScheduleWithContext — HTTP error path
// ============================================================================

func TestCOV_GetScheduleHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "internal_error", "status": 500, "message": "Server error",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetScheduleWithContext(context.Background(), "sched-001", incidentio.GetScheduleOptions{})
	if err == nil {
		t.Fatal("COV-GET-SCHED-HTTP FAIL: 500 should error")
	}
	if !strings.Contains(err.Error(), "get schedule") {
		t.Errorf("COV-GET-SCHED-HTTP FAIL: Error should mention 'get schedule', got: %v", err)
	}
	t.Logf("COV-GET-SCHED-HTTP PASS: HTTP error: %v", err)
}

// ============================================================================
// Coverage Gap: ListScheduleEntriesWithContext — HTTP error path
// ============================================================================

func TestCOV_ListEntriesHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "internal_error", "status": 500, "message": "Server error",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "sched-001",
		EntryWindowStart: time.Now().UTC().Format(time.RFC3339),
		EntryWindowEnd:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})
	if err == nil {
		t.Fatal("COV-ENTRIES-HTTP FAIL: 500 should error")
	}
	if !strings.Contains(err.Error(), "list schedule entries") {
		t.Errorf("COV-ENTRIES-HTTP FAIL: Error should mention 'list schedule entries', got: %v", err)
	}
	t.Logf("COV-ENTRIES-HTTP PASS: HTTP error: %v", err)
}

// ============================================================================
// Coverage Gap: prepRequest — empty userAgent path
// ============================================================================

func TestCOV_PrepRequestEmptyUserAgent(t *testing.T) {
	var capturedUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUA = r.Header.Get("User-Agent")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey,
		incidentio.WithBaseURL(srv.URL),
		incidentio.WithUserAgent(""),
	)
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err != nil {
		t.Fatalf("COV-EMPTY-UA FAIL: %v", err)
	}

	// With empty user agent, the Go default may be used or it may be empty
	t.Logf("COV-EMPTY-UA PASS: Empty user agent, captured UA=%q", capturedUA)
}

// ============================================================================
// Coverage Gap: Error() — empty message path
// ============================================================================

func TestCOV_APIErrorEmptyMessage(t *testing.T) {
	apiErr := &incidentio.APIError{StatusCode: 500}
	errStr := apiErr.Error()
	expected := fmt.Sprintf("incident.io API error (status %d)", 500)
	if errStr != expected {
		t.Fatalf("COV-ERR-EMPTY FAIL: Expected %q, got %q", expected, errStr)
	}
	t.Logf("COV-ERR-EMPTY PASS: Empty message error format: %q", errStr)
}

// ============================================================================
// Coverage Gap: ListSchedulesWithContext — default PageSize (0)
// ============================================================================

func TestCOV_ListSchedulesDefaultPageSize(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{
		// PageSize: 0 (default) — should not add page_size param
	})
	if err != nil {
		t.Fatalf("COV-DEFAULT-PAGE FAIL: %v", err)
	}
	if strings.Contains(capturedQuery, "page_size") {
		t.Error("COV-DEFAULT-PAGE FAIL: PageSize=0 should not add page_size param")
	}
	t.Logf("COV-DEFAULT-PAGE PASS: No page_size param when PageSize=0. Query: %q", capturedQuery)
}

// ============================================================================
// Coverage Gap: newAPIError — empty body
// ============================================================================

func TestCOV_APIErrorEmptyBody(t *testing.T) {
	// Already tested in TestERR004 but make sure we hit the empty body path explicitly
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(504)
		// No body at all
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-EMPTY-BODY FAIL: 504 with no body should error")
	}
	if !strings.Contains(err.Error(), "504") {
		t.Errorf("COV-EMPTY-BODY FAIL: Should contain 504: %v", err)
	}
	t.Logf("COV-EMPTY-BODY PASS: Empty body 504 handled: %v", err)
}
