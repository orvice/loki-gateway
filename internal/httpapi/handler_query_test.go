package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type queryProcessorMock struct {
	status int
	err    error
}

func (m *queryProcessorMock) Proxy(_ context.Context, w http.ResponseWriter, _ *http.Request) (int, error) {
	if m.err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(m.status)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}
	return m.status, m.err
}

func TestQueryHandlerAllSupportedEndpoints(t *testing.T) {
	paths := []string{
		"/loki/api/v1/query?query=up",
		"/loki/api/v1/query_range?query=up&start=1&end=2",
		"/loki/api/v1/labels",
		"/loki/api/v1/label/job/values",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			RegisterQueryRoutes(r, &queryProcessorMock{status: http.StatusOK})

			req := httptest.NewRequest(http.MethodGet, p, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			if w.Body.String() != `{"status":"success"}` {
				t.Fatalf("expected proxied body, got %s", w.Body.String())
			}
		})
	}
}

func TestQueryHandlerUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterQueryRoutes(r, &queryProcessorMock{err: service.ErrQueryUnavailable})

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", w.Code)
	}
}

func TestQueryHandlerInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterQueryRoutes(r, &queryProcessorMock{err: errors.New("boom")})

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/query?query=up", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
