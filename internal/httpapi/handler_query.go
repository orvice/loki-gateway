package httpapi

import (
	"context"
	"errors"
	"net/http"

	bflog "butterfly.orx.me/core/log"
	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type QueryProcessor interface {
	Proxy(ctx context.Context, in *http.Request) (status int, body []byte, err error)
}

func RegisterQueryRoutes(r gin.IRoutes, query QueryProcessor) {
	h := func(c *gin.Context) {
		logger := bflog.FromContext(c.Request.Context()).With(
			"component", "httpapi.query",
			"path", c.Request.URL.Path,
			"raw_query", c.Request.URL.RawQuery,
			"request_id", c.Request.Header.Get("X-Request-ID"),
			"tenant", c.Request.Header.Get("X-Scope-OrgID"),
		)

		status, body, err := query.Proxy(c.Request.Context(), c.Request)
		if errors.Is(err, service.ErrQueryUnavailable) {
			logger.Error("default loki unavailable", "error", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "default loki unavailable"})
			return
		}
		if err != nil {
			logger.Error("query proxy failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query proxy failed"})
			return
		}
		logger.Info("query proxy response", "status", status)
		c.Data(status, "application/json", body)
	}

	r.GET("/loki/api/v1/query", h)
	r.GET("/loki/api/v1/query_range", h)
	r.GET("/loki/api/v1/labels", h)
	r.GET("/loki/api/v1/label/:name/values", h)
}
