package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/orvice/loki-gateway/internal/config"
)

type queryForwarderMock struct {
	resp *http.Response
	err  error
	req  *http.Request
}

func (m *queryForwarderMock) PostPush(context.Context, config.LokiTarget, []byte, http.Header) error {
	return nil
}

func (m *queryForwarderMock) ProxyQuery(_ context.Context, _ config.LokiTarget, in *http.Request) (*http.Response, error) {
	m.req = in
	if m.err != nil {
		return nil, m.err
	}
	return m.resp, nil
}

func TestQueryProxySuccess(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}},
	}
	mock := &queryForwarderMock{resp: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"status":"success"}`))}}
	svc := NewQueryService(cfg, mock)

	req, _ := http.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	req.Header.Set("X-Scope-OrgID", "tenant-a")

	status, body, err := svc.Proxy(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d", status)
	}
	if string(body) != `{"status":"success"}` {
		t.Fatalf("unexpected body: %s", string(body))
	}
	if mock.req.Header.Get("X-Scope-OrgID") != "tenant-a" {
		t.Fatalf("expected tenant header to be forwarded")
	}
}

func TestQueryProxyUnavailable(t *testing.T) {
	cfg := config.LokiConfig{DefaultTarget: "loki-a", Targets: []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}}}
	svc := NewQueryService(cfg, &queryForwarderMock{err: errors.New("dial failed")})
	req, _ := http.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)

	_, _, err := svc.Proxy(context.Background(), req)
	if !errors.Is(err, ErrQueryUnavailable) {
		t.Fatalf("expected ErrQueryUnavailable, got %v", err)
	}
}
