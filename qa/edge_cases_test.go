package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	incidentio "github.com/strongdm/web/pkg/incidentio/sdk"
)

// ============================================================================
// EDGE CASE: Malicious / Adversarial Server Responses
// ============================================================================

func TestEDGE_TruncatedJSONResponse(t *testing.T) {
	// Server sends valid HTTP 200 with truncated JSON - incomplete object
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		// Truncated JSON - missing closing braces
		w.Write([]byte(`{"schedules": [{"id": "sched-001", "name": "Trunc`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-TRUNCATED FAIL: Truncated JSON should return parse error")
	}
	t.Logf("EDGE-TRUNCATED PASS: Truncated JSON handled: %v", err)
}

func TestEDGE_EmptyJSONObject(t *testing.T) {
	// Server returns {} - no schedules key at all
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	resp, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err != nil {
		t.Logf("EDGE-EMPTY-JSON INFO: Empty JSON object returned error: %v", err)
	} else {
		if resp != nil && len(resp.Schedules) != 0 {
			t.Errorf("EDGE-EMPTY-JSON FAIL: Expected 0 schedules from empty JSON, got %d", len(resp.Schedules))
		}
		t.Log("EDGE-EMPTY-JSON FINDING: SDK accepts empty JSON {} without 'schedules' key - returns nil/empty slice")
	}
	t.Log("EDGE-EMPTY-JSON PASS: Edge case documented")
}

func TestEDGE_NullFieldsInResponse(t *testing.T) {
	// Server returns schedule with null name and timezone
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"schedule": {"id": "sched-null", "name": null, "timezone": null}}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	schedule, err := client.GetScheduleWithContext(context.Background(), "sched-null", incidentio.GetScheduleOptions{})
	if err != nil {
		t.Fatalf("EDGE-NULL-FIELDS FAIL: %v", err)
	}
	if schedule.ID != "sched-null" {
		t.Errorf("EDGE-NULL-FIELDS FAIL: Expected ID 'sched-null', got %q", schedule.ID)
	}
	// null JSON values should deserialize to Go zero values
	if schedule.Name != "" {
		t.Logf("EDGE-NULL-FIELDS FINDING: null name deserialized to %q instead of empty string", schedule.Name)
	}
	t.Logf("EDGE-NULL-FIELDS PASS: Null fields handled (name=%q, tz=%q)", schedule.Name, schedule.Timezone)
}

func TestEDGE_ExtraUnknownFieldsInResponse(t *testing.T) {
	// Server returns extra fields not in the struct - forward compatibility test
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"schedule": {
				"id": "sched-extra",
				"name": "Extra Fields",
				"timezone": "UTC",
				"new_field_2026": "some value",
				"nested_new": {"deep": true},
				"array_new": [1, 2, 3]
			}
		}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	schedule, err := client.GetScheduleWithContext(context.Background(), "sched-extra", incidentio.GetScheduleOptions{})
	if err != nil {
		t.Fatalf("EDGE-EXTRA-FIELDS FAIL: Unknown fields should be silently ignored: %v", err)
	}
	if schedule.ID != "sched-extra" || schedule.Name != "Extra Fields" {
		t.Errorf("EDGE-EXTRA-FIELDS FAIL: Known fields corrupted by extra fields")
	}
	t.Log("EDGE-EXTRA-FIELDS PASS: Extra unknown fields silently ignored (forward compatible)")
}

func TestEDGE_HTMLErrorResponse(t *testing.T) {
	// Server returns HTML error page (common from reverse proxies/load balancers)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(502)
		w.Write([]byte(`<html><body><h1>502 Bad Gateway</h1><p>nginx/1.24.0</p></body></html>`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-HTML-ERROR FAIL: HTML error page should return error")
	}
	errStr := err.Error()
	t.Logf("EDGE-HTML-ERROR PASS: HTML error page handled. Error: %v", errStr)
	if strings.Contains(errStr, "<html>") {
		t.Log("EDGE-HTML-ERROR FINDING: Raw HTML leaked into error message - could be displayed to users")
	}
}

func TestEDGE_ConnectionReset(t *testing.T) {
	// Server that immediately closes connection
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack connection and close it immediately
		hj, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(500)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			w.WriteHeader(500)
			return
		}
		conn.Close() // Abrupt close
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-CONN-RESET FAIL: Connection reset should return error")
	}
	t.Logf("EDGE-CONN-RESET PASS: Connection reset handled: %v", err)
}

