package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAuthenticationAndErrorRedaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		want := "Bearer org-secret"
		if strings.HasPrefix(r.URL.Path, "/scim/") {
			want = "Bearer scim-secret"
		}
		if got := r.Header.Get("Authorization"); got != want {
			t.Errorf("authorization = %q, want %q", got, want)
		}
		if r.URL.Path == "/api/v1/error" {
			w.WriteHeader(422)
			fmt.Fprint(w, `{"detail":"org-secret scim-secret echoed personal data"}`)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer server.Close()
	c, err := New(server.URL, "org-secret", "scim-secret", "test")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/api/v1/groups", "/scim/v2/Users"} {
		if _, err := c.Do(context.Background(), "GET", p, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	_, err = c.Do(context.Background(), "POST", "/api/v1/error", nil, map[string]any{"name": "private"})
	if err == nil || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
		t.Fatalf("unsafe error: %v", err)
	}
}
func TestCursorPaginationPreservesFilters(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("search") != "Finance & Ops" {
			t.Error("lost filter")
		}
		if requests == 1 {
			fmt.Fprint(w, `{"results":[{"id":"one"}],"next_cursor":"a+b/="}`)
		} else {
			if r.URL.Query().Get("cursor") != "a+b/=" {
				t.Error("cursor corrupted")
			}
			fmt.Fprint(w, `{"results":[{"id":"two"}],"next_cursor":null}`)
		}
	}))
	defer server.Close()
	c, _ := New(server.URL, "key", "", "test")
	q := url.Values{"search": {"Finance & Ops"}}
	items, err := c.List(context.Background(), "/api/v1/groups", q)
	if err != nil || len(items) != 2 || requests != 2 {
		t.Fatalf("items=%v requests=%d err=%v", items, requests, err)
	}
	if q.Has("cursor") {
		t.Error("mutated caller query")
	}
}
func TestSCIMPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := r.URL.Query().Get("startIndex")
		if start == "" {
			start = "1"
		}
		if start != "1" && start != "2" {
			t.Errorf("bad index %s", start)
		}
		json.NewEncoder(w).Encode(map[string]any{"Resources": []any{map[string]any{"id": start}}, "totalResults": 2, "itemsPerPage": 1})
	}))
	defer server.Close()
	c, _ := New(server.URL, "", "scim", "test")
	items, err := c.List(context.Background(), "/scim/v2/Users", nil)
	if err != nil || len(items) != 2 {
		t.Fatalf("%v %v", items, err)
	}
}
func TestRepeatedCursorFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"results":[],"next_cursor":"same"}`) }))
	defer server.Close()
	c, _ := New(server.URL, "key", "", "test")
	_, err := c.List(context.Background(), "/api/v1/groups", nil)
	if err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("%v", err)
	}
}
func TestOnlyReadsRetry(t *testing.T) {
	for _, method := range []string{"GET", "POST"} {
		t.Run(method, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "0")
				if calls == 1 {
					w.WriteHeader(429)
				} else {
					fmt.Fprint(w, `{}`)
				}
			}))
			defer server.Close()
			c, _ := New(server.URL, "key", "", "test")
			_, err := c.Do(context.Background(), method, "/api/v1/groups", nil, nil)
			if method == "GET" && (err != nil || calls != 2) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if method == "POST" && (err == nil || calls != 1) {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
		})
	}
}
func TestRejectsRedirectsAndMissingCredentials(t *testing.T) {
	called := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	c, _ := New(server.URL, "key", "", "test")
	if _, err := c.Do(context.Background(), "GET", "/api/v1/groups", nil, nil); err == nil {
		t.Fatal("accepted redirect")
	}
	if called {
		t.Fatal("followed redirect")
	}
	if _, err := c.Do(context.Background(), "GET", "/scim/v2/Users", nil, nil); err == nil {
		t.Fatal("used organization key for SCIM")
	}
}
func TestBaseURLValidation(t *testing.T) {
	for _, base := range []string{"ftp://example.com", "https://example.com/api", "https://user:pass@example.com", "https://example.com?x=y", "http://example.com", "relative"} {
		if _, err := New(base, "", "", "test"); err == nil {
			t.Errorf("accepted %s", base)
		}
	}
}
