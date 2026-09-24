package proxy

import (
	"errors"
	"sync"

	core "github.com/xtls/xray-core/core"

	"go-reality-server/internal/config"
)

// Engine 包装嵌入的 xray-core 实例生命周期。
// xray 实例进程内同时只能有一个在运行,互斥锁保证 Start/Stop 串行。
type Engine struct {
	mu       sync.Mutex
	instance *core.Instance
	started  bool
	stopped  bool
}

// NewEngine 由 ServerConfig 拼装 proto 并创建(未启动的)xray 实例。
func NewEngine(cfg *config.ServerConfig) (*Engine, error) {
	inboundConfig, err := buildVLESSInbound(cfg)
	if err != nil {
		return nil, err
	}
	xrayConfig := &core.Config{
		App:      buildAppMessages(cfg.LogLevel),
		Inbound:  []*core.InboundHandlerConfig{inboundConfig},
		Outbound: []*core.OutboundHandlerConfig{buildFreedomOutbound()},
	}
	instance, err := core.New(xrayConfig)
	if err != nil {
		return nil, err
	}
	return &Engine{instance: instance}, nil
}

// Start 启动引擎,幂等失败:重复启动返回错误。
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return errors.New("engine already started")
	}
	if err := e.instance.Start(); err != nil {
		return err
	}
	e.started = true
	return nil
}

// Stop 优雅关闭引擎。幂等,可安全重复调用。
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return nil
	}
	e.stopped = true
	if !e.started {
		return nil
	}
	return e.instance.Close()
}

// IsReady 报告引擎是否正在运行(供 httpapi 探活)。
func (e *Engine) IsReady() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started && !e.stopped
}