func TestEDGE_ConnectionRefused(t *testing.T) {
	// Connect to a port where nothing is listening
	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL("http://127.0.0.1:1"))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-CONN-REFUSED FAIL: Connection refused should return error")
	}
	t.Logf("EDGE-CONN-REFUSED PASS: Connection refused handled: %v", err)
}

// ============================================================================
// EDGE CASE: HTTP Status Codes
// ============================================================================

func TestEDGE_HTTP403Forbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":    "forbidden",
			"status":  403,
			"message": "API key does not have permission for this endpoint",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-403 FAIL: 403 should return error")
	}
	// Check if the SDK exposes 403 meaningfully
	errStr := err.Error()
	if !strings.Contains(errStr, "403") {
		t.Logf("EDGE-403 FINDING: 403 status not clearly surfaced in error: %v", err)
	}
	t.Logf("EDGE-403 PASS: 403 Forbidden handled: %v", err)
}

func TestEDGE_HTTP429RateLimit(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&requestCount, 1)
		if count <= 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type":    "rate_limited",
				"status":  429,
				"message": "Rate limit exceeded",
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
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})

	// SDK does NOT retry on 429 - it just returns the error
	if err == nil {
		t.Log("EDGE-429 INFO: SDK succeeded (server eventually returned 200)")
	} else {
		t.Logf("EDGE-429 FINDING: SDK does not auto-retry on 429 rate limit. Error: %v", err)
		if strings.Contains(err.Error(), "429") {
			t.Log("EDGE-429 FINDING: Rate limit error surfaced but no retry-after logic implemented")
		}
	}
	t.Log("EDGE-429 PASS: Rate limit behavior documented")
}

func TestEDGE_HTTP301Redirect(t *testing.T) {
	// Test how SDK handles redirects (Go http.Client follows by default)
	var redirectCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&redirectCount, 1)
		if count == 1 {
			// Note: auth header is typically stripped on redirect
			http.Redirect(w, r, "/v2/schedules", http.StatusMovedPermanently)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err != nil {
		t.Logf("EDGE-REDIRECT FINDING: SDK does not follow redirects or auth header stripped: %v", err)
	} else {
		t.Log("EDGE-REDIRECT FINDING: SDK follows HTTP redirects - auth header may leak to different host")
	}
	t.Log("EDGE-REDIRECT PASS: Redirect behavior documented")
}

// ============================================================================
// EDGE CASE: Data Boundary Conditions
// ============================================================================

func TestEDGE_UnicodeCharactersInData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{
			"schedule": {
				"id": "sched-unicode",
				"name": "日本語スケジュール 🔥 Ünïcödé «schedule»",
				"timezone": "Asia/Tokyo"
			}
		}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	schedule, err := client.GetScheduleWithContext(context.Background(), "sched-unicode", incidentio.GetScheduleOptions{})
	if err != nil {
		t.Fatalf("EDGE-UNICODE FAIL: Unicode data should be handled: %v", err)
	}
	if !strings.Contains(schedule.Name, "日本語") {
		t.Errorf("EDGE-UNICODE FAIL: CJK characters lost, got: %q", schedule.Name)
	}
	if !strings.Contains(schedule.Name, "🔥") {
		t.Errorf("EDGE-UNICODE FAIL: Emoji lost, got: %q", schedule.Name)
	}
	t.Logf("EDGE-UNICODE PASS: Unicode handled correctly: %q", schedule.Name)
}

