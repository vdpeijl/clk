// Package clockify is an HTTP client for the Clockify REST API. It owns request
// shaping and transport only — auth header, verbs, paths, and JSON bodies — and
// contains no planning logic, so it can be exercised end-to-end against an
// httptest server.
package clockify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Client communicates with the Clockify REST API.
type Client struct {
	apiKey      string
	workspaceID string
	httpClient  *http.Client
	baseURL     string
}

// New creates a Clockify client.
func New(apiKey, workspaceID string) *Client {
	return &Client{
		apiKey:      apiKey,
		workspaceID: workspaceID,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		baseURL:     "https://api.clockify.me/api/v1",
	}
}

// Workspace is a Clockify workspace the authenticated user belongs to.
type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Workspaces returns the workspaces the API key has access to. It is the basis
// for workspace selection during `clk auth login`.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	var workspaces []Workspace
	if err := c.do(ctx, http.MethodGet, "/workspaces", nil, &workspaces); err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	return workspaces, nil
}

// Project is a Clockify project within the pinned workspace. It is the target a
// local project token maps to.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// pageSize is the per-page count requested when paging through list endpoints.
// Clockify caps it at 5000; one page covers any realistic workspace.
const pageSize = 5000

// Projects returns every project in the pinned workspace, following pagination
// until a short page signals the end. It is the candidate list for the
// prompt-once fuzzy pick and `clk link`.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	if c.workspaceID == "" {
		return nil, fmt.Errorf("list projects: no workspace selected")
	}

	var all []Project
	for page := 1; ; page++ {
		path := fmt.Sprintf("/workspaces/%s/projects?page=%d&page-size=%d",
			url.PathEscape(c.workspaceID), page, pageSize)
		var batch []Project
		if err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		all = append(all, batch...)
		if len(batch) < pageSize {
			return all, nil
		}
	}
}

// Task is a task within a Clockify project. A mapping may optionally pin a task
// in addition to the project.
type Task struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Tasks returns the tasks belonging to a project in the pinned workspace.
func (c *Client) Tasks(ctx context.Context, projectID string) ([]Task, error) {
	if c.workspaceID == "" {
		return nil, fmt.Errorf("list tasks: no workspace selected")
	}

	var all []Task
	for page := 1; ; page++ {
		path := fmt.Sprintf("/workspaces/%s/projects/%s/tasks?page=%d&page-size=%d",
			url.PathEscape(c.workspaceID), url.PathEscape(projectID), page, pageSize)
		var batch []Task
		if err := c.do(ctx, http.MethodGet, path, nil, &batch); err != nil {
			return nil, fmt.Errorf("list tasks: %w", err)
		}
		all = append(all, batch...)
		if len(batch) < pageSize {
			return all, nil
		}
	}
}

// TimeEntry is a Clockify time entry as returned by create and update.
type TimeEntry struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

// NewTimeEntry is the payload for creating or updating a time entry. Start and
// End are serialized as RFC3339 UTC timestamps; an empty TaskID is omitted so
// Clockify does not reject a blank task id.
type NewTimeEntry struct {
	Start       time.Time
	End         time.Time
	Description string
	ProjectID   string
	TaskID      string
	Billable    bool
}

// timeEntryBody is the wire form of NewTimeEntry, with task id omitted when
// empty and timestamps formatted as Clockify expects.
type timeEntryBody struct {
	Start       string `json:"start"`
	End         string `json:"end"`
	Description string `json:"description"`
	ProjectID   string `json:"projectId"`
	TaskID      string `json:"taskId,omitempty"`
	Billable    bool   `json:"billable"`
}

func (n NewTimeEntry) body() timeEntryBody {
	return timeEntryBody{
		Start:       n.Start.UTC().Format(time.RFC3339),
		End:         n.End.UTC().Format(time.RFC3339),
		Description: n.Description,
		ProjectID:   n.ProjectID,
		TaskID:      n.TaskID,
		Billable:    n.Billable,
	}
}

// CreateTimeEntry creates a time entry in the pinned workspace and returns it.
func (c *Client) CreateTimeEntry(ctx context.Context, e NewTimeEntry) (TimeEntry, error) {
	if c.workspaceID == "" {
		return TimeEntry{}, fmt.Errorf("create time entry: no workspace selected")
	}
	path := fmt.Sprintf("/workspaces/%s/time-entries", url.PathEscape(c.workspaceID))
	var out TimeEntry
	if err := c.do(ctx, http.MethodPost, path, e.body(), &out); err != nil {
		return TimeEntry{}, fmt.Errorf("create time entry: %w", err)
	}
	return out, nil
}

// UpdateTimeEntry replaces an existing time entry and returns the updated form.
func (c *Client) UpdateTimeEntry(ctx context.Context, id string, e NewTimeEntry) (TimeEntry, error) {
	if c.workspaceID == "" {
		return TimeEntry{}, fmt.Errorf("update time entry: no workspace selected")
	}
	path := fmt.Sprintf("/workspaces/%s/time-entries/%s",
		url.PathEscape(c.workspaceID), url.PathEscape(id))
	var out TimeEntry
	if err := c.do(ctx, http.MethodPut, path, e.body(), &out); err != nil {
		return TimeEntry{}, fmt.Errorf("update time entry: %w", err)
	}
	return out, nil
}

// DeleteTimeEntry removes a time entry. It is the transport behind `clk unpush`;
// clk never deletes implicitly.
func (c *Client) DeleteTimeEntry(ctx context.Context, id string) error {
	if c.workspaceID == "" {
		return fmt.Errorf("delete time entry: no workspace selected")
	}
	path := fmt.Sprintf("/workspaces/%s/time-entries/%s",
		url.PathEscape(c.workspaceID), url.PathEscape(id))
	if err := c.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("delete time entry: %w", err)
	}
	return nil
}

// do performs an authenticated request. A non-nil body is JSON-encoded; a
// non-nil out is JSON-decoded from a successful response. A nil out (e.g.
// DELETE) discards the body. Non-2xx responses become errors, with 401 mapped
// to a clear "invalid API key" message.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("invalid API key (HTTP 401)")
		}
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return fmt.Errorf("HTTP %s: %s", strconv.Itoa(resp.StatusCode), errBody)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
