package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/config"
	"github.com/orvice/loki-gateway/internal/forwarder"
	"github.com/orvice/loki-gateway/internal/service"
)

func TestIntegrationPushRoutesToMatchedAndDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)

	pushA := make(chan []byte, 1)
	pushB := make(chan []byte, 1)

	a := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		b, _ := io.ReadAll(r.Body)
		pushA <- b
		w.WriteHeader(http.StatusNoContent)
	}))
	defer a.Close()

	b := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/push" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		pushB <- body
		w.WriteHeader(http.StatusNoContent)
	}))
	defer b.Close()

	cfg := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets: []config.LokiTarget{
			{Name: "loki-a", URL: a.URL, TimeoutMS: 1000},
			{Name: "loki-b", URL: b.URL, TimeoutMS: 1000},
		},
		Rules: []config.RouteRule{
			{Name: "staging", Match: map[string]string{"env": "staging"}, Target: "loki-b"},
		},
	}

	fwd := forwarder.NewHTTPClient()
	pushSvc := service.NewPushService(cfg, fwd, nil)

	r := gin.New()
	RegisterPushRoutes(r, pushSvc)

	payload := `{"streams":[{"stream":{"env":"staging"},"values":[["1","line1"]]},{"stream":{"env":"prod"},"values":[["2","line2"]]}]}`
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", strings.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	waitBody(t, pushA)
	waitBody(t, pushB)
}

func TestIntegrationQueryPassthroughAndUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/loki/api/v1/query" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer downstream.Close()

	cfgOK := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: downstream.URL, TimeoutMS: 1000}},
	}
	cfgBad := config.LokiConfig{
		DefaultTarget: "loki-a",
		Targets:       []config.LokiTarget{{Name: "loki-a", URL: "http://127.0.0.1:1", TimeoutMS: 30}},
	}
	fwd := forwarder.NewHTTPClient()

	rOK := gin.New()
	RegisterQueryRoutes(rOK, service.NewQueryService(cfgOK, fwd))
	w1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	rOK.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w1.Code)
	}

	rBad := gin.New()
	RegisterQueryRoutes(rBad, service.NewQueryService(cfgBad, fwd))
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	rBad.ServeHTTP(w2, req2)
	if w2.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w2.Code)
	}
}

func waitBody(t *testing.T, ch <-chan []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	select {
	case b := <-ch:
		if len(b) == 0 {
			t.Fatalf("received empty forwarded body")
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for forwarded body")
	}
}
