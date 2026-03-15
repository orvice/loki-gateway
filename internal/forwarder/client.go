package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	bflog "butterfly.orx.me/core/log"
	"github.com/orvice/loki-gateway/internal/config"
)

type Client interface {
	PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error
	ProxyQuery(ctx context.Context, target config.LokiTarget, w http.ResponseWriter, in *http.Request) (int, error)
}

type HTTPClient struct {
	client *http.Client
}

type discardResponseWriter struct {
	header http.Header
	status int
}

func (w *discardResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *discardResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return len(p), nil
}

func (w *discardResponseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{client: &http.Client{}}
}

func (c *HTTPClient) PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error {
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		return err
	}
	path := "/loki/api/v1/push"
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

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = c.client.Transport
	statusCode := 0
	proxy.ModifyResponse = func(resp *http.Response) error {
		statusCode = resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	var proxyErr error
	proxy.ErrorHandler = func(_ http.ResponseWriter, _ *http.Request, err error) {
		proxyErr = err
	}

	outReq := req.Clone(ctx)
	outReq.RequestURI = ""
	w := &discardResponseWriter{}

	logger.Info("forward push request", "body_bytes", len(body))
	start := time.Now()
	proxy.ServeHTTP(w, outReq)

	if proxyErr != nil {
		logger.Error("forward push request failed", "error", proxyErr)
		return proxyErr
	}

	if statusCode == 0 {
		statusCode = w.status
	}
	logger.Info("forward push response", "status", statusCode, "latency_ms", time.Since(start).Milliseconds())

	if statusCode >= http.StatusMultipleChoices {
		logger.Warn("forward push downstream error status", "status", statusCode)
		return fmt.Errorf("downstream push status: %d", statusCode)
	}

	return nil
}

func (c *HTTPClient) ProxyQuery(ctx context.Context, target config.LokiTarget, w http.ResponseWriter, in *http.Request) (int, error) {
	targetURL, err := url.Parse(target.URL)
	if err != nil {
		return 0, err
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
	defer cancel()

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	baseDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		baseDirector(req)
		applyTargetBasicAuth(req, target)
		applyTargetExtraHeaders(req, target)
		if req.Header.Get("X-Scope-OrgID") == "" && target.TenantID != "" {
			req.Header.Set("X-Scope-OrgID", target.TenantID)
		}
	}
	proxy.Transport = c.client.Transport

	statusCode := 0
	proxy.ModifyResponse = func(resp *http.Response) error {
		statusCode = resp.StatusCode
		return nil
	}

	var proxyErr error
	proxy.ErrorHandler = func(_ http.ResponseWriter, _ *http.Request, err error) {
		proxyErr = err
	}

	outReq := in.Clone(ctx)
	outReq.RequestURI = ""

	logger.Info("forward query request")
	start := time.Now()
	proxy.ServeHTTP(w, outReq)

	if proxyErr != nil {
		logger.Error("forward query request failed", "error", proxyErr)
		return 0, proxyErr
	}

	logger.Info("forward query response", "status", statusCode, "latency_ms", time.Since(start).Milliseconds())
	if statusCode >= http.StatusBadRequest {
		logger.Warn("forward query downstream error status", "status", statusCode)
	}

	return statusCode, nil
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
