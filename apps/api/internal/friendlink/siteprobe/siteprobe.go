// Package siteprobe decides whether a friend-link homepage is reachable.
//
// The verdict is deliberately lenient: any HTTP response below 500, including
// 401/403/404, counts as alive because a surprising number of galgame sites
// sit behind Cloudflare and answer a bot probe with a challenge page. Only a
// transport failure (timeout/DNS/TLS/connection) or a 5xx response means
// "打不开".
package siteprobe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const DefaultTimeout = 10 * time.Second

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 " +
	"(KHTML, like Gecko) Chrome/129.0.0.0 Safari/537.36 KungalFriendLinkCheck/1.0"

const maxReasonLen = 200

type Prober interface {
	Probe(ctx context.Context, rawURL string) (ok bool, reason string)
}

type Func func(ctx context.Context, rawURL string) (bool, string)

func (f Func) Probe(ctx context.Context, rawURL string) (bool, string) {
	return f(ctx, rawURL)
}

type HTTP struct {
	client  *http.Client
	timeout time.Duration
}

func NewHTTP(timeout time.Duration) *HTTP {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &HTTP{
		timeout: timeout,
		client:  &http.Client{Timeout: timeout},
	}
}

func (h *HTTP) Probe(ctx context.Context, rawURL string) (bool, string) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return false, "链接不是 http(s) 地址"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return false, truncate(err.Error())
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")

	resp, err := h.client.Do(req)
	if err != nil {
		return false, h.classify(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 500 {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return true, ""
}

func (h *HTTP) classify(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() || errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("请求超时（%.0fs）", h.timeout.Seconds())
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "DNS 解析失败"
	}
	var certVerify *tls.CertificateVerificationError
	if errors.As(err, &certVerify) {
		return "TLS 证书校验失败"
	}
	var unknownCA *x509.UnknownAuthorityError
	if errors.As(err, &unknownCA) {
		return "TLS 证书不受信任"
	}
	var headerErr *tls.RecordHeaderError
	if errors.As(err, &headerErr) {
		return "TLS 握手失败"
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "连接被拒绝"
	case strings.Contains(msg, "connection reset"):
		return "连接被重置"
	}
	return truncate(msg)
}

func truncate(s string) string {
	if len(s) <= maxReasonLen {
		return s
	}
	r := []rune(s)
	if len(r) <= maxReasonLen {
		return s
	}
	return string(r[:maxReasonLen])
}
