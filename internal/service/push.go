package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
	decodeBody, decoded, err := decodePushBodyForRouting(body, headers)
	if err != nil {
		s.logger.Error("decode push body failed, fallback to default target", "encoding", headers.Get("Content-Encoding"), "error", err)
		return s.forwardRawToDefault(ctx, body, headers)
	}

	var req pushRequest
	if err := json.Unmarshal(decodeBody, &req); err != nil {
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

	forwardHeaders := headers
	if decoded {
		forwardHeaders = cloneHeaders(headers)
		forwardHeaders.Del("Content-Encoding")
		forwardHeaders.Del("Content-Length")
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

		go s.forwardPush(bg, target, payload, forwardHeaders)
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

func decodePushBodyForRouting(body []byte, headers http.Header) ([]byte, bool, error) {
	encoding := strings.ToLower(strings.TrimSpace(headers.Get("Content-Encoding")))
	switch encoding {
	case "", "identity":
		return body, false, nil
	case "gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, true, err
		}
		defer r.Close()
		decoded, err := io.ReadAll(r)
		if err != nil {
			return nil, true, err
		}
		return decoded, true, nil
	default:
		return nil, true, fmt.Errorf("unsupported content encoding: %s", encoding)
	}
}

func cloneHeaders(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for k, vv := range src {
		copied := make([]string, len(vv))
		copy(copied, vv)
		dst[k] = copied
	}
	return dst
}
