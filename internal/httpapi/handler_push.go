package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type PushProcessor interface {
	HandlePush(ctx context.Context, body []byte, headers http.Header) error
}

func RegisterPushRoutes(r gin.IRoutes, push PushProcessor) {
	r.POST("/loki/api/v1/push", func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		err = push.HandlePush(c.Request.Context(), body, c.Request.Header)
		if errors.Is(err, service.ErrInvalidPushPayload) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid loki push payload"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "push handling failed"})
			return
		}

		c.Status(http.StatusNoContent)
	})
}
