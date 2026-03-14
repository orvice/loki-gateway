package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type pushProcessorMock struct{ err error }

func (m *pushProcessorMock) HandlePush(context.Context, []byte, http.Header) error { return m.err }

func TestPushHandlerValidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPushRoutes(r, &pushProcessorMock{})

	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", strings.NewReader(`{"streams":[]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestPushHandlerInvalidPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPushRoutes(r, &pushProcessorMock{err: service.ErrInvalidPushPayload})

	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", strings.NewReader(`bad`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestPushHandlerInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterPushRoutes(r, &pushProcessorMock{err: errors.New("boom")})

	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", strings.NewReader(`{"streams":[]}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
