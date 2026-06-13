// Package clockify is an HTTP client for the Clockify REST API.
package clockify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/workspaces", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("invalid API key (HTTP 401)")
		}
		return nil, fmt.Errorf("list workspaces: HTTP %d: %s", resp.StatusCode, body)
	}

	var workspaces []Workspace
	if err := json.NewDecoder(resp.Body).Decode(&workspaces); err != nil {
		return nil, fmt.Errorf("decode workspaces: %w", err)
	}
	return workspaces, nil
}
