package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	bflog "butterfly.orx.me/core/log"
	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type PushProcessor interface {
	HandlePush(ctx context.Context, body []byte, headers http.Header) error
}

func RegisterPushRoutes(r gin.IRoutes, push PushProcessor) {
	r.POST("/loki/api/v1/push", func(c *gin.Context) {
		logger := bflog.FromContext(c.Request.Context()).With(
			"component", "httpapi.push",
			"path", c.Request.URL.Path,
			"raw_query", c.Request.URL.RawQuery,
			"request_id", c.Request.Header.Get("X-Request-ID"),
			"tenant", c.Request.Header.Get("X-Scope-OrgID"),
		)

		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			logger.Error("read push request body failed", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		logger.Info("push request received", "body_bytes", len(body))

		err = push.HandlePush(c.Request.Context(), body, c.Request.Header)
		if errors.Is(err, service.ErrInvalidPushPayload) {
			logger.Error("invalid loki push payload", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid loki push payload"})
			return
		}
		if err != nil {
			logger.Error("push handling failed", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "push handling failed"})
			return
		}

		logger.Info("push request handled", "status", http.StatusNoContent)
		c.Status(http.StatusNoContent)
	})
}
