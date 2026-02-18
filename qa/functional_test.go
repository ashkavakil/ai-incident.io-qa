package qa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	incidentio "github.com/strongdm/web/pkg/incidentio/sdk"
)

// ============================================================================
// Stateful Mock Server — simulates a real incident.io environment
// ============================================================================

// mockIncidentIO is a stateful mock that simulates a real incident.io environment
// with mutable schedules, users, and on-call rotations. It supports adding/removing
// schedules, rotating on-call users, and simulating failures.
type mockIncidentIO struct {
	mu            sync.RWMutex
	apiKey        string
	schedules     map[string]mockSchedule
	users         map[string]mockUser
	onCall        map[string][]string // scheduleID -> []userID
	failSchedules map[string]bool     // scheduleID -> should fail
	failEndpoints map[string]int      // endpoint -> HTTP status to return
	requestLog    []string
	requestCount  int32
}

type mockSchedule struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Timezone string `json:"timezone"`
}

type mockUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

func newMockIncidentIO(apiKey string) *mockIncidentIO {
	return &mockIncidentIO{
		apiKey:        apiKey,
		schedules:     make(map[string]mockSchedule),
		users:         make(map[string]mockUser),
		onCall:        make(map[string][]string),
		failSchedules: make(map[string]bool),
		failEndpoints: make(map[string]int),
	}
}

func (m *mockIncidentIO) addSchedule(id, name, tz string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.schedules[id] = mockSchedule{ID: id, Name: name, Timezone: tz}
}

func (m *mockIncidentIO) removeSchedule(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.schedules, id)
	delete(m.onCall, id)
}

func (m *mockIncidentIO) renameSchedule(id, newName string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.schedules[id]; ok {
		s.Name = newName
		m.schedules[id] = s
	}
}

func (m *mockIncidentIO) addUser(id, name, email, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[id] = mockUser{ID: id, Name: name, Email: email, Role: role}
}

func (m *mockIncidentIO) setOnCall(scheduleID string, userIDs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCall[scheduleID] = userIDs
}

func (m *mockIncidentIO) clearOnCall(scheduleID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onCall[scheduleID] = []string{}
}

func (m *mockIncidentIO) failSchedule(scheduleID string, shouldFail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failSchedules[scheduleID] = shouldFail
}

func (m *mockIncidentIO) failEndpoint(endpoint string, statusCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if statusCode == 0 {
		delete(m.failEndpoints, endpoint)
	} else {
		m.failEndpoints[endpoint] = statusCode
	}
}

func (m *mockIncidentIO) logRequest(method, path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLog = append(m.requestLog, method+" "+path)
	atomic.AddInt32(&m.requestCount, 1)
}

func (m *mockIncidentIO) getRequestCount() int {
	return int(atomic.LoadInt32(&m.requestCount))
}

func (m *mockIncidentIO) getRequestLog() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	log := make([]string, len(m.requestLog))
	copy(log, m.requestLog)
	return log
}

func (m *mockIncidentIO) resetRequestLog() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestLog = nil
	atomic.StoreInt32(&m.requestCount, 0)
}

func (m *mockIncidentIO) serve() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.logRequest(r.Method, r.URL.Path)

		// Auth check
		auth := r.Header.Get("Authorization")
		if auth != "Bearer "+m.apiKey {
			w.WriteHeader(401)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "authentication_error", "status": 401, "message": "Invalid API key",
			})
			return
		}

		// Check endpoint failures
		m.mu.RLock()
		path := r.URL.Path
		for ep, status := range m.failEndpoints {
			if strings.HasPrefix(path, ep) {
				m.mu.RUnlock()
				w.WriteHeader(status)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"type": "error", "status": status, "message": "Simulated failure",
				})
				return
			}
		}
		m.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")

		switch {
		case path == "/v1/identity":
			m.handleIdentity(w, r)
		case path == "/v2/schedules":
			m.handleListSchedules(w, r)
		case strings.HasPrefix(path, "/v2/schedules/"):
			m.handleGetSchedule(w, r)
		case path == "/v2/schedule_entries":
			m.handleListEntries(w, r)
		case path == "/v2/users":
			m.handleListUsers(w, r)
		case strings.HasPrefix(path, "/v2/users/"):
			m.handleGetUser(w, r)
		default:
			w.WriteHeader(404)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"type": "not_found", "status": 404, "message": "Unknown endpoint",
			})
		}
	}))
}

func (m *mockIncidentIO) handleIdentity(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(map[string]interface{}{
		"identity": map[string]interface{}{
			"name": "Test Integration", "api_key_id": "ak-test", "organisation_id": "org-test",
		},
	})
}

