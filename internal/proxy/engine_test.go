package proxy

import (
	"net"
	"strconv"
	"testing"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestEngineLifecycle(t *testing.T) {
	cfg := testServerConfig(t)
	cfg.ListenPort = freePort(t)

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine.IsReady() {
		t.Fatal("IsReady before Start should be false")
	}

	if err := engine.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !engine.IsReady() {
		t.Fatal("IsReady after Start should be true")
	}
	// 端口确实在监听
	if err := pingPort(cfg.ListenPort); err != nil {
		t.Fatalf("port %d not listening: %v", cfg.ListenPort, err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if engine.IsReady() {
		t.Fatal("IsReady after Stop should be false")
	}
	// 二次 Stop 幂等
	if err := engine.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestEngineNewFailsOnBadConfig(t *testing.T) {
	cfg := testServerConfig(t)
	cfg.Reality.PrivateKey = "not-base64!"
	if _, err := NewEngine(cfg); err == nil {
		t.Fatal("NewEngine with bad private key should fail")
	}
}

func pingPort(port int) error {
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
