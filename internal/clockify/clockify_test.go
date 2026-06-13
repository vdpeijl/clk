package clockify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestClient points a Client at an httptest server instead of the live API.
func newTestClient(srv *httptest.Server, workspaceID string) *Client {
	c := New("test-key", workspaceID)
	c.baseURL = srv.URL
	return c
}

func TestWorkspacesRequestShaping(t *testing.T) {
	var gotPath, gotMethod, gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotKey = r.URL.Path, r.Method, r.Header.Get("X-Api-Key")
		_, _ = io.WriteString(w, `[{"id":"w1","name":"Acme"}]`)
	}))
	defer srv.Close()

	ws, err := newTestClient(srv, "").Workspaces(context.Background())
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/workspaces" {
		t.Errorf("path = %s, want /workspaces", gotPath)
	}
	if gotKey != "test-key" {
		t.Errorf("X-Api-Key = %q, want test-key", gotKey)
	}
	if len(ws) != 1 || ws[0].ID != "w1" || ws[0].Name != "Acme" {
		t.Errorf("workspaces = %+v, want one Acme/w1", ws)
	}
}

func TestProjectsRequestShaping(t *testing.T) {
	var gotPath, gotMethod string
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotQuery = r.URL.Path, r.Method, r.URL.RawQuery
		_, _ = io.WriteString(w, `[{"id":"p1","name":"Internal"},{"id":"p2","name":"Client"}]`)
	}))
	defer srv.Close()

	projects, err := newTestClient(srv, "ws-1").Projects(context.Background())
	if err != nil {
		t.Fatalf("Projects: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/workspaces/ws-1/projects" {
		t.Errorf("path = %s, want /workspaces/ws-1/projects", gotPath)
	}
	if gotQuery == "" {
		t.Errorf("expected pagination query params, got none")
	}
	if len(projects) != 2 || projects[1].ID != "p2" {
		t.Errorf("projects = %+v, want two with p2", projects)
	}
}

func TestProjectsWithoutWorkspaceErrors(t *testing.T) {
	if _, err := New("k", "").Projects(context.Background()); err == nil {
		t.Fatal("expected error when no workspace is selected")
	}
}

func TestTasksRequestShaping(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = io.WriteString(w, `[{"id":"t1","name":"Build"}]`)
	}))
	defer srv.Close()

	tasks, err := newTestClient(srv, "ws-1").Tasks(context.Background(), "p1")
	if err != nil {
		t.Fatalf("Tasks: %v", err)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/workspaces/ws-1/projects/p1/tasks" {
		t.Errorf("path = %s, want /workspaces/ws-1/projects/p1/tasks", gotPath)
	}
	if len(tasks) != 1 || tasks[0].Name != "Build" {
		t.Errorf("tasks = %+v, want one Build", tasks)
	}
}

func TestCreateTimeEntryRequestShaping(t *testing.T) {
	var gotPath, gotMethod, gotContentType string
	var gotBody timeEntryBody
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod, gotContentType = r.URL.Path, r.Method, r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"e1","description":"work"}`)
	}))
	defer srv.Close()

	entry, err := newTestClient(srv, "ws-1").CreateTimeEntry(context.Background(), NewTimeEntry{
		Start:       time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 6, 13, 10, 30, 0, 0, time.UTC),
		Description: "work",
		ProjectID:   "p1",
		TaskID:      "t1",
		Billable:    true,
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	if gotPath != "/workspaces/ws-1/time-entries" {
		t.Errorf("path = %s, want /workspaces/ws-1/time-entries", gotPath)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody.Start != "2026-06-13T09:00:00Z" || gotBody.End != "2026-06-13T10:30:00Z" {
		t.Errorf("body times = %s/%s, want RFC3339 UTC", gotBody.Start, gotBody.End)
	}
	if gotBody.ProjectID != "p1" || gotBody.TaskID != "t1" || !gotBody.Billable {
		t.Errorf("body = %+v, want p1/t1/billable", gotBody)
	}
	if entry.ID != "e1" {
		t.Errorf("entry id = %q, want e1", entry.ID)
	}
}

func TestCreateTimeEntryOmitsEmptyTask(t *testing.T) {
	var raw map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Errorf("decode body: %v", err)
		}
		_, _ = io.WriteString(w, `{"id":"e2"}`)
	}))
	defer srv.Close()

	_, err := newTestClient(srv, "ws-1").CreateTimeEntry(context.Background(), NewTimeEntry{
		Start:     time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		ProjectID: "p1",
	})
	if err != nil {
		t.Fatalf("CreateTimeEntry: %v", err)
	}
	if _, present := raw["taskId"]; present {
		t.Errorf("taskId should be omitted when empty, body = %v", raw)
	}
}

func TestUpdateTimeEntryRequestShaping(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_, _ = io.WriteString(w, `{"id":"e1","description":"updated"}`)
	}))
	defer srv.Close()

	entry, err := newTestClient(srv, "ws-1").UpdateTimeEntry(context.Background(), "e1", NewTimeEntry{
		Start:       time.Date(2026, 6, 13, 9, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC),
		Description: "updated",
		ProjectID:   "p1",
	})
	if err != nil {
		t.Fatalf("UpdateTimeEntry: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("method = %s, want PUT", gotMethod)
	}
	if gotPath != "/workspaces/ws-1/time-entries/e1" {
		t.Errorf("path = %s, want /workspaces/ws-1/time-entries/e1", gotPath)
	}
	if entry.Description != "updated" {
		t.Errorf("description = %q, want updated", entry.Description)
	}
}

func TestDeleteTimeEntryRequestShaping(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	if err := newTestClient(srv, "ws-1").DeleteTimeEntry(context.Background(), "e1"); err != nil {
		t.Fatalf("DeleteTimeEntry: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", gotMethod)
	}
	if gotPath != "/workspaces/ws-1/time-entries/e1" {
		t.Errorf("path = %s, want /workspaces/ws-1/time-entries/e1", gotPath)
	}
}

func TestUnauthorizedMapsToClearError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := newTestClient(srv, "").Workspaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
