package forwarder

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/orvice/loki-gateway/internal/config"
)

type Client interface {
	PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error
	ProxyQuery(ctx context.Context, target config.LokiTarget, in *http.Request) (*http.Response, error)
}

type HTTPClient struct {
	client *http.Client
}

func NewHTTPClient() *HTTPClient {
	return &HTTPClient{client: &http.Client{}}
}

func (c *HTTPClient) PostPush(ctx context.Context, target config.LokiTarget, body []byte, headers http.Header) error {
	path := strings.TrimRight(target.URL, "/") + "/loki/api/v1/push"
	timeout := time.Duration(target.TimeoutMS) * time.Millisecond

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
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

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= http.StatusMultipleChoices {
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

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, in.Method, path, nil)
	if err != nil {
		return nil, err
	}
	copyHeaders(in.Header, req.Header)
	applyTargetBasicAuth(req, target)
	applyTargetExtraHeaders(req, target)
	if req.Header.Get("X-Scope-OrgID") == "" && target.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", target.TenantID)
	}

	return c.client.Do(req)
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
