package siteprobe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPProbeVerdicts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/bad-gateway":
			w.WriteHeader(http.StatusBadGateway)
		case "/service-unavailable":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/redirect":
			http.Redirect(w, r, "/forbidden", http.StatusFound)
		}
	}))
	defer srv.Close()

	p := NewHTTP(time.Second)
	cases := []struct {
		name       string
		path       string
		wantOK     bool
		wantReason string
	}{
		{"200 alive", "/ok", true, ""},
		{"403 alive (cloudflare-style)", "/forbidden", true, ""},
		{"404 alive", "/missing", true, ""},
		{"401 alive", "/unauthorized", true, ""},
		{"redirect followed to 403 alive", "/redirect", true, ""},
		{"502 dead", "/bad-gateway", false, "HTTP 502"},
		{"503 dead", "/service-unavailable", false, "HTTP 503"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := p.Probe(context.Background(), srv.URL+c.path)
			if ok != c.wantOK || reason != c.wantReason {
				t.Errorf("Probe(%s) = (%v, %q), want (%v, %q)", c.path, ok, reason, c.wantOK, c.wantReason)
			}
		})
	}
}

func TestHTTPProbeTransportFailures(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refused := "http://" + ln.Addr().String()
	_ = ln.Close()

	p := NewHTTP(time.Second)
	if ok, reason := p.Probe(context.Background(), refused); ok || reason != "连接被拒绝" {
		t.Errorf("refused = (%v, %q), want (false, 连接被拒绝)", ok, reason)
	}

	// 10.255.255.1 is non-routable from inside Docker for Mac too; a shorter
	// client timeout keeps the test fast while exercising the timeout path.
	fast := NewHTTP(200 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	if ok, reason := fast.Probe(ctx, "http://10.255.255.1:80/"); ok || reason == "" {
		t.Errorf("black-hole = (%v, %q), want failure with reason", ok, reason)
	}
}

func TestHTTPProbeRejectsNonHTTP(t *testing.T) {
	p := NewHTTP(time.Second)
	if ok, _ := p.Probe(context.Background(), "ftp://example.com"); ok {
		t.Error("non-http url must fail")
	}
}

func TestFuncImplementsProber(t *testing.T) {
	var _ Prober = Func(func(context.Context, string) (bool, string) { return true, "" })
}

func TestTruncateKeepsUTF8(t *testing.T) {
	got := truncate(strings.Repeat("超", maxReasonLen+50))
	if len([]rune(got)) != maxReasonLen || strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("truncation broke a rune boundary: %d runes", len([]rune(got)))
	}
	short := "HTTP 500"
	if truncate(short) != short {
		t.Fatal("a short reason must stay untouched")
	}
}
