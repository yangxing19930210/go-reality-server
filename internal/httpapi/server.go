// Package httpapi 提供健康检查与 Prometheus 指标端点。
package httpapi

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EngineState 是 HTTP API 所需的引擎最小视图,避免包循环依赖。
type EngineState interface {
	IsReady() bool
}

// Server 暴露 /metrics、/healthz、/ready。
type Server struct {
	http *http.Server
}

// New 构建 HTTP API 服务。registerer 传 nil 时用 prometheus.DefaultRegisterer。
// 注意:同一 registerer 只能注册一次,main 中只创建一个 Server。
func New(addr string, engine EngineState, registerer prometheus.Registerer) *Server {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(
		collectors.NewGoCollector(),
		newReadyCollector(engine),
	)

	// promhttp 需要 Gatherer 而非 Registerer。常见实现(*prometheus.Registry、
	// DefaultRegisterer)两者兼具;非 Gatherer 的 registerer 退化为默认 Gatherer。
	gatherer, ok := registerer.(prometheus.Gatherer)
	if !ok {
		gatherer = prometheus.DefaultGatherer
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(gatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if engine.IsReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})

	return &Server{http: &http.Server{Addr: addr, Handler: mux}}
}

// ListenAndServe 阻塞服务,直到 Shutdown。
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// handler 暴露给测试用 httptest 包装。
func (s *Server) handler() http.Handler {
	return s.http.Handler
}

// readyCollector 在抓取时实时读取引擎状态,免去同步 gauge 的复杂度。
type readyCollector struct {
	engine EngineState
	desc   *prometheus.Desc
}

func newReadyCollector(engine EngineState) *readyCollector {
	return &readyCollector{
		engine: engine,
		desc: prometheus.NewDesc(
			"go_reality_engine_ready",
			"1 when the proxy engine is running, 0 otherwise.",
			nil, nil,
		),
	}
}

func (c *readyCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *readyCollector) Collect(ch chan<- prometheus.Metric) {
	v := 0.0
	if c.engine.IsReady() {
		v = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v)
}
