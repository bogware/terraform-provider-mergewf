// Package client implements authenticated, bounded Workforce API requests.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://wf-api.merge.dev"
const maxResponse = 32 << 20

type Client struct {
	base                       *url.URL
	apiKey, scimToken, version string
	http                       *http.Client
}
type APIError struct {
	Status       int
	Method, Path string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Workforce API returned HTTP %d for %s %s; check credentials, permission scopes, and request values", e.Status, e.Method, e.Path)
}
func IsNotFound(err error) bool { var e *APIError; return errors.As(err, &e) && e.Status == 404 }
func New(base, key, scim, version string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("base_url must be an absolute HTTP(S) origin without a path, credentials, query, or fragment")
	}
	if u.Scheme == "http" && u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1" {
		return nil, fmt.Errorf("base_url must use HTTPS except for local test servers")
	}
	return &Client{base: u, apiKey: key, scimToken: scim, version: version, http: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body any) (any, error) {
	token := c.apiKey
	if strings.HasPrefix(path, "/scim/") {
		token = c.scimToken
		if token == "" {
			return nil, fmt.Errorf("scim_token or MERGEWF_SCIM_TOKEN is required for SCIM operations")
		}
	} else if token == "" {
		return nil, fmt.Errorf("api_key or MERGEWF_API_KEY is required for Workforce operations")
	}
	if !strings.HasPrefix(path, "/api/v1/") && !strings.HasPrefix(path, "/scim/v2/") {
		return nil, fmt.Errorf("unsupported API path")
	}
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	// Paths are assembled from fixed routes and escaped individual identifiers.
	u := strings.TrimRight(c.base.String(), "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "terraform-provider-mergewf/"+c.version)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("Workforce request failed (%s %s): %w", method, path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
		res.Body.Close()
		// Only reads are retried: repeating a successful-but-unacknowledged POST can duplicate objects.
		if method == http.MethodGet && attempt < 3 && (res.StatusCode == 429 || res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504) {
			delay := time.Duration(1<<attempt) * time.Second
			if value, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && value >= 0 {
				delay = time.Duration(value) * time.Second
			}
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, &APIError{Status: res.StatusCode, Method: method, Path: path}
		}
		if readErr != nil {
			return nil, readErr
		}
		if len(data) > maxResponse {
			return nil, fmt.Errorf("API response exceeds 32 MiB")
		}
		if len(bytes.TrimSpace(data)) == 0 {
			return nil, nil
		}
		var result any
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("invalid JSON response for %s %s", method, path)
		}
		return result, nil
	}
}

// List follows Workforce cursors and SCIM indexes, preserving all caller filters.
// Bare arrays are supported for the non-paginated list endpoints.
func (c *Client) List(ctx context.Context, path string, q url.Values) ([]any, error) {
	query := url.Values{}
	for k, v := range q {
		query[k] = append([]string(nil), v...)
	}
	result := []any{}
	seen := map[string]bool{}
	for page := 0; page < 10000; page++ {
		data, err := c.Do(ctx, http.MethodGet, path, query, nil)
		if err != nil {
			return nil, err
		}
		if items, ok := data.([]any); ok {
			return append(result, items...), nil
		}
		obj, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected list response for %s", path)
		}
		if strings.HasPrefix(path, "/scim/") {
			items, ok := obj["Resources"].([]any)
			if !ok {
				return nil, fmt.Errorf("SCIM list response missing Resources")
			}
			result = append(result, items...)
			total, ok := obj["totalResults"].(float64)
			if !ok {
				return nil, fmt.Errorf("SCIM list response missing totalResults")
			}
			start := 1
			if s, err := strconv.Atoi(query.Get("startIndex")); err == nil {
				start = s
			}
			if start-1+len(items) >= int(total) {
				return result, nil
			}
			if len(items) == 0 {
				return nil, fmt.Errorf("SCIM pagination made no progress")
			}
			query.Set("startIndex", strconv.Itoa(start+len(items)))
			continue
		}
		items, ok := obj["results"].([]any)
		if !ok {
			return nil, fmt.Errorf("list response missing results")
		}
		result = append(result, items...)
		cursor, _ := obj["next_cursor"].(string)
		if cursor == "" {
			return result, nil
		}
		if seen[cursor] {
			return nil, fmt.Errorf("API repeated a pagination cursor")
		}
		seen[cursor] = true
		query.Set("cursor", cursor)
	}
	return nil, fmt.Errorf("pagination exceeded 10000 pages")
}
