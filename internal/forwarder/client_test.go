package forwarder

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
)

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