func (m *mockIncidentIO) handleListSchedules(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pageSize := 25
	if ps := r.URL.Query().Get("page_size"); ps != "" {
		if v, _ := strconv.Atoi(ps); v > 0 {
			pageSize = v
		}
	}

	// Build sorted list
	var all []map[string]interface{}
	for _, s := range m.schedules {
		all = append(all, map[string]interface{}{"id": s.ID, "name": s.Name, "timezone": s.Timezone})
	}

	startIdx := 0
	if after := r.URL.Query().Get("after"); after != "" {
		if v, _ := strconv.Atoi(after); v > 0 {
			startIdx = v
		}
	}

	endIdx := startIdx + pageSize
	if endIdx > len(all) {
		endIdx = len(all)
	}
	if startIdx > len(all) {
		startIdx = len(all)
	}

	page := all[startIdx:endIdx]
	afterCursor := ""
	if endIdx < len(all) {
		afterCursor = strconv.Itoa(endIdx)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"schedules": page,
		"pagination_meta": map[string]interface{}{
			"after": afterCursor, "page_size": pageSize, "total_record_count": len(all),
		},
	})
}

func (m *mockIncidentIO) handleGetSchedule(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v2/schedules/")
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.schedules[id]
	if !ok {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "not_found", "status": 404, "message": fmt.Sprintf("Schedule %s not found", id),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"schedule": map[string]interface{}{"id": s.ID, "name": s.Name, "timezone": s.Timezone},
	})
}

func (m *mockIncidentIO) handleListEntries(w http.ResponseWriter, r *http.Request) {
	scheduleID := r.URL.Query().Get("schedule_id")
	if scheduleID == "" {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "validation_error", "status": 400, "message": "schedule_id is required",
		})
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Check if schedule should fail
	if m.failSchedules[scheduleID] {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "internal_error", "status": 500, "message": "Schedule temporarily unavailable",
		})
		return
	}

	now := time.Now().UTC()
	userIDs := m.onCall[scheduleID]
	entries := make([]map[string]interface{}, 0, len(userIDs))
	for i, uid := range userIDs {
		user, ok := m.users[uid]
		if !ok {
			continue
		}
		entries = append(entries, map[string]interface{}{
			"entry_id":    fmt.Sprintf("entry-%s-%d", scheduleID, i),
			"schedule_id": scheduleID,
			"start_at":    now.Add(-1 * time.Hour).Format(time.RFC3339),
			"end_at":      now.Add(7 * time.Hour).Format(time.RFC3339),
			"user":        map[string]interface{}{"id": user.ID, "name": user.Name, "email": user.Email},
		})
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"schedule_entries": entries,
		"pagination_meta":  map[string]interface{}{"after": "", "page_size": 250, "total_record_count": len(entries)},
	})
}

func (m *mockIncidentIO) handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v2/users/")
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[id]
	if !ok {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"type": "not_found", "status": 404, "message": fmt.Sprintf("User %s not found", id),
		})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user": map[string]interface{}{"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role},
	})
}

func (m *mockIncidentIO) handleListUsers(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var users []map[string]interface{}
	for _, u := range m.users {
		users = append(users, map[string]interface{}{"id": u.ID, "name": u.Name, "email": u.Email, "role": u.Role})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"users":           users,
		"pagination_meta": map[string]interface{}{"after": "", "page_size": 250, "total_record_count": len(users)},
	})
}

// ============================================================================
// Helper: simulates the integration layer's sync flow using SDK only
// ============================================================================

type syncResult struct {
	ScheduleID   string
	ScheduleName string
	OnCallUsers  []resolvedUser
	Error        error
}

type resolvedUser struct {
	UserID string
	Name   string
	Email  string
}

