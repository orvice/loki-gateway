package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
	"github.com/orvice/loki-gateway/internal/forwarder"
	"github.com/orvice/loki-gateway/internal/metrics"
	"github.com/orvice/loki-gateway/internal/routing"
)

var ErrInvalidPushPayload = errors.New("invalid loki push payload")

type PushService struct {
	cfg       config.LokiConfig
	forwarder forwarder.Client
	logger    *slog.Logger
}

type pushRequest struct {
	Streams []pushStream `json:"streams"`
}

type pushStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

func NewPushService(cfg config.LokiConfig, f forwarder.Client, logger *slog.Logger) *PushService {
	if logger == nil {
		logger = slog.Default()
	}
	return &PushService{cfg: cfg, forwarder: f, logger: logger}
}

func (s *PushService) HandlePush(ctx context.Context, body []byte, headers http.Header) error {
	var req pushRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.logger.Error("decode push payload failed, fallback to default target", "error", err)
		return s.forwardRawToDefault(ctx, body, headers)
	}
	if req.Streams == nil {
		s.logger.Error("push payload missing streams, fallback to default target")
		return s.forwardRawToDefault(ctx, body, headers)
	}

	buckets := make(map[string][]pushStream)
	for _, stream := range req.Streams {
		targets := routing.MatchTargets(stream.Stream, s.cfg.Rules)
		if len(targets) == 0 {
			targets = []string{s.cfg.DefaultTarget}
		}
		for _, target := range targets {
			buckets[target] = append(buckets[target], stream)
		}
	}

	bg := context.WithoutCancel(ctx)
	for targetName, streams := range buckets {
		target, ok := s.cfg.TargetByName(targetName)
		if !ok {
			s.logger.Error("skip unknown target at runtime", "target", targetName)
			continue
		}

		payload, err := json.Marshal(pushRequest{Streams: streams})
		if err != nil {
			s.logger.Error("marshal push payload failed", "target", targetName, "error", err)
			continue
		}

		go s.forwardPush(bg, target, payload, headers)
	}

	return nil
}

func (s *PushService) forwardRawToDefault(ctx context.Context, body []byte, headers http.Header) error {
	target, ok := s.cfg.TargetByName(s.cfg.DefaultTarget)
	if !ok {
		return fmt.Errorf("default_target=%q not found for fallback", s.cfg.DefaultTarget)
	}

	go s.forwardPush(context.WithoutCancel(ctx), target, body, headers)
	return nil
}

func (s *PushService) forwardPush(ctx context.Context, target config.LokiTarget, payload []byte, headers http.Header) {
	start := time.Now()
	defer metrics.RecordLatency(target.Name, "push", time.Since(start).Seconds())

	metrics.RecordAttempt(target.Name, "push")
	if err := s.forwarder.PostPush(ctx, target, payload, headers); err != nil {
		s.logger.Error("forward push failed", "target", target.Name, "endpoint", "push", "error", err)
		metrics.RecordFail(target.Name, "push", reasonFromError(err))
		return
	}
	metrics.RecordSuccess(target.Name, "push")
}

func reasonFromError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%T", err)
}
