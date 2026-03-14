package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/orvice/loki-gateway/internal/service"
)

type QueryProcessor interface {
	Proxy(ctx context.Context, in *http.Request) (status int, body []byte, err error)
}

func RegisterQueryRoutes(r gin.IRoutes, query QueryProcessor) {
	h := func(c *gin.Context) {
		status, body, err := query.Proxy(c.Request.Context(), c.Request)
		if errors.Is(err, service.ErrQueryUnavailable) {
			c.JSON(http.StatusBadGateway, gin.H{"error": "default loki unavailable"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "query proxy failed"})
			return
		}
		c.Data(status, "application/json", body)
	}

	r.GET("/loki/api/v1/query", h)
	r.GET("/loki/api/v1/query_range", h)
	r.GET("/loki/api/v1/labels", h)
	r.GET("/loki/api/v1/label/:name/values", h)
}