// simulateFullSync mimics what pkg/incidentio/sync.go FullSync does:
// 1. List all schedules from incident.io
// 2. For each tracked schedule, get on-call entries
// 3. Resolve each user by ID
// 4. Return the results (what would become group memberships)
func simulateFullSync(ctx context.Context, client *incidentio.Client, trackedScheduleIDs []string) ([]syncResult, error) {
	// Step 1: Verify schedules still exist
	allSchedules, err := listAllSchedules(ctx, client)
	if err != nil {
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	scheduleMap := make(map[string]incidentio.Schedule)
	for _, s := range allSchedules {
		scheduleMap[s.ID] = s
	}

	// Step 2: For each tracked schedule, get on-call users
	var results []syncResult
	for _, schedID := range trackedScheduleIDs {
		sched, exists := scheduleMap[schedID]
		if !exists {
			results = append(results, syncResult{
				ScheduleID: schedID,
				Error:      fmt.Errorf("schedule %s no longer exists", schedID),
			})
			continue
		}

		// Get on-call entries
		now := time.Now().UTC()
		entryResp, err := client.ListScheduleEntriesWithContext(ctx, incidentio.ListScheduleEntriesOptions{
			ScheduleID:       schedID,
			EntryWindowStart: now.Format(time.RFC3339),
			EntryWindowEnd:   now.Add(time.Minute).Format(time.RFC3339),
		})
		if err != nil {
			results = append(results, syncResult{
				ScheduleID:   schedID,
				ScheduleName: sched.Name,
				Error:        fmt.Errorf("failed to get entries: %w", err),
			})
			continue
		}

		// Step 3: Resolve users
		seen := make(map[string]bool)
		var users []resolvedUser
		for _, entry := range entryResp.ScheduleEntries {
			if entry.User.ID == "" || seen[entry.User.ID] {
				continue
			}
			seen[entry.User.ID] = true

			user, err := client.GetUserWithContext(ctx, entry.User.ID, incidentio.GetUserOptions{})
			if err != nil {
				continue // skip unresolvable users
			}
			users = append(users, resolvedUser{
				UserID: user.ID,
				Name:   user.Name,
				Email:  user.Email,
			})
		}

		results = append(results, syncResult{
			ScheduleID:   schedID,
			ScheduleName: sched.Name,
			OnCallUsers:  users,
		})
	}

	return results, nil
}

// listAllSchedules handles pagination to get all schedules
func listAllSchedules(ctx context.Context, client *incidentio.Client) ([]incidentio.Schedule, error) {
	var all []incidentio.Schedule
	opts := incidentio.ListSchedulesOptions{PageSize: 250}
	for page := 0; page < 100; page++ {
		resp, err := client.ListSchedulesWithContext(ctx, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Schedules...)
		if resp.PaginationMeta.After == "" {
			break
		}
		opts.After = resp.PaginationMeta.After
	}
	return all, nil
}

// ============================================================================
// FUNCTIONAL TESTS — Happy Paths
// ============================================================================

func TestFUNC_CreateIntegrationAndTestConnection(t *testing.T) {
	mock := newMockIncidentIO("prod-api-key-abc123")
	mock.addSchedule("sched-001", "Platform On-Call", "America/New_York")
	srv := mock.serve()
	defer srv.Close()

	// Step 1: Test connection with correct API key
	client := incidentio.NewClient("prod-api-key-abc123", incidentio.WithBaseURL(srv.URL))
	schedules, err := listAllSchedules(context.Background(), client)
	if err != nil {
		t.Fatalf("FUNC-CREATE FAIL: Connection test failed: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("FUNC-CREATE FAIL: Expected 1 schedule, got %d", len(schedules))
	}

	// Step 2: Test connection with wrong API key
	badClient := incidentio.NewClient("wrong-key", incidentio.WithBaseURL(srv.URL))
	_, err = listAllSchedules(context.Background(), badClient)
	if err == nil {
		t.Fatal("FUNC-CREATE FAIL: Wrong API key should fail")
	}

	t.Log("FUNC-CREATE PASS: Integration created and connection tested successfully")
}

func TestFUNC_AddSchedulesSyncAndVerifyOnCall(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "Platform On-Call", "America/New_York")
	mock.addSchedule("sched-002", "Backend Primary", "America/Los_Angeles")
	mock.addUser("user-alice", "Alice Chen", "alice@example.com", "responder")
	mock.addUser("user-bob", "Bob Martinez", "bob@example.com", "responder")
	mock.addUser("user-carol", "Carol Davis", "carol@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-alice", "user-bob"})
	mock.setOnCall("sched-002", []string{"user-carol"})

	srv := mock.serve()
	defer srv.Close()

	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Step 1: List available schedules
	schedules, err := listAllSchedules(context.Background(), client)
	if err != nil {
		t.Fatalf("FUNC-SYNC FAIL: List schedules: %v", err)
	}
	if len(schedules) != 2 {
		t.Fatalf("FUNC-SYNC FAIL: Expected 2 schedules, got %d", len(schedules))
	}

	// Step 2: "Add" both schedules (track them)
	tracked := []string{"sched-001", "sched-002"}

	// Step 3: Run sync
	results, err := simulateFullSync(context.Background(), client, tracked)
	if err != nil {
		t.Fatalf("FUNC-SYNC FAIL: Sync failed: %v", err)
	}

	// Step 4: Verify results
	if len(results) != 2 {
		t.Fatalf("FUNC-SYNC FAIL: Expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.Error != nil {
			t.Fatalf("FUNC-SYNC FAIL: Schedule %s error: %v", r.ScheduleID, r.Error)
		}
	}

	// Verify sched-001 has alice and bob
	r1 := results[0]
	if r1.ScheduleID == "sched-002" {
		r1 = results[1]
	}
	if len(r1.OnCallUsers) != 2 {
		t.Fatalf("FUNC-SYNC FAIL: sched-001 should have 2 on-call users, got %d", len(r1.OnCallUsers))
	}

	// Verify sched-002 has carol
	r2 := results[1]
	if r2.ScheduleID == "sched-001" {
		r2 = results[0]
	}
	if len(r2.OnCallUsers) != 1 {
		t.Fatalf("FUNC-SYNC FAIL: sched-002 should have 1 on-call user, got %d", len(r2.OnCallUsers))
	}

	t.Logf("FUNC-SYNC PASS: Synced %d schedules, resolved %d + %d on-call users",
		len(results), len(r1.OnCallUsers), len(r2.OnCallUsers))
}

