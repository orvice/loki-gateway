package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
)

type pushCall struct {
	target  string
	body    []byte
	headers http.Header
}

type pushForwarderMock struct {
	mu    sync.Mutex
	calls []pushCall
	err   error
	ch    chan struct{}
}

func (m *pushForwarderMock) PostPush(_ context.Context, target config.LokiTarget, body []byte, headers http.Header) error {
	m.mu.Lock()
	m.calls = append(m.calls, pushCall{target: target.Name, body: body, headers: cloneHeaders(headers)})
	m.mu.Unlock()
	if m.ch != nil {
		m.ch <- struct{}{}
	}
	return m.err
}

func (m *pushForwarderMock) ProxyQuery(context.Context, config.LokiTarget, http.ResponseWriter, *http.Request) (int, error) {
	return 0, errors.New("not used")
}

func (m *pushForwarderMock) snapshot() []pushCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]pushCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func TestPushRoutesToMatchedAndDefaultTargets(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []config.LokiTarget{
			{Name: "loki-a", URL: "http://a", TimeoutMS: 1000},
			{Name: "loki-b", URL: "http://b", TimeoutMS: 1000},
		},
		Rules: []config.RouteRule{{Name: "staging", Match: map[string]string{"env": "staging"}, Target: "loki-b"}},
	}
	mock := &pushForwarderMock{ch: make(chan struct{}, 2)}
	svc := NewPushService(cfg, mock, nil)

	body := []byte(`{"streams":[{"stream":{"env":"staging"},"values":[["1","line1"]]},{"stream":{"env":"prod"},"values":[["2","line2"]]}]}`)
	if err := svc.HandlePush(context.Background(), body, http.Header{}); err != nil {
		t.Fatalf("HandlePush returned error: %v", err)
	}

	waitCalls(t, mock.ch, 2)
	calls := mock.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected 2 forwards, got %d", len(calls))
	}
}

func TestPushMulticastDedup(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []config.LokiTarget{
			{Name: "loki-a", URL: "http://a", TimeoutMS: 1000},
			{Name: "loki-b", URL: "http://b", TimeoutMS: 1000},
			{Name: "loki-c", URL: "http://c", TimeoutMS: 1000},
		},
		Rules: []config.RouteRule{
			{Name: "r1", Match: map[string]string{"env": "prod"}, Target: "loki-b"},
			{Name: "r2", Match: map[string]string{"team": "core"}, Target: "loki-c"},
			{Name: "r3", Match: map[string]string{"env": "prod"}, Target: "loki-b"},
		},
	}
	mock := &pushForwarderMock{ch: make(chan struct{}, 2)}
	svc := NewPushService(cfg, mock, nil)

	body := []byte(`{"streams":[{"stream":{"env":"prod","team":"core"},"values":[["1","line1"]]}]}`)
	if err := svc.HandlePush(context.Background(), body, http.Header{}); err != nil {
		t.Fatalf("HandlePush returned error: %v", err)
	}

	waitCalls(t, mock.ch, 2)
	calls := mock.snapshot()
	if len(calls) != 2 {
		t.Fatalf("expected 2 unique target forwards, got %d", len(calls))
	}

	for _, c := range calls {
		var req pushRequest
		if err := json.Unmarshal(c.body, &req); err != nil {
			t.Fatalf("invalid marshaled payload: %v", err)
		}
		if len(req.Streams) != 1 {
			t.Fatalf("expected one stream in payload")
		}
	}
}

func TestPushInvalidPayload(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}},
	}
	mock := &pushForwarderMock{ch: make(chan struct{}, 1)}
	svc := NewPushService(cfg, mock, nil)

	err := svc.HandlePush(context.Background(), []byte("not-json"), http.Header{})
	if err != nil {
		t.Fatalf("expected nil error on invalid payload fallback, got %v", err)
	}

	waitCalls(t, mock.ch, 1)
	calls := mock.snapshot()
	if len(calls) != 1 || calls[0].target != "loki-a" {
		t.Fatalf("expected fallback to default target loki-a, got %+v", calls)
	}
}

func TestPushInvalidPayloadFallbackDefaultMissing(t *testing.T) {
	svc := NewPushService(config.LokiConfig{DefaultTarget: "missing"}, &pushForwarderMock{}, nil)
	err := svc.HandlePush(context.Background(), []byte("not-json"), http.Header{})
	if err == nil {
		t.Fatalf("expected error when fallback default target is missing")
	}
}

func TestPushGzipPayloadRoutesAndDropsEncodingHeader(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []config.LokiTarget{
			{Name: "loki-a", URL: "http://a", TimeoutMS: 1000},
			{Name: "loki-b", URL: "http://b", TimeoutMS: 1000},
		},
		Rules: []config.RouteRule{{Name: "staging", Match: map[string]string{"env": "staging"}, Target: "loki-b"}},
	}
	mock := &pushForwarderMock{ch: make(chan struct{}, 1)}
	svc := NewPushService(cfg, mock, nil)

	raw := []byte(`{"streams":[{"stream":{"env":"staging"},"values":[["1","line1"]]}]}`)
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}

	headers := http.Header{
		"Content-Encoding": []string{"gzip"},
		"Content-Type":     []string{"application/json"},
	}
	if err := svc.HandlePush(context.Background(), buf.Bytes(), headers); err != nil {
		t.Fatalf("HandlePush returned error: %v", err)
	}

	waitCalls(t, mock.ch, 1)
	calls := mock.snapshot()
	if len(calls) != 1 || calls[0].target != "loki-b" {
		t.Fatalf("expected routing to loki-b, got %+v", calls)
	}
	if calls[0].headers.Get("Content-Encoding") != "" {
		t.Fatalf("expected Content-Encoding removed after decode")
	}
}

func TestPushDownstreamFailureDoesNotFailClient(t *testing.T) {
	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://a", TimeoutMS: 1000}},
	}
	mock := &pushForwarderMock{err: errors.New("downstream failed"), ch: make(chan struct{}, 1)}
	svc := NewPushService(cfg, mock, nil)
	body := []byte(`{"streams":[{"stream":{"env":"dev"},"values":[["1","line"]]}]}`)

	if err := svc.HandlePush(context.Background(), body, http.Header{}); err != nil {
		t.Fatalf("expected nil error for accepted push, got %v", err)
	}
	waitCalls(t, mock.ch, 1)
}

func waitCalls(t *testing.T, ch <-chan struct{}, n int) {
	t.Helper()
	deadline := time.After(500 * time.Millisecond)
	count := 0
	for count < n {
		select {
		case <-ch:
			count++
		case <-deadline:
			t.Fatalf("timed out waiting for %d calls; got %d", n, count)
		}
	}
}
