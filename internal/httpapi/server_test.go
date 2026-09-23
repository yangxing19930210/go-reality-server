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

func TestNewWithDefaultRegistererDoesNotPanic(t *testing.T) {
	// 生产路径:registerer 传 nil → DefaultRegisterer(其 init 已注册 GoCollector)。
	// 修复前这里会因重复注册 panic。
	srv := New("127.0.0.1:0", mockEngine{ready: true}, nil)
	if srv == nil {
		t.Fatal("New returned nil")
	}
	// 指标仍可正常抓取(就绪 gauge 存在)
	gathered, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	found := false
	for _, mf := range gathered {
		if mf.GetName() == "go_reality_engine_ready" {
			found = true
		}
	}
	if !found {
		t.Error("go_reality_engine_ready metric missing from default gatherer")
	}
}