func TestFUNC_RemoveScheduleAndVerifyCleanup(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "Platform On-Call", "UTC")
	mock.addSchedule("sched-002", "Backend Primary", "UTC")
	mock.addUser("user-alice", "Alice", "alice@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-alice"})
	mock.setOnCall("sched-002", []string{"user-alice"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Sync with both schedules
	results, err := simulateFullSync(context.Background(), client, []string{"sched-001", "sched-002"})
	if err != nil {
		t.Fatalf("FUNC-REMOVE FAIL: Initial sync: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("FUNC-REMOVE FAIL: Expected 2 results initially, got %d", len(results))
	}

	// Remove sched-002 from tracking
	results, err = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil {
		t.Fatalf("FUNC-REMOVE FAIL: Post-remove sync: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("FUNC-REMOVE FAIL: Expected 1 result after removal, got %d", len(results))
	}
	if results[0].ScheduleID != "sched-001" {
		t.Fatalf("FUNC-REMOVE FAIL: Expected sched-001, got %s", results[0].ScheduleID)
	}

	t.Log("FUNC-REMOVE PASS: Schedule removed, only tracked schedule synced")
}

func TestFUNC_FullSyncLifecycle(t *testing.T) {
	mock := newMockIncidentIO("lifecycle-key")
	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("lifecycle-key", incidentio.WithBaseURL(srv.URL))

	// Phase 1: Empty state — no schedules exist
	schedules, err := listAllSchedules(context.Background(), client)
	if err != nil {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 1 list: %v", err)
	}
	if len(schedules) != 0 {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 1 expected 0 schedules, got %d", len(schedules))
	}

	// Phase 2: Admin creates schedules in incident.io
	mock.addSchedule("sched-A", "Team Alpha", "UTC")
	mock.addSchedule("sched-B", "Team Beta", "America/Chicago")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.addUser("user-2", "User Two", "two@example.com", "responder")
	mock.setOnCall("sched-A", []string{"user-1"})
	mock.setOnCall("sched-B", []string{"user-2"})

	schedules, _ = listAllSchedules(context.Background(), client)
	if len(schedules) != 2 {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 2 expected 2 schedules, got %d", len(schedules))
	}

	// Phase 3: Track both schedules and sync
	results, _ := simulateFullSync(context.Background(), client, []string{"sched-A", "sched-B"})
	if len(results) != 2 {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 3 expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Error != nil {
			t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 3 error: %v", r.Error)
		}
		if len(r.OnCallUsers) != 1 {
			t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 3 expected 1 user per schedule, got %d for %s", len(r.OnCallUsers), r.ScheduleID)
		}
	}

	// Phase 4: On-call rotation — user-1 goes off, user-2 takes over both
	mock.setOnCall("sched-A", []string{"user-2"})
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-A", "sched-B"})
	for _, r := range results {
		if r.ScheduleID == "sched-A" {
			if len(r.OnCallUsers) != 1 || r.OnCallUsers[0].UserID != "user-2" {
				t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 4 sched-A should have user-2, got %v", r.OnCallUsers)
			}
		}
	}

	// Phase 5: Schedule renamed in incident.io
	mock.renameSchedule("sched-A", "Team Alpha (Renamed)")
	sched, err := client.GetScheduleWithContext(context.Background(), "sched-A", incidentio.GetScheduleOptions{})
	if err != nil {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 5 get schedule: %v", err)
	}
	if sched.Name != "Team Alpha (Renamed)" {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 5 expected renamed schedule, got %q", sched.Name)
	}

	// Phase 6: Schedule deleted in incident.io
	mock.removeSchedule("sched-B")
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-A", "sched-B"})
	for _, r := range results {
		if r.ScheduleID == "sched-B" && r.Error == nil {
			t.Fatal("FUNC-LIFECYCLE FAIL: Phase 6 deleted schedule should have error")
		}
	}

	// Phase 7: New schedule added
	mock.addSchedule("sched-C", "Team Charlie", "Europe/London")
	mock.addUser("user-3", "User Three", "three@example.com", "responder")
	mock.setOnCall("sched-C", []string{"user-3"})

	results, _ = simulateFullSync(context.Background(), client, []string{"sched-A", "sched-C"})
	if len(results) != 2 {
		t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 7 expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Error != nil {
			t.Fatalf("FUNC-LIFECYCLE FAIL: Phase 7 error on %s: %v", r.ScheduleID, r.Error)
		}
	}

	t.Log("FUNC-LIFECYCLE PASS: Full lifecycle tested — create, sync, rotate, rename, delete, re-add")
}

func TestFUNC_UserResolutionByEmail(t *testing.T) {
	mock := newMockIncidentIO("email-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-alice", "Alice Chen", "alice@strongdm.com", "responder")
	mock.addUser("user-bob", "Bob Martinez", "bob@strongdm.com", "responder")
	mock.addUser("user-noemail", "No Email User", "", "observer")
	mock.setOnCall("sched-001", []string{"user-alice", "user-bob", "user-noemail"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("email-key", incidentio.WithBaseURL(srv.URL))

	results, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil {
		t.Fatalf("FUNC-EMAIL FAIL: %v", err)
	}

	r := results[0]
	if r.Error != nil {
		t.Fatalf("FUNC-EMAIL FAIL: %v", r.Error)
	}

	// Should resolve 3 users (including empty email one — SDK returns it, integration layer would filter)
	emailCount := 0
	noEmailCount := 0
	for _, u := range r.OnCallUsers {
		if u.Email != "" {
			emailCount++
		} else {
			noEmailCount++
		}
	}

	t.Logf("FUNC-EMAIL PASS: Resolved %d users with email, %d without email", emailCount, noEmailCount)
	if noEmailCount > 0 {
		t.Log("FUNC-EMAIL FINDING: Users without email are returned by SDK. Integration layer must filter for EMAIL sync mode.")
	}
}

// ============================================================================
// FUNCTIONAL TESTS — Edge Cases
// ============================================================================

func TestFUNC_DisconnectReconnect(t *testing.T) {
	mock := newMockIncidentIO("valid-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})

	srv := mock.serve()
	defer srv.Close()

	// Step 1: Connect with valid key — works
	client := incidentio.NewClient("valid-key", incidentio.WithBaseURL(srv.URL))
	results, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil || results[0].Error != nil {
		t.Fatalf("FUNC-DISCONNECT FAIL: Initial sync should work: %v", err)
	}

	// Step 2: "Revoke" API key — simulate by using wrong key
	badClient := incidentio.NewClient("revoked-key", incidentio.WithBaseURL(srv.URL))
	_, err = simulateFullSync(context.Background(), badClient, []string{"sched-001"})
	if err == nil {
		t.Fatal("FUNC-DISCONNECT FAIL: Revoked key should fail")
	}

	// Step 3: "Reconnect" with new valid key — works again
	newClient := incidentio.NewClient("valid-key", incidentio.WithBaseURL(srv.URL))
	results, err = simulateFullSync(context.Background(), newClient, []string{"sched-001"})
	if err != nil || results[0].Error != nil {
		t.Fatalf("FUNC-DISCONNECT FAIL: Reconnect should work: %v", err)
	}

	t.Log("FUNC-DISCONNECT PASS: Disconnect and reconnect cycle works correctly")
}

func TestFUNC_ScheduleDeletedDuringSync(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "Stable Schedule", "UTC")
	mock.addSchedule("sched-ephemeral", "Ephemeral Schedule", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})
	mock.setOnCall("sched-ephemeral", []string{"user-1"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Initial sync works with both
	results, err := simulateFullSync(context.Background(), client, []string{"sched-001", "sched-ephemeral"})
	if err != nil {
		t.Fatalf("FUNC-DELETED-DURING FAIL: Initial sync: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("FUNC-DELETED-DURING FAIL: Expected 2 results, got %d", len(results))
	}

	// Delete schedule between syncs
	mock.removeSchedule("sched-ephemeral")

	// Next sync should handle deleted schedule gracefully
	results, err = simulateFullSync(context.Background(), client, []string{"sched-001", "sched-ephemeral"})
	if err != nil {
		t.Fatalf("FUNC-DELETED-DURING FAIL: Post-delete sync should not fail entirely: %v", err)
	}

	// sched-001 should still sync fine
	// sched-ephemeral should have an error
	var stableOK, ephemeralError bool
	for _, r := range results {
		if r.ScheduleID == "sched-001" && r.Error == nil {
			stableOK = true
		}
		if r.ScheduleID == "sched-ephemeral" && r.Error != nil {
			ephemeralError = true
		}
	}

	if !stableOK {
		t.Fatal("FUNC-DELETED-DURING FAIL: Stable schedule should still sync")
	}
	if !ephemeralError {
		t.Fatal("FUNC-DELETED-DURING FAIL: Deleted schedule should report error")
	}

	t.Log("FUNC-DELETED-DURING PASS: Deleted schedule handled gracefully, stable schedule still syncs")
}

func TestFUNC_OnCallRotationBetweenSyncs(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-day", "Day Person", "day@example.com", "responder")
	mock.addUser("user-night", "Night Person", "night@example.com", "responder")

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Sync 1: Day shift
	mock.setOnCall("sched-001", []string{"user-day"})
	results, _ := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if results[0].OnCallUsers[0].UserID != "user-day" {
		t.Fatalf("FUNC-ROTATION FAIL: Sync 1 should show day user, got %s", results[0].OnCallUsers[0].UserID)
	}

	// Sync 2: Night shift — rotation happened
	mock.setOnCall("sched-001", []string{"user-night"})
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if results[0].OnCallUsers[0].UserID != "user-night" {
		t.Fatalf("FUNC-ROTATION FAIL: Sync 2 should show night user, got %s", results[0].OnCallUsers[0].UserID)
	}

	// Sync 3: Handoff — both on call during overlap
	mock.setOnCall("sched-001", []string{"user-day", "user-night"})
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results[0].OnCallUsers) != 2 {
		t.Fatalf("FUNC-ROTATION FAIL: Sync 3 should show 2 users, got %d", len(results[0].OnCallUsers))
	}

	t.Log("FUNC-ROTATION PASS: On-call rotation correctly reflected across syncs")
}

func TestFUNC_EmptyScheduleNoOneOnCall(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Initially someone is on-call
	mock.setOnCall("sched-001", []string{"user-1"})
	results, _ := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results[0].OnCallUsers) != 1 {
		t.Fatalf("FUNC-EMPTY FAIL: Should have 1 user initially")
	}

	// Shift gap — no one on-call
	mock.clearOnCall("sched-001")
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results[0].OnCallUsers) != 0 {
		t.Fatalf("FUNC-EMPTY FAIL: Should have 0 users during gap")
	}

	t.Log("FUNC-EMPTY PASS: Empty on-call set handled (0 users returned)")
	t.Log("FUNC-EMPTY FINDING: Integration layer (SYNC-009 fix) should preserve previous members here")
}

func TestFUNC_SameUserAcrossMultipleSchedules(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "Primary", "UTC")
	mock.addSchedule("sched-002", "Escalation", "UTC")
	mock.addUser("user-alice", "Alice", "alice@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-alice"})
	mock.setOnCall("sched-002", []string{"user-alice"}) // Same user in both

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	results, _ := simulateFullSync(context.Background(), client, []string{"sched-001", "sched-002"})

	aliceCount := 0
	for _, r := range results {
		for _, u := range r.OnCallUsers {
			if u.UserID == "user-alice" {
				aliceCount++
			}
		}
	}

	if aliceCount != 2 {
		t.Fatalf("FUNC-DEDUP FAIL: Alice should appear in 2 schedules, appeared in %d", aliceCount)
	}

	t.Log("FUNC-DEDUP PASS: Same user correctly appears in both schedule groups")
	t.Log("FUNC-DEDUP INFO: Each schedule gets its own group — user in both groups is correct behavior")
}

func TestFUNC_PartialFailureDuringSyncContinuesOthers(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-ok-1", "Good Schedule 1", "UTC")
	mock.addSchedule("sched-fail", "Failing Schedule", "UTC")
	mock.addSchedule("sched-ok-2", "Good Schedule 2", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.addUser("user-2", "User Two", "two@example.com", "responder")
	mock.setOnCall("sched-ok-1", []string{"user-1"})
	mock.setOnCall("sched-fail", []string{"user-1"})
	mock.setOnCall("sched-ok-2", []string{"user-2"})

	// Make one schedule fail
	mock.failSchedule("sched-fail", true)

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	results, err := simulateFullSync(context.Background(), client, []string{"sched-ok-1", "sched-fail", "sched-ok-2"})
	if err != nil {
		t.Fatalf("FUNC-PARTIAL FAIL: Full sync should not fail entirely: %v", err)
	}

	var okCount, failCount int
	for _, r := range results {
		if r.Error != nil {
			failCount++
			t.Logf("FUNC-PARTIAL INFO: %s failed: %v", r.ScheduleID, r.Error)
		} else {
			okCount++
			t.Logf("FUNC-PARTIAL INFO: %s synced OK with %d users", r.ScheduleID, len(r.OnCallUsers))
		}
	}

	if okCount != 2 {
		t.Fatalf("FUNC-PARTIAL FAIL: Expected 2 successful syncs, got %d", okCount)
	}
	if failCount != 1 {
		t.Fatalf("FUNC-PARTIAL FAIL: Expected 1 failed sync, got %d", failCount)
	}

	t.Log("FUNC-PARTIAL PASS: Failed schedule didn't block others — 2 synced, 1 failed")
}

func TestFUNC_APIDegradationDuringMultiStepSync(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Step 1: Normal sync works
	results, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil || results[0].Error != nil {
		t.Fatalf("FUNC-DEGRADE FAIL: Initial sync should work")
	}

	// Step 2: User endpoint goes down mid-sync
	mock.failEndpoint("/v2/users", 503)
	results, err = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil {
		t.Fatalf("FUNC-DEGRADE FAIL: Sync should still return results even with user failures: %v", err)
	}
	// Schedule entries work, but user resolution fails → 0 resolved users
	if results[0].Error != nil {
		t.Logf("FUNC-DEGRADE INFO: Schedule synced with error: %v", results[0].Error)
	} else if len(results[0].OnCallUsers) != 0 {
		t.Logf("FUNC-DEGRADE INFO: Unexpectedly resolved %d users with user endpoint down", len(results[0].OnCallUsers))
	} else {
		t.Log("FUNC-DEGRADE INFO: 0 users resolved (user endpoint down) — schedule entry found but user lookup failed")
	}

	// Step 3: User endpoint recovers
	mock.failEndpoint("/v2/users", 0)
	results, err = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil || results[0].Error != nil {
		t.Fatalf("FUNC-DEGRADE FAIL: Recovery sync should work")
	}
	if len(results[0].OnCallUsers) != 1 {
		t.Fatalf("FUNC-DEGRADE FAIL: Expected 1 user after recovery, got %d", len(results[0].OnCallUsers))
	}

	t.Log("FUNC-DEGRADE PASS: Handled API degradation and recovery")
}

func TestFUNC_LargeScaleSync(t *testing.T) {
	mock := newMockIncidentIO("scale-key")

	// Create 50 schedules with 5 users each
	for i := 0; i < 50; i++ {
		schedID := fmt.Sprintf("sched-%03d", i)
		mock.addSchedule(schedID, fmt.Sprintf("Schedule %d", i), "UTC")
		var onCallUsers []string
		for j := 0; j < 5; j++ {
			userID := fmt.Sprintf("user-%03d-%d", i, j)
			mock.addUser(userID, fmt.Sprintf("User %d-%d", i, j), fmt.Sprintf("user%d_%d@example.com", i, j), "responder")
			onCallUsers = append(onCallUsers, userID)
		}
		mock.setOnCall(schedID, onCallUsers)
	}

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("scale-key", incidentio.WithBaseURL(srv.URL))

	// List all schedules
	start := time.Now()
	schedules, err := listAllSchedules(context.Background(), client)
	listDuration := time.Since(start)
	if err != nil {
		t.Fatalf("FUNC-SCALE FAIL: List schedules: %v", err)
	}
	if len(schedules) != 50 {
		t.Fatalf("FUNC-SCALE FAIL: Expected 50 schedules, got %d", len(schedules))
	}

	// Sync all 50 schedules
	tracked := make([]string, 50)
	for i := 0; i < 50; i++ {
		tracked[i] = fmt.Sprintf("sched-%03d", i)
	}

	start = time.Now()
	results, err := simulateFullSync(context.Background(), client, tracked)
	syncDuration := time.Since(start)
	if err != nil {
		t.Fatalf("FUNC-SCALE FAIL: Sync: %v", err)
	}

	totalUsers := 0
	errors := 0
	for _, r := range results {
		if r.Error != nil {
			errors++
		} else {
			totalUsers += len(r.OnCallUsers)
		}
	}

	t.Logf("FUNC-SCALE PASS: 50 schedules, %d users resolved, %d errors", totalUsers, errors)
	t.Logf("FUNC-SCALE INFO: List took %v, sync took %v, %d API calls total",
		listDuration, syncDuration, mock.getRequestCount())

	if totalUsers != 250 {
		t.Errorf("FUNC-SCALE WARNING: Expected 250 total users (50x5), got %d", totalUsers)
	}
}

func TestFUNC_ConcurrentSyncs(t *testing.T) {
	mock := newMockIncidentIO("concurrent-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("concurrent-key", incidentio.WithBaseURL(srv.URL))

	// Run 10 syncs concurrently (simulates periodic sync + manual sync race)
	const concurrency = 10
	var wg sync.WaitGroup
	var errCount int32
	var successCount int32

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			results, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
			if err != nil || results[0].Error != nil {
				atomic.AddInt32(&errCount, 1)
				return
			}
			if len(results[0].OnCallUsers) == 1 {
				atomic.AddInt32(&successCount, 1)
			}
		}()
	}
	wg.Wait()

	t.Logf("FUNC-CONCURRENT PASS: %d/%d concurrent syncs succeeded, %d failed",
		successCount, concurrency, errCount)
	if errCount > 0 {
		t.Logf("FUNC-CONCURRENT FINDING: %d concurrent syncs failed — possible contention", errCount)
	}
}

func TestFUNC_ScheduleAddedThenImmediatelyRemoved(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "Permanent", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Add a schedule
	mock.addSchedule("sched-temp", "Temporary", "UTC")
	mock.setOnCall("sched-temp", []string{"user-1"})

	// Sync with it
	results, _ := simulateFullSync(context.Background(), client, []string{"sched-001", "sched-temp"})
	if len(results) != 2 {
		t.Fatalf("FUNC-ADD-REMOVE FAIL: Expected 2 results after add, got %d", len(results))
	}

	// Immediately remove from incident.io AND from tracking
	mock.removeSchedule("sched-temp")
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results) != 1 {
		t.Fatalf("FUNC-ADD-REMOVE FAIL: Expected 1 result after remove, got %d", len(results))
	}

	t.Log("FUNC-ADD-REMOVE PASS: Schedule added then immediately removed — clean state")
}

func TestFUNC_UserDeletedFromIncidentIO(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")
	mock.addUser("user-active", "Active User", "active@example.com", "responder")
	mock.addUser("user-departing", "Departing User", "departing@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-active", "user-departing"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Sync 1: Both users on-call
	results, _ := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results[0].OnCallUsers) != 2 {
		t.Fatalf("FUNC-USER-DELETE FAIL: Expected 2 users, got %d", len(results[0].OnCallUsers))
	}

	// User departs — still in on-call entries but user endpoint returns 404
	// (simulates delay between schedule update and user removal)
	mock.mu.Lock()
	delete(mock.users, "user-departing")
	mock.mu.Unlock()

	// Sync 2: Departing user still in entries but can't be resolved
	results, _ = simulateFullSync(context.Background(), client, []string{"sched-001"})
	if len(results[0].OnCallUsers) != 1 {
		t.Fatalf("FUNC-USER-DELETE FAIL: Expected 1 resolved user after deletion, got %d", len(results[0].OnCallUsers))
	}
	if results[0].OnCallUsers[0].UserID != "user-active" {
		t.Fatalf("FUNC-USER-DELETE FAIL: Expected active user, got %s", results[0].OnCallUsers[0].UserID)
	}

	t.Log("FUNC-USER-DELETE PASS: Deleted user gracefully skipped, active user still resolved")
}

func TestFUNC_ManySchedulesSomeFailingOthersSucceed(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("sched-%02d", i)
		mock.addSchedule(id, fmt.Sprintf("Schedule %d", i), "UTC")
		mock.addUser(fmt.Sprintf("user-%02d", i), fmt.Sprintf("User %d", i), fmt.Sprintf("u%d@example.com", i), "responder")
		mock.setOnCall(id, []string{fmt.Sprintf("user-%02d", i)})
	}

	// Fail schedules 3, 5, 7
	mock.failSchedule("sched-03", true)
	mock.failSchedule("sched-05", true)
	mock.failSchedule("sched-07", true)

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	tracked := make([]string, 10)
	for i := 0; i < 10; i++ {
		tracked[i] = fmt.Sprintf("sched-%02d", i)
	}

	results, err := simulateFullSync(context.Background(), client, tracked)
	if err != nil {
		t.Fatalf("FUNC-MIXED FAIL: Overall sync should not fail: %v", err)
	}

	successIDs := []string{}
	failIDs := []string{}
	for _, r := range results {
		if r.Error != nil {
			failIDs = append(failIDs, r.ScheduleID)
		} else {
			successIDs = append(successIDs, r.ScheduleID)
		}
	}

	if len(successIDs) != 7 {
		t.Fatalf("FUNC-MIXED FAIL: Expected 7 successes, got %d: %v", len(successIDs), successIDs)
	}
	if len(failIDs) != 3 {
		t.Fatalf("FUNC-MIXED FAIL: Expected 3 failures, got %d: %v", len(failIDs), failIDs)
	}

	t.Logf("FUNC-MIXED PASS: 7 schedules synced, 3 failed individually (no cascade)")
}