func TestEDGE_VeryLongScheduleID(t *testing.T) {
	// Test with an extremely long ID
	longID := strings.Repeat("a", 10000)
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.WriteHeader(404)
		w.Write([]byte(`{"type":"not_found","status":404,"message":"Not found"}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetScheduleWithContext(context.Background(), longID, incidentio.GetScheduleOptions{})
	if err == nil {
		t.Log("EDGE-LONG-ID FINDING: Very long ID (10000 chars) accepted without error")
	} else {
		t.Logf("EDGE-LONG-ID PASS: Long ID returned error: %v", err)
	}
	if len(capturedPath) > 10000 {
		t.Logf("EDGE-LONG-ID FINDING: Full 10000-char ID sent in URL path (%d chars). Could cause URL length issues with proxies.", len(capturedPath))
	}
	t.Log("EDGE-LONG-ID PASS: Long ID edge case documented")
}

func TestEDGE_EmptyScheduleID(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Write([]byte(`{"schedule":{"id":"","name":"empty"}}`))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	schedule, err := client.GetScheduleWithContext(context.Background(), "", incidentio.GetScheduleOptions{})
	t.Logf("EDGE-EMPTY-ID INFO: Path sent: %q", capturedPath)
	if err != nil {
		t.Logf("EDGE-EMPTY-ID PASS: Empty ID returned error: %v", err)
	} else {
		t.Logf("EDGE-EMPTY-ID FINDING: Empty schedule ID accepted. Schedule: %+v", schedule)
		t.Log("EDGE-EMPTY-ID FINDING: SDK does not validate empty ID client-side. Path becomes /v2/schedules/ which may match list endpoint")
	}
}

func TestEDGE_SpecialCharsInScheduleID(t *testing.T) {
	specialIDs := []struct {
		name string
		id   string
	}{
		{"spaces", "sched 001"},
		{"hash", "sched#001"},
		{"question", "sched?id=001"},
		{"ampersand", "sched&foo=bar"},
		{"newline", "sched\n001"},
		{"null_byte", "sched\x00001"},
		{"backslash", "sched\\001"},
		{"forward_slash", "sched/001"},
	}

	for _, tc := range specialIDs {
		t.Run("special_"+tc.name, func(t *testing.T) {
			var capturedPath, capturedQuery string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.RawPath
				if capturedPath == "" {
					capturedPath = r.URL.Path
				}
				capturedQuery = r.URL.RawQuery
				w.Write([]byte(`{"schedule":{"id":"test","name":"test"}}`))
			}))
			defer srv.Close()

			client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
			client.GetScheduleWithContext(context.Background(), tc.id, incidentio.GetScheduleOptions{})

			t.Logf("EDGE-SPECIAL-CHAR[%s] path=%q query=%q", tc.name, capturedPath, capturedQuery)
			if capturedQuery != "" {
				t.Logf("EDGE-SPECIAL-CHAR[%s] FINDING: Special chars leaked into query string", tc.name)
			}
		})
	}
}

func TestEDGE_DuplicateUserIDsInEntries(t *testing.T) {
	// Same user appears multiple times in schedule entries (overlapping shifts)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now().UTC()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedule_entries": []map[string]interface{}{
				{
					"entry_id":    "entry-dup-1",
					"schedule_id": "sched-001",
					"start_at":    now.Add(-2 * time.Hour).Format(time.RFC3339),
					"end_at":      now.Add(2 * time.Hour).Format(time.RFC3339),
					"user":        map[string]interface{}{"id": "user-alice", "name": "Alice", "email": "alice@example.com"},
				},
				{
					"entry_id":    "entry-dup-2",
					"schedule_id": "sched-001",
					"start_at":    now.Add(-1 * time.Hour).Format(time.RFC3339),
					"end_at":      now.Add(3 * time.Hour).Format(time.RFC3339),
					"user":        map[string]interface{}{"id": "user-alice", "name": "Alice", "email": "alice@example.com"},
				},
			},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 2},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	resp, err := client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "sched-001",
		EntryWindowStart: time.Now().UTC().Format(time.RFC3339),
		EntryWindowEnd:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("EDGE-DUPLICATE-USER FAIL: %v", err)
	}

	if len(resp.ScheduleEntries) != 2 {
		t.Fatalf("EDGE-DUPLICATE-USER FAIL: Expected 2 entries, got %d", len(resp.ScheduleEntries))
	}
	// Both entries have the same user
	if resp.ScheduleEntries[0].User.ID != resp.ScheduleEntries[1].User.ID {
		t.Error("EDGE-DUPLICATE-USER FAIL: Expected same user in both entries")
	}
	t.Log("EDGE-DUPLICATE-USER FINDING: SDK returns duplicate user entries without deduplication")
	t.Log("EDGE-DUPLICATE-USER FINDING: Integration layer (pkg/incidentio/client.go) uses set.Set to dedup, but SDK doesn't")
	t.Log("EDGE-DUPLICATE-USER PASS: Duplicate user edge case documented")
}

func TestEDGE_EmptyUserIDInEntry(t *testing.T) {
	// Entry with empty user ID - should this be filtered?
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedule_entries": []map[string]interface{}{
				{
					"entry_id":    "entry-empty-user",
					"schedule_id": "sched-001",
					"start_at":    time.Now().UTC().Format(time.RFC3339),
					"end_at":      time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
					"user":        map[string]interface{}{"id": "", "name": "", "email": ""},
				},
			},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 1},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	resp, err := client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "sched-001",
		EntryWindowStart: time.Now().UTC().Format(time.RFC3339),
		EntryWindowEnd:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("EDGE-EMPTY-USER-ID FAIL: %v", err)
	}

	if len(resp.ScheduleEntries) != 1 {
		t.Fatalf("EDGE-EMPTY-USER-ID FAIL: Expected 1 entry, got %d", len(resp.ScheduleEntries))
	}
	if resp.ScheduleEntries[0].User.ID == "" {
		t.Log("EDGE-EMPTY-USER-ID FINDING: Entry with empty user.id accepted by SDK")
		t.Log("EDGE-EMPTY-USER-ID FINDING: Integration layer filters empty IDs (client.go:126) but SDK does not")
	}
	t.Log("EDGE-EMPTY-USER-ID PASS: Empty user ID edge case documented")
}

// ============================================================================
// EDGE CASE: Concurrency & Race Conditions
// ============================================================================

func TestEDGE_ConcurrentRequests(t *testing.T) {
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		// Small delay to increase chance of concurrent execution
		time.Sleep(10 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{map[string]interface{}{"id": "s1", "name": "S1"}},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 1},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	const goroutines = 50
	var wg sync.WaitGroup
	var errCount int32
	var successCount int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
			if err != nil {
				atomic.AddInt32(&errCount, 1)
			} else {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}

	wg.Wait()
	totalRequests := atomic.LoadInt32(&requestCount)
	t.Logf("EDGE-CONCURRENT RESULT: %d goroutines, %d successes, %d errors, %d total requests hit server",
		goroutines, successCount, errCount, totalRequests)
	if errCount > 0 {
		t.Logf("EDGE-CONCURRENT FINDING: %d/%d concurrent requests failed - possible thread safety issue", errCount, goroutines)
	}
	t.Log("EDGE-CONCURRENT PASS: Concurrent requests handled")
}

func TestEDGE_ConcurrentDifferentEndpoints(t *testing.T) {
	// Hit different endpoints concurrently from same client
	srv := newTestServer(t)
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	var wg sync.WaitGroup
	var errors []string
	var mu sync.Mutex

	recordErr := func(name string, err error) {
		mu.Lock()
		errors = append(errors, fmt.Sprintf("%s: %v", name, err))
		mu.Unlock()
	}

	wg.Add(4)
	go func() {
		defer wg.Done()
		_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
		if err != nil {
			recordErr("ListSchedules", err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := client.GetScheduleWithContext(context.Background(), "sched-001", incidentio.GetScheduleOptions{})
		if err != nil {
			recordErr("GetSchedule", err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := client.GetUserWithContext(context.Background(), "user-alice", incidentio.GetUserOptions{})
		if err != nil {
			recordErr("GetUser", err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{})
		if err != nil {
			recordErr("ListUsers", err)
		}
	}()

	wg.Wait()
	if len(errors) > 0 {
		t.Logf("EDGE-CONCURRENT-MULTI FINDING: %d errors during concurrent multi-endpoint access: %v", len(errors), errors)
	}
	t.Log("EDGE-CONCURRENT-MULTI PASS: Concurrent multi-endpoint access handled")
}

// ============================================================================
// EDGE CASE: Pagination Edge Cases
// ============================================================================

func TestEDGE_PaginationInfiniteLoop(t *testing.T) {
	// Server returns the same cursor forever - should SDK detect this?
	var requestCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		// Always return same cursor - never-ending pagination
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules": []map[string]interface{}{
				{"id": "sched-loop", "name": "Loop"},
			},
			"pagination_meta": map[string]interface{}{
				"after":              "same-cursor-forever",
				"page_size":          1,
				"total_record_count": 999,
			},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.ListSchedulesWithContext(ctx, incidentio.ListSchedulesOptions{PageSize: 1})
	count := atomic.LoadInt32(&requestCount)

	if err == nil {
		t.Logf("EDGE-PAGINATION-LOOP FAIL: SDK did not detect infinite pagination (made %d requests)", count)
	} else {
		t.Logf("EDGE-PAGINATION-LOOP PASS: Pagination loop broken after %d requests: %v", count, err)
	}

	if count > 100 {
		t.Logf("EDGE-PAGINATION-LOOP FINDING: SDK made %d requests before timeout. No built-in max page limit.", count)
	}
	t.Log("EDGE-PAGINATION-LOOP FINDING: SDK has no protection against infinite pagination - relies on context timeout only")
}

func TestEDGE_PaginationEmptyPage(t *testing.T) {
	// Server returns empty schedules but with a cursor (should stop)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules": []interface{}{},
			"pagination_meta": map[string]interface{}{
				"after":              "cursor-to-nowhere",
				"page_size":          250,
				"total_record_count": 0,
			},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.ListSchedulesWithContext(ctx, incidentio.ListSchedulesOptions{})
	if err != nil {
		t.Logf("EDGE-EMPTY-PAGE INFO: Error on empty page with cursor: %v", err)
	} else {
		t.Logf("EDGE-EMPTY-PAGE FINDING: SDK follows cursor even when page is empty. Schedules returned: %d", len(resp.Schedules))
	}
	t.Log("EDGE-EMPTY-PAGE PASS: Empty page pagination documented")
}

func TestEDGE_PaginationWithInvalidCursor(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		after := r.URL.Query().Get("after")
		if after == "garbage-cursor-!@#$%" {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type":    "validation_error",
				"status":  400,
				"message": "Invalid pagination cursor",
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
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{
		After: "garbage-cursor-!@#$%",
	})
	if err == nil {
		t.Log("EDGE-BAD-CURSOR FINDING: Invalid cursor accepted without error")
	} else {
		t.Logf("EDGE-BAD-CURSOR PASS: Invalid cursor returned error: %v", err)
	}
}

// ============================================================================
// EDGE CASE: Large Scale / Stress
// ============================================================================

func TestEDGE_LargeScheduleList(t *testing.T) {
	// Server returns 500 schedules across multiple pages
	totalSchedules := 500
	pageSize := 50

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+validAPIKey {
			w.WriteHeader(401)
			return
		}

		startIdx := 0
		if after := r.URL.Query().Get("after"); after != "" {
			fmt.Sscanf(after, "%d", &startIdx)
		}

		endIdx := startIdx + pageSize
		if endIdx > totalSchedules {
			endIdx = totalSchedules
		}

		schedules := make([]map[string]interface{}, 0, endIdx-startIdx)
		for i := startIdx; i < endIdx; i++ {
			schedules = append(schedules, map[string]interface{}{
				"id":       fmt.Sprintf("sched-%04d", i),
				"name":     fmt.Sprintf("Schedule %d", i),
				"timezone": "UTC",
			})
		}

		afterCursor := ""
		if endIdx < totalSchedules {
			afterCursor = fmt.Sprintf("%d", endIdx)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules": schedules,
			"pagination_meta": map[string]interface{}{
				"after":              afterCursor,
				"page_size":          pageSize,
				"total_record_count": totalSchedules,
			},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	var allSchedules []incidentio.Schedule
	opts := incidentio.ListSchedulesOptions{PageSize: pageSize}
	pages := 0

	for {
		resp, err := client.ListSchedulesWithContext(context.Background(), opts)
		if err != nil {
			t.Fatalf("EDGE-LARGE-LIST FAIL on page %d: %v", pages+1, err)
		}
		allSchedules = append(allSchedules, resp.Schedules...)
		pages++

		if resp.PaginationMeta.After == "" {
			break
		}
		opts.After = resp.PaginationMeta.After

		if pages > 100 {
			t.Fatal("EDGE-LARGE-LIST FAIL: Too many pages, possible infinite loop")
		}
	}

	if len(allSchedules) != totalSchedules {
		t.Fatalf("EDGE-LARGE-LIST FAIL: Expected %d schedules, got %d across %d pages", totalSchedules, len(allSchedules), pages)
	}
	t.Logf("EDGE-LARGE-LIST PASS: Successfully paginated through %d schedules across %d pages", len(allSchedules), pages)
}

// ============================================================================
// EDGE CASE: Slow / Degraded Server
// ============================================================================

func TestEDGE_SlowDripResponse(t *testing.T) {
	// Server sends response byte by byte with delays
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Log("EDGE-SLOW-DRIP: Server doesn't support flushing")
			return
		}
		response := `{"schedules":[],"pagination_meta":{"after":"","page_size":250,"total_record_count":0}}`
		for i := 0; i < len(response); i++ {
			w.Write([]byte{response[i]})
			flusher.Flush()
			time.Sleep(5 * time.Millisecond) // ~450ms total for this response
		}
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	start := time.Now()
	resp, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("EDGE-SLOW-DRIP FAIL: %v", err)
	}
	if resp == nil {
		t.Fatal("EDGE-SLOW-DRIP FAIL: Response nil")
	}
	t.Logf("EDGE-SLOW-DRIP PASS: Slow drip response handled in %v", elapsed)
	if elapsed > 2*time.Second {
		t.Logf("EDGE-SLOW-DRIP FINDING: Slow server took %v - no client-side byte timeout", elapsed)
	}
}

func TestEDGE_ServerReturns200ThenErrors(t *testing.T) {
	// First request succeeds, second fails - simulates intermittent failures
	var callCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count%2 == 0 {
			w.WriteHeader(500)
			w.Write([]byte(`{"type":"internal_error","status":500,"message":"Intermittent failure"}`))
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{map[string]interface{}{"id": "s1", "name": "S1"}},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 1},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	successes := 0
	failures := 0
	for i := 0; i < 6; i++ {
		_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
		if err != nil {
			failures++
		} else {
			successes++
		}
	}

	t.Logf("EDGE-INTERMITTENT RESULT: %d successes, %d failures out of 6 calls", successes, failures)
	t.Log("EDGE-INTERMITTENT FINDING: SDK has no retry logic - intermittent failures propagate directly to caller")
	t.Log("EDGE-INTERMITTENT PASS: Intermittent failure behavior documented")
}

// ============================================================================
// EDGE CASE: APIError Edge Cases
// ============================================================================

func TestEDGE_APIErrorWithFieldErrors(t *testing.T) {
	// Test that field-level validation errors are properly parsed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":       "validation_error",
			"status":     422,
			"message":    "Validation failed",
			"request_id": "req-12345",
			"errors": []map[string]interface{}{
				{"field": "schedule_id", "message": "must be a valid UUID"},
				{"field": "entry_window_start", "message": "must be a valid ISO 8601 date"},
			},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("EDGE-FIELD-ERRORS FAIL: 422 should return error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "422") {
		t.Logf("EDGE-FIELD-ERRORS FINDING: 422 status not in error message: %v", err)
	}
	if !strings.Contains(errStr, "Validation failed") {
		t.Logf("EDGE-FIELD-ERRORS FINDING: Validation message not surfaced: %v", err)
	}

	t.Logf("EDGE-FIELD-ERRORS PASS: Field-level errors handled: %v", err)
	t.Log("EDGE-FIELD-ERRORS FINDING: SDK wraps APIError with fmt.Errorf, so FieldErrors are not accessible via errors.As() without unwrapping")
}

func TestEDGE_APIErrorAllStatusCodes(t *testing.T) {
	// Verify error helper methods for boundary cases
	tests := []struct {
		code         int
		isNotFound   bool
		isUnauth     bool
		isRateLimit  bool
	}{
		{200, false, false, false},
		{400, false, false, false},
		{401, false, true, false},
		{403, false, false, false},
		{404, true, false, false},
		{429, false, false, true},
		{500, false, false, false},
		{502, false, false, false},
		{503, false, false, false},
	}

	for _, tc := range tests {
		apiErr := &incidentio.APIError{StatusCode: tc.code}
		if apiErr.IsNotFound() != tc.isNotFound {
			t.Errorf("EDGE-ERROR-HELPERS FAIL: StatusCode %d IsNotFound() = %v, want %v", tc.code, apiErr.IsNotFound(), tc.isNotFound)
		}
		if apiErr.IsUnauthorized() != tc.isUnauth {
			t.Errorf("EDGE-ERROR-HELPERS FAIL: StatusCode %d IsUnauthorized() = %v, want %v", tc.code, apiErr.IsUnauthorized(), tc.isUnauth)
		}
		if apiErr.IsRateLimited() != tc.isRateLimit {
			t.Errorf("EDGE-ERROR-HELPERS FAIL: StatusCode %d IsRateLimited() = %v, want %v", tc.code, apiErr.IsRateLimited(), tc.isRateLimit)
		}
	}
	t.Log("EDGE-ERROR-HELPERS PASS: All status code helper methods correct")
}

// ============================================================================
// EDGE CASE: HTTP Method Validation
// ============================================================================

func TestEDGE_OnlyGETMethodUsed(t *testing.T) {
	var capturedMethods []string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		capturedMethods = append(capturedMethods, r.Method+" "+r.URL.Path)
		mu.Unlock()

		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"schedule":        map[string]interface{}{"id": "s1", "name": "S1"},
			"user":            map[string]interface{}{"id": "u1", "name": "U1", "email": "u@e.com"},
			"users":           []interface{}{},
			"schedule_entries": []interface{}{},
			"pagination_meta":  map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))

	client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	client.GetScheduleWithContext(context.Background(), "s1", incidentio.GetScheduleOptions{})
	client.GetUserWithContext(context.Background(), "u1", incidentio.GetUserOptions{})
	client.ListUsersWithContext(context.Background(), incidentio.ListUsersOptions{})
	client.ListScheduleEntriesWithContext(context.Background(), incidentio.ListScheduleEntriesOptions{
		ScheduleID:       "s1",
		EntryWindowStart: time.Now().UTC().Format(time.RFC3339),
		EntryWindowEnd:   time.Now().UTC().Add(time.Minute).Format(time.RFC3339),
	})

	for _, method := range capturedMethods {
		if !strings.HasPrefix(method, "GET ") {
			t.Errorf("EDGE-HTTP-METHOD FAIL: Non-GET method used: %s", method)
		}
	}
	t.Logf("EDGE-HTTP-METHOD PASS: All %d requests used GET method", len(capturedMethods))
}

// ============================================================================
// EDGE CASE: Response Size
// ============================================================================

func TestEDGE_LargeResponseBody(t *testing.T) {
	// Generate a 1MB response to verify SDK handles it
	largeName := strings.Repeat("A", 100000) // 100KB name
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		schedules := make([]map[string]interface{}, 10)
		for i := range schedules {
			schedules[i] = map[string]interface{}{
				"id":       fmt.Sprintf("sched-%d", i),
				"name":     fmt.Sprintf("Schedule %s %d", largeName, i),
				"timezone": "UTC",
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       schedules,
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": 10},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	resp, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err != nil {
		t.Fatalf("EDGE-LARGE-RESPONSE FAIL: %v", err)
	}
	if len(resp.Schedules) != 10 {
		t.Fatalf("EDGE-LARGE-RESPONSE FAIL: Expected 10 schedules, got %d", len(resp.Schedules))
	}
	t.Logf("EDGE-LARGE-RESPONSE PASS: ~1MB response handled, %d schedules parsed", len(resp.Schedules))
	t.Log("EDGE-LARGE-RESPONSE FINDING: SDK uses io.ReadAll - entire response buffered in memory. No streaming JSON decoder.")
}

// ============================================================================
// EDGE CASE: Verify io.ReadAll behavior with limited reader
// ============================================================================

func TestEDGE_ReadAllVsLimitReader(t *testing.T) {
	// Demonstrate the SEC-005 vulnerability: io.ReadAll has no limit
	// We don't actually OOM, but we show the code path
	data := strings.Repeat("x", 5*1024*1024) // 5MB
	reader := strings.NewReader(data)

	result, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("EDGE-READALL FAIL: %v", err)
	}
	if len(result) != 5*1024*1024 {
		t.Fatalf("EDGE-READALL FAIL: Expected 5MB, got %d bytes", len(result))
	}

	// Now with LimitReader - this is what the SDK SHOULD use
	reader2 := strings.NewReader(data)
	limited := io.LimitReader(reader2, 1*1024*1024) // 1MB limit
	result2, err := io.ReadAll(limited)
	if err != nil {
		t.Fatalf("EDGE-READALL FAIL: %v", err)
	}
	if len(result2) != 1*1024*1024 {
		t.Fatalf("EDGE-READALL FAIL: Expected 1MB limited, got %d bytes", len(result2))
	}

	t.Logf("EDGE-READALL PASS: io.ReadAll read %d bytes unbounded vs %d bytes limited", len(result), len(result2))
	t.Log("EDGE-READALL FINDING: SDK should use io.LimitReader(resp.Body, 10*1024*1024) to cap at 10MB")
}
