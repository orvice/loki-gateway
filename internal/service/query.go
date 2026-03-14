package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"time"

	bflog "butterfly.orx.me/core/log"
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
	logger := bflog.FromContext(ctx).With(
		"component", "query-service",
		"path", in.URL.Path,
		"raw_query", in.URL.RawQuery,
		"default_target", s.cfg.DefaultTarget,
		"request_id", in.Header.Get("X-Request-ID"),
		"tenant", in.Header.Get("X-Scope-OrgID"),
	)

	logger.Info("proxy query request")
	target, ok := s.cfg.TargetByName(s.cfg.DefaultTarget)
	if !ok {
		err := fmt.Errorf("%w: default_target=%q not found in targets=%v", ErrQueryUnavailable, s.cfg.DefaultTarget, targetNames(s.cfg.Targets))
		logger.Error("default target missing", "error", err)
		return 0, nil, err
	}

	start := time.Now()
	defer metrics.RecordLatency(target.Name, "query", time.Since(start).Seconds())

	metrics.RecordAttempt(target.Name, "query")
	resp, err := s.forwarder.ProxyQuery(ctx, target, in)
	if err != nil {
		metrics.RecordFail(target.Name, "query", reasonFromError(err))
		wrapped := fmt.Errorf("%w: target=%q url=%q method=%s path=%s raw_query=%q cause=%v", ErrQueryUnavailable, target.Name, target.URL, in.Method, in.URL.Path, in.URL.RawQuery, err)
		logger.Error("proxy query downstream failed", "target", target.Name, "target_url", target.URL, "timeout_ms", target.TimeoutMS, "error", err)
		return 0, nil, wrapped
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		metrics.RecordFail(target.Name, "query", reasonFromError(err))
		wrapped := fmt.Errorf("%w: target=%q read_response_failed cause=%v", ErrQueryUnavailable, target.Name, err)
		logger.Error("proxy query read response failed", "target", target.Name, "error", err)
		return 0, nil, wrapped
	}

	if resp.StatusCode >= http.StatusBadRequest {
		logger.Warn("proxy query downstream returned error status", "target", target.Name, "status", resp.StatusCode)
	}

	metrics.RecordSuccess(target.Name, "query")
	logger.Info("proxy query completed", "target", target.Name, "status", resp.StatusCode)
	return resp.StatusCode, body, nil
}

func targetNames(targets []config.LokiTarget) []string {
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		names = append(names, t.Name)
	}
	slices.Sort(names)
	return names
}
