// internal/httpapi/server_test.go
package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type mockEngine struct{ ready bool }

func (m mockEngine) IsReady() bool { return m.ready }

func testServer(t *testing.T, engine EngineState) *httptest.Server {
	t.Helper()
	reg := prometheus.NewRegistry()
	srv := New("127.0.0.1:0", engine, reg)
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestHealthz(t *testing.T) {
	ts := testServer(t, mockEngine{ready: false})
	code, body := get(t, ts, "/healthz")
	if code != http.StatusOK {
		t.Errorf("healthz code = %d, want 200", code)
	}
	if body != "ok" {
		t.Errorf("healthz body = %q, want ok", body)
	}
}

func TestReady(t *testing.T) {
	ts := testServer(t, mockEngine{ready: true})
	code, body := get(t, ts, "/ready")
	if code != http.StatusOK || body != "ready" {
		t.Errorf("ready = %d %q, want 200 ready", code, body)
	}
}

func TestNotReady(t *testing.T) {
	ts := testServer(t, mockEngine{ready: false})
	code, body := get(t, ts, "/ready")
	if code != http.StatusServiceUnavailable || body != "not ready" {
		t.Errorf("ready = %d %q, want 503 not ready", code, body)
	}
}

func TestMetrics(t *testing.T) {
	ts := testServer(t, mockEngine{ready: true})
	code, body := get(t, ts, "/metrics")
	if code != http.StatusOK {
		t.Fatalf("metrics code = %d, want 200", code)
	}
	for _, want := range []string{"go_reality_engine_ready", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	// ready 引擎的 gauge 值为 1
	if !strings.Contains(body, "go_reality_engine_ready 1") {
		t.Error("engine gauge should be 1 when ready")
	}
}
