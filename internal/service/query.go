package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
	"github.com/orvice/loki-gateway/internal/forwarder"
	"github.com/orvice/loki-gateway/internal/metrics"
)

var ErrQueryUnavailable = errors.New("query downstream unavailable")

type QueryService struct {
	cfg       config.LokiConfig
	forwarder forwarder.Client
}

func NewQueryService(cfg config.LokiConfig, f forwarder.Client) *QueryService {
	return &QueryService{cfg: cfg, forwarder: f}
}

func (s *QueryService) Proxy(ctx context.Context, in *http.Request) (int, []byte, error) {
	target, ok := s.cfg.TargetByName(s.cfg.DefaultTarget)
	if !ok {
		return 0, nil, ErrQueryUnavailable
	}

	start := time.Now()
	defer metrics.RecordLatency(target.Name, "query", time.Since(start).Seconds())

	metrics.RecordAttempt(target.Name, "query")
	resp, err := s.forwarder.ProxyQuery(ctx, target, in)
	if err != nil {
		metrics.RecordFail(target.Name, "query", reasonFromError(err))
		return 0, nil, ErrQueryUnavailable
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.RecordFail(target.Name, "query", reasonFromError(err))
		return 0, nil, ErrQueryUnavailable
	}

	metrics.RecordSuccess(target.Name, "query")
	return resp.StatusCode, body, nil
}
