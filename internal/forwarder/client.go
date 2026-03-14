package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	bflog "butterfly.orx.me/core/log"
	"github.com/orvice/loki-gateway/internal/config"
)

type Client interface {
	PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error
	ProxyQuery(ctx context.Context, target config.LokiTarget, in *http.Request) (*http.Response, error)
}

type HTTPClient struct {
	client *http.Client
}

type cancelOnCloseReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnCloseReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{client: &http.Client{}}
}

func (c *HTTPClient) PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error {
	path := strings.TrimRight(target.URL, "/") + "/loki/api/v1/push"
	timeout := time.Duration(target.TimeoutMS) * time.Millisecond
	logger := bflog.FromContext(ctx).With(
		"component", "forwarder.push",
		"target", target.Name,
		"target_url", target.URL,
		"path", path,
		"timeout_ms", target.TimeoutMS,
		"request_id", headers.Get("X-Request-ID"),
		"tenant", headers.Get("X-Scope-OrgID"),
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		logger.Error("create downstream push request failed", "error", err)
		return err
	}
	copyHeaders(headers, req.Header)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	applyTargetBasicAuth(req, target)
	applyTargetExtraHeaders(req, target)
	if req.Header.Get("X-Scope-OrgID") == "" && target.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", target.TenantID)
	}

	logger.Info("forward push request", "body_bytes", len(body))
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		logger.Error("forward push request failed", "error", err)
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	logger.Info("forward push response", "status", resp.StatusCode, "latency_ms", time.Since(start).Milliseconds())

	if resp.StatusCode >= http.StatusMultipleChoices {
		logger.Warn("forward push downstream error status", "status", resp.StatusCode)
		return fmt.Errorf("downstream push status: %d", resp.StatusCode)
	}

	return nil
}

func (c *HTTPClient) ProxyQuery(ctx context.Context, target config.LokiTarget, in *http.Request) (*http.Response, error) {
	path := strings.TrimRight(target.URL, "/") + in.URL.Path
	if in.URL.RawQuery != "" {
		path += "?" + in.URL.RawQuery
	}
	timeout := time.Duration(target.TimeoutMS) * time.Millisecond
	logger := bflog.FromContext(ctx).With(
		"component", "forwarder.query",
		"target", target.Name,
		"target_url", target.URL,
		"path", in.URL.Path,
		"raw_query", in.URL.RawQuery,
		"timeout_ms", target.TimeoutMS,
		"request_id", in.Header.Get("X-Request-ID"),
		"tenant", in.Header.Get("X-Scope-OrgID"),
	)

	ctx, cancel := context.WithTimeout(ctx, timeout)

	req, err := http.NewRequestWithContext(ctx, in.Method, path, nil)
	if err != nil {
		cancel()
		logger.Error("create downstream query request failed", "error", err)
		return nil, err
	}
	copyHeaders(in.Header, req.Header)
	applyTargetBasicAuth(req, target)
	applyTargetExtraHeaders(req, target)
	if req.Header.Get("X-Scope-OrgID") == "" && target.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", target.TenantID)
	}

	logger.Info("forward query request")
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		cancel()
		logger.Error("forward query request failed", "error", err)
		return nil, err
	}
	resp.Body = &cancelOnCloseReadCloser{
		ReadCloser: resp.Body,
		cancel:     cancel,
	}
	logger.Info("forward query response", "status", resp.StatusCode, "latency_ms", time.Since(start).Milliseconds())
	if resp.StatusCode >= http.StatusBadRequest {
		logger.Warn("forward query downstream error status", "status", resp.StatusCode)
	}
	return resp, nil
}

func copyHeaders(src, dst http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func applyTargetBasicAuth(req *http.Request, target config.LokiTarget) {
	if target.BasicAuth.Username == "" || target.BasicAuth.Password == "" {
		return
	}
	req.SetBasicAuth(target.BasicAuth.Username, target.BasicAuth.Password)
}

func applyTargetExtraHeaders(req *http.Request, target config.LokiTarget) {
	for k, v := range target.ExtraHeaders {
		req.Header.Set(k, v)
	}
}
