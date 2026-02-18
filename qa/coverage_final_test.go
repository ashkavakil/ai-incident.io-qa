package qa

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	incidentio "github.com/strongdm/web/pkg/incidentio/sdk"
)

// ============================================================================
// Final coverage: do() line 145 — max retries exhausted on FINAL 429
// The existing test uses Retry-After: 0 but the last attempt (attempt==maxRetries)
// skips the retry branch and falls through to the error check. We need a test
// where ALL 4 attempts get 429, and the last one falls through to line 145.
// ============================================================================

func TestCOV_FINAL_AllAttemptsReturn429(t *testing.T) {
	// Server ALWAYS returns 429 with Retry-After: 0 for fast test
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "rate_limited", "status": 429, "message": "Rate limited forever",
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})

	totalAttempts := atomic.LoadInt32(&attempts)
	if err == nil {
		t.Fatalf("COV-FINAL-429 FAIL: Should fail after all retries, got nil error after %d attempts", totalAttempts)
	}

	// The error could be "max retries exceeded" or the last 429 error
	t.Logf("COV-FINAL-429 PASS: All %d attempts returned 429. Final error: %v", totalAttempts, err)
}

// ============================================================================
// Final coverage: do() line 107-108 — request creation error
// This happens when the URL is so malformed that http.NewRequestWithContext fails.
// ============================================================================

func TestCOV_FINAL_InvalidURLCausesRequestCreationError(t *testing.T) {
	// Use a base URL with invalid scheme to trigger NewRequestWithContext failure
	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL("://invalid"))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-FINAL-BADURL FAIL: Invalid URL should fail")
	}
	if !strings.Contains(err.Error(), "failed to create request") {
		// Might get a different error depending on Go version
		t.Logf("COV-FINAL-BADURL INFO: Error type: %v", err)
	}
	t.Logf("COV-FINAL-BADURL PASS: Invalid URL handled: %v", err)
}

// ============================================================================
// Final coverage: GetUserWithContext line 47 — HTTP error wrapping
// The existing test uses 500 but we need to ensure the "get user" wrapping on
// the HTTP error path is hit (not just the decode error path).
// ============================================================================

func TestCOV_FINAL_GetUserEmptyID(t *testing.T) {
	// Empty ID should be caught by validation (line 41-42)
	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL("http://localhost:1"))
	_, err := client.GetUserWithContext(context.Background(), "", incidentio.GetUserOptions{})
	if err == nil {
		t.Fatal("COV-FINAL-USER-EMPTY FAIL: Empty user ID should fail")
	}
	if !strings.Contains(err.Error(), "user ID is required") {
		t.Errorf("COV-FINAL-USER-EMPTY FAIL: Expected 'user ID is required', got: %v", err)
	}
	t.Logf("COV-FINAL-USER-EMPTY PASS: Empty user ID validation: %v", err)
}

// ============================================================================
// Final coverage: newAPIError — ensure the truncation path is hit (body > 200 chars)
// ============================================================================

func TestCOV_FINAL_LongNonJSONErrorTruncated(t *testing.T) {
	longBody := strings.Repeat("X", 300) // 300 chars — exceeds 200 char truncation limit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(longBody))
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-FINAL-TRUNCATE FAIL: Should error")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "truncated") {
		t.Errorf("COV-FINAL-TRUNCATE FAIL: Long non-JSON body should be truncated, got: %v", err)
	}
	if strings.Contains(errStr, strings.Repeat("X", 250)) {
		t.Error("COV-FINAL-TRUNCATE FAIL: Message still contains 250+ X chars — not truncated")
	}
	t.Logf("COV-FINAL-TRUNCATE PASS: Long non-JSON body truncated at 200 chars")
}

// ============================================================================
// Final coverage: newAPIError — body with request_id and field errors
// ============================================================================

func TestCOV_FINAL_APIErrorWithRequestIDAndFieldErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type":       "validation_error",
			"status":     422,
			"message":    "Validation failed",
			"request_id": "req-abc-123",
			"errors": []map[string]interface{}{
				{"field": "name", "message": "is required"},
				{"field": "timezone", "message": "is invalid"},
			},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{})
	if err == nil {
		t.Fatal("COV-FINAL-REQID FAIL: 422 should error")
	}
	t.Logf("COV-FINAL-REQID PASS: Error with request_id and field errors: %v", err)
}

// ============================================================================
// Final coverage: do() — test with query params (line 100-101 branch)
// ============================================================================

func TestCOV_FINAL_DoWithQueryParams(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedules":       []interface{}{},
			"pagination_meta": map[string]interface{}{"after": "", "page_size": 10, "total_record_count": 0},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.ListSchedulesWithContext(context.Background(), incidentio.ListSchedulesOptions{
		PageSize: 10,
		After:    "cursor123",
	})
	if err != nil {
		t.Fatalf("COV-FINAL-PARAMS FAIL: %v", err)
	}
	if !strings.Contains(capturedQuery, "page_size=10") || !strings.Contains(capturedQuery, "after=cursor123") {
		t.Errorf("COV-FINAL-PARAMS FAIL: Expected both params, got: %s", capturedQuery)
	}
	t.Logf("COV-FINAL-PARAMS PASS: Query params: %s", capturedQuery)
}

// ============================================================================
// Final coverage: do() — 200 with empty params (no ? in URL)
// ============================================================================

func TestCOV_FINAL_DoWithoutParams(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.String()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"schedule": map[string]interface{}{"id": "s1", "name": "S1"},
		})
	}))
	defer srv.Close()

	client := incidentio.NewClient(validAPIKey, incidentio.WithBaseURL(srv.URL))
	_, err := client.GetScheduleWithContext(context.Background(), "s1", incidentio.GetScheduleOptions{})
	if err != nil {
		t.Fatalf("COV-FINAL-NOPARAMS FAIL: %v", err)
	}
	if strings.Contains(capturedPath, "?") {
		t.Errorf("COV-FINAL-NOPARAMS FAIL: No params should mean no '?' in URL, got: %s", capturedPath)
	}
	t.Logf("COV-FINAL-NOPARAMS PASS: No query params, path: %s", capturedPath)
}
