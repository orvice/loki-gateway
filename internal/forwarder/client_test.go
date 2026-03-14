package forwarder

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
)

type contextBoundBody struct {
	ctx context.Context
}

func (b *contextBoundBody) Read(_ []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

func (b *contextBoundBody) Close() error { return nil }

type contextAwareRoundTripper struct{}

func (contextAwareRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       &contextBoundBody{ctx: req.Context()},
		Header:     make(http.Header),
	}, nil
}

func TestPostPushForwardsBodyAndHeader(t *testing.T) {
	var gotBody []byte
	var gotScope string
	var gotPath string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotScope = r.Header.Get("X-Scope-OrgID")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	err := client.PostPush(context.Background(), config.LokiTarget{Name: "a", URL: ts.URL, TimeoutMS: 2000}, []byte(`{"streams":[]}`), http.Header{"X-Scope-OrgID": {"tenant-a"}})
	if err != nil {
		t.Fatalf("PostPush failed: %v", err)
	}

	if gotPath != "/loki/api/v1/push" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotScope != "tenant-a" {
		t.Fatalf("unexpected scope header: %s", gotScope)
	}
	if string(gotBody) != `{"streams":[]}` {
		t.Fatalf("unexpected body: %s", string(gotBody))
	}
}

func TestPostPushTimeout(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	err := client.PostPush(context.Background(), config.LokiTarget{Name: "a", URL: ts.URL, TimeoutMS: 10}, []byte(`{"streams":[]}`), http.Header{})
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}

func TestProxyQueryForwardsQueryAndHeader(t *testing.T) {
	var gotPath, gotQuery, gotScope string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotScope = r.Header.Get("X-Scope-OrgID")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	req.Header.Set("X-Scope-OrgID", "tenant-x")

	resp, err := client.ProxyQuery(context.Background(), config.LokiTarget{Name: "a", URL: ts.URL, TimeoutMS: 2000}, req)
	if err != nil {
		t.Fatalf("ProxyQuery failed: %v", err)
	}
	defer resp.Body.Close()

	if gotPath != "/loki/api/v1/query" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotQuery != "query=up" {
		t.Fatalf("unexpected query: %s", gotQuery)
	}
	if gotScope != "tenant-x" {
		t.Fatalf("unexpected scope: %s", gotScope)
	}
}

func TestPostPushUsesTargetBasicAuth(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	err := client.PostPush(
		context.Background(),
		config.LokiTarget{
			Name:      "a",
			URL:       ts.URL,
			TimeoutMS: 2000,
			BasicAuth: config.BasicAuth{Username: "alice", Password: "secret"},
		},
		[]byte(`{"streams":[]}`),
		http.Header{"Authorization": {"Bearer token"}},
	)
	if err != nil {
		t.Fatalf("PostPush failed: %v", err)
	}

	if gotAuth == "" {
		t.Fatalf("expected authorization header to be set")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetBasicAuth("alice", "secret")
	if gotAuth != req.Header.Get("Authorization") {
		t.Fatalf("unexpected authorization header: %s", gotAuth)
	}
}

func TestProxyQueryUsesTargetBasicAuth(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := client.ProxyQuery(
		context.Background(),
		config.LokiTarget{
			Name:      "a",
			URL:       ts.URL,
			TimeoutMS: 2000,
			BasicAuth: config.BasicAuth{Username: "bob", Password: "pwd"},
		},
		req,
	)
	if err != nil {
		t.Fatalf("ProxyQuery failed: %v", err)
	}
	defer resp.Body.Close()

	reqExpected := httptest.NewRequest(http.MethodGet, "/", nil)
	reqExpected.SetBasicAuth("bob", "pwd")
	if gotAuth != reqExpected.Header.Get("Authorization") {
		t.Fatalf("unexpected authorization header: %s", gotAuth)
	}
}

func TestPostPushUsesTargetExtraHeadersWithOverride(t *testing.T) {
	var gotRegion, gotTenant string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRegion = r.Header.Get("X-Region")
		gotTenant = r.Header.Get("X-Tenant")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	err := client.PostPush(
		context.Background(),
		config.LokiTarget{
			Name:         "a",
			URL:          ts.URL,
			TimeoutMS:    2000,
			ExtraHeaders: map[string]string{"X-Region": "cn", "X-Tenant": "team-a"},
		},
		[]byte(`{"streams":[]}`),
		http.Header{"X-Region": {"us"}},
	)
	if err != nil {
		t.Fatalf("PostPush failed: %v", err)
	}

	if gotRegion != "cn" {
		t.Fatalf("expected X-Region to be overridden by target header, got %s", gotRegion)
	}
	if gotTenant != "team-a" {
		t.Fatalf("expected X-Tenant from target header, got %s", gotTenant)
	}
}

func TestProxyQueryUsesTargetExtraHeadersWithOverride(t *testing.T) {
	var gotRegion, gotTrace string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRegion = r.Header.Get("X-Region")
		gotTrace = r.Header.Get("X-Trace-ID")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewHTTPClient()
	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	req.Header.Set("X-Region", "us")

	resp, err := client.ProxyQuery(
		context.Background(),
		config.LokiTarget{
			Name:         "a",
			URL:          ts.URL,
			TimeoutMS:    2000,
			ExtraHeaders: map[string]string{"X-Region": "eu", "X-Trace-ID": "t-1"},
		},
		req,
	)
	if err != nil {
		t.Fatalf("ProxyQuery failed: %v", err)
	}
	defer resp.Body.Close()

	if gotRegion != "eu" {
		t.Fatalf("expected X-Region to be overridden by target header, got %s", gotRegion)
	}
	if gotTrace != "t-1" {
		t.Fatalf("expected X-Trace-ID from target header, got %s", gotTrace)
	}
}

func TestProxyQueryDoesNotCancelBeforeCallerReadsBody(t *testing.T) {
	client := &HTTPClient{
		client: &http.Client{Transport: contextAwareRoundTripper{}},
	}

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	resp, err := client.ProxyQuery(
		context.Background(),
		config.LokiTarget{Name: "a", URL: "http://example.com", TimeoutMS: 2000},
		req,
	)
	if err != nil {
		t.Fatalf("ProxyQuery failed: %v", err)
	}
	defer resp.Body.Close()

	_, readErr := io.ReadAll(resp.Body)
	if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
		t.Fatalf("expected response body to remain readable before caller closes it, got %v", readErr)
	}
}
