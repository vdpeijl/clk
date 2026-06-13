// Package clockify is an HTTP client for the Clockify REST API.
package clockify

import "net/http"

// Client communicates with the Clockify REST API.
type Client struct {
	apiKey     string
	workspaceID string
	httpClient *http.Client
	baseURL    string
}

// New creates a Clockify client.
func New(apiKey, workspaceID string) *Client {
	return &Client{
		apiKey:      apiKey,
		workspaceID: workspaceID,
		httpClient:  &http.Client{},
		baseURL:     "https://api.clockify.me/api/v1",
	}
}