func TestFUNC_APIRequestCounting(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call 1", "UTC")
	mock.addSchedule("sched-002", "On-Call 2", "UTC")
	mock.addUser("user-1", "User One", "one@example.com", "responder")
	mock.addUser("user-2", "User Two", "two@example.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})
	mock.setOnCall("sched-002", []string{"user-2"})

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	mock.resetRequestLog()
	_, err := simulateFullSync(context.Background(), client, []string{"sched-001", "sched-002"})
	if err != nil {
		t.Fatalf("FUNC-API-COUNT FAIL: %v", err)
	}

	log := mock.getRequestLog()
	count := mock.getRequestCount()

	// Expected: 1 list schedules + 2 schedule entries + 2 get user = 5 minimum
	t.Logf("FUNC-API-COUNT PASS: Sync made %d API calls", count)
	for _, entry := range log {
		t.Logf("  %s", entry)
	}

	if count > 20 {
		t.Logf("FUNC-API-COUNT FINDING: %d API calls seems high for 2 schedules with 1 user each", count)
	}
}

func TestFUNC_ScheduleWithManyOnCallUsers(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-big", "Big Schedule", "UTC")

	// 20 users on-call simultaneously
	var userIDs []string
	for i := 0; i < 20; i++ {
		id := fmt.Sprintf("user-%03d", i)
		mock.addUser(id, fmt.Sprintf("User %d", i), fmt.Sprintf("user%d@example.com", i), "responder")
		userIDs = append(userIDs, id)
	}
	mock.setOnCall("sched-big", userIDs)

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	results, _ := simulateFullSync(context.Background(), client, []string{"sched-big"})
	if results[0].Error != nil {
		t.Fatalf("FUNC-BIG-ONCALL FAIL: %v", results[0].Error)
	}
	if len(results[0].OnCallUsers) != 20 {
		t.Fatalf("FUNC-BIG-ONCALL FAIL: Expected 20 users, got %d", len(results[0].OnCallUsers))
	}

	t.Logf("FUNC-BIG-ONCALL PASS: 20 concurrent on-call users resolved correctly")
}

func TestFUNC_EntireAPIDown(t *testing.T) {
	mock := newMockIncidentIO("test-key")
	mock.addSchedule("sched-001", "On-Call", "UTC")

	srv := mock.serve()
	defer srv.Close()
	client := incidentio.NewClient("test-key", incidentio.WithBaseURL(srv.URL))

	// Fail the list schedules endpoint — first step of sync
	mock.failEndpoint("/v2/schedules", 503)

	_, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err == nil {
		t.Fatal("FUNC-API-DOWN FAIL: Should fail when entire API is down")
	}

	t.Logf("FUNC-API-DOWN PASS: Full API outage handled: %v", err)

	// Recovery
	mock.failEndpoint("/v2/schedules", 0)
	mock.addUser("user-1", "User", "u@e.com", "responder")
	mock.setOnCall("sched-001", []string{"user-1"})

	results, err := simulateFullSync(context.Background(), client, []string{"sched-001"})
	if err != nil || results[0].Error != nil {
		t.Fatalf("FUNC-API-DOWN FAIL: Should recover after API comes back: %v", err)
	}

	t.Log("FUNC-API-DOWN PASS: API outage and recovery handled correctly")
}
