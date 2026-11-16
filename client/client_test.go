package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetPooledTransport(t *testing.T) {
	tr1 := getPooledTransport("")
	tr2 := getPooledTransport("")
	if tr1 != tr2 {
		t.Errorf("expected same pooled transport instance for empty proxy, got %p and %p", tr1, tr2)
	}

	trProxy1 := getPooledTransport("http://127.0.0.1:8080")
	trProxy2 := getPooledTransport("http://127.0.0.1:8080")
	if trProxy1 != trProxy2 {
		t.Errorf("expected same pooled transport instance for same proxy, got %p and %p", trProxy1, trProxy2)
	}

	if tr1 == trProxy1 {
		t.Errorf("expected different transport instances for direct vs proxy")
	}

	if tr1.DisableKeepAlives {
		t.Errorf("expected DisableKeepAlives to be false")
	}
}

func TestGetParamPooled(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	agent := GetParam(ts.URL, "http://referrer.com", "foo=bar")
	if agent.Transport == nil {
		t.Fatalf("expected non-nil transport")
	}
	if agent.Transport.DisableKeepAlives {
		t.Fatalf("expected DisableKeepAlives=false on agent transport")
	}

	resp, body, errs := agent.End()
	if len(errs) > 0 {
		t.Fatalf("unexpected error: %v", errs[0])
	}
	if resp.StatusCode != 200 || body != "ok" {
		t.Fatalf("expected 200 and 'ok', got %d and %s", resp.StatusCode, body)
	}
}
