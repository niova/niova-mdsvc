// Package mdsvctidbclient is a Go client for the mdsvc-tidb REST API — a
// plain JSON/HTTP+JWT reimplementation of the control plane. Satisfies
// domain.ControlPlaneClient/domain.UserServiceClient.
package mdsvctidbclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// StatusOK mirrors mdsvc-tidb's dto.Response.Status: 0 is success, any
// other value a failure (class encoded in the value, but callers here only
// need success vs failure plus the Error string).
const StatusOK = 0

// envelope mirrors mdsvc-tidb's dto.Response[T]. HTTP status is always 200
// (even on auth failure) — success/failure is only distinguishable via the
// Status field in the body.
type envelope struct {
	Status  int             `json:"status"`
	Error   string          `json:"error,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Client is a minimal HTTP client for mdsvc-tidb's REST API.
type Client struct {
	BaseURL string
	Token   string
	http    *http.Client
}

// New creates a client pointed at baseURL (e.g. "http://localhost:8081").
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// SetToken sets the bearer token used for subsequent requests.
func (c *Client) SetToken(token string) { c.Token = token }

// withToken returns a lightweight *Client sharing this one's BaseURL and
// underlying http.Client but carrying a different token — used for the
// tenant-admin session, which is a separate identity from the regular
// user/admin session held in c.Token and must never overwrite it.
func (c *Client) withToken(token string) *Client {
	return &Client{BaseURL: c.BaseURL, Token: token, http: c.http}
}

// doRaw decodes mdsvc-tidb's envelope but does NOT treat a non-zero Status
// as a Go error — only transport-level failures are. Most callers want `do`
// instead; doRaw exists for the auth probe, which needs to tell "server
// says unauthorized" from "couldn't reach the server" apart.
func (c *Client) doRaw(method, path string, query url.Values, body any) (envelope, error) {
	fullURL := c.BaseURL + path
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}

	var bodyReader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return envelope{}, fmt.Errorf("mdsvctidbclient: encode request body: %w", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, fullURL, bodyReader)
	if err != nil {
		return envelope{}, fmt.Errorf("mdsvctidbclient: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return envelope{}, fmt.Errorf("mdsvctidbclient: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return envelope{}, fmt.Errorf("mdsvctidbclient: read response: %w", err)
	}

	// mdsvc-tidb reports failures with HTTP 200 and a non-zero envelope
	// Status, but a reverse proxy or the server itself can still fail before
	// producing that envelope (e.g. 404/502) — surface those distinctly.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return envelope{}, fmt.Errorf("mdsvctidbclient: %s %s: http %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var env envelope
	if err := json.Unmarshal(respBody, &env); err != nil {
		return envelope{}, fmt.Errorf("mdsvctidbclient: decode response: %w", err)
	}
	return env, nil
}

func (c *Client) do(method, path string, query url.Values, body any, out any) error {
	env, err := c.doRaw(method, path, query, body)
	if err != nil {
		return err
	}
	if env.Status != StatusOK {
		if env.Error != "" {
			return fmt.Errorf("mdsvctidbclient: %s", env.Error)
		}
		return fmt.Errorf("mdsvctidbclient: request failed (status %d)", env.Status)
	}
	if out != nil && len(env.Payload) > 0 {
		if err := json.Unmarshal(env.Payload, out); err != nil {
			return fmt.Errorf("mdsvctidbclient: decode payload: %w", err)
		}
	}
	return nil
}

func (c *Client) get(path string, query url.Values, out any) error {
	return c.do(http.MethodGet, path, query, nil, out)
}

func (c *Client) post(path string, body any, out any) error {
	return c.do(http.MethodPost, path, nil, body, out)
}

func (c *Client) put(path string, query url.Values, body any, out any) error {
	return c.do(http.MethodPut, path, query, body, out)
}

func (c *Client) delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil, nil)
}
