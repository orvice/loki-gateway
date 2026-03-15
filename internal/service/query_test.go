package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/orvice/loki-gateway/internal/config"
)

type queryForwarderMock struct {
	status int
	err    error
	req    *http.Request
	w      http.ResponseWriter
}

func (m *queryForwarderMock) PostPush(context.Context, config.LokiTarget, []byte, http.Header) error {
	return nil
}

func (m *queryForwarderMock) ProxyQuery(_ context.Context, _ config.LokiTarget, w http.ResponseWriter, in *http.Request) (int, error) {
	m.req = in
	m.w = w
	if m.err != nil {
		return 0, m.err
	}
	return m.status, nil
}

func TestQueryProxySuccess(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}},
	}
	mock := &queryForwarderMock{status: http.StatusOK}
	svc := NewQueryService(cfg, mock)

	req, _ := http.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	req.Header.Set("X-Scope-OrgID", "tenant-a")
	w := httptest.NewRecorder()

	status, err := svc.Proxy(context.Background(), w, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if mock.req.Header.Get("X-Scope-OrgID") != "tenant-a" {
		t.Fatalf("expected tenant header to be forwarded")
	}
	if mock.w == nil {
		t.Fatalf("expected response writer to be forwarded")
	}
}

func TestQueryProxyUnavailable(t *testing.T) {
	cfg := config.LokiConfig{DefaultTarget: "loki-a", Targets: []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}}}
	svc := NewQueryService(cfg, &queryForwarderMock{err: errors.New("dial failed")})
	req, _ := http.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	_, err := svc.Proxy(context.Background(), w, req)
	if !errors.Is(err, ErrQueryUnavailable) {
		t.Fatalf("expected ErrQueryUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "dial failed") {
		t.Fatalf("expected wrapped root cause in error, got %v", err)
	}
}

func TestQueryProxyUnavailableWhenDefaultTargetMissing(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "missing",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}},
	}
	svc := NewQueryService(cfg, &queryForwarderMock{})
	req, _ := http.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()

	_, err := svc.Proxy(context.Background(), w, req)
	if !errors.Is(err, ErrQueryUnavailable) {
		t.Fatalf("expected ErrQueryUnavailable, got %v", err)
	}
	if !strings.Contains(err.Error(), "default_target") {
		t.Fatalf("expected missing default target details in error, got %v", err)
	}
}
