//go:build integration

package proxy

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	stdnet "net"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/dokodemo"
	"github.com/xtls/xray-core/proxy/vless"
	vlessoutbound "github.com/xtls/xray-core/proxy/vless/outbound"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"

	"go-reality-server/internal/config"
)

// TestIntegrationEgress 完整链路:本地 dokodemo 入口 -> 进程内 vless+reality 客户端
// -> 本服务端 -> freedom 直连 ifconfig.me,断言拿到出口 IP。
// 需要可访问 www.apple.com(REALITY 握手 dest)与 ifconfig.me。
//
// 注:本测试不用产品默认 dest www.microsoft.com:443——实测(2026-09,本机网络)
// 微软边缘对 uTLS ClientHello 返回的加密记录为 8273 字节,超过 xtls/reality
// (xray-core v1.260327.0 依赖)硬编码的 8192 上限,握手必失败;www.apple.com
// 的记录为 3376 字节,可正常完成。详见 task-8-report.md。
func TestIntegrationEgress(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	uuidStr, shortID, sni := GenerateUUID(), GenerateShortID(), "www.apple.com"

	// 1. 服务端(SNI 与 dest 保持同一站点,与真实部署一致)
	cfg := config.Defaults()
	cfg.ListenAddress = "127.0.0.1"
	cfg.ListenPort = freePort(t)
	cfg.VLESS.UUID = uuidStr
	cfg.VLESS.ShortID = shortID
	cfg.Reality.PrivateKey = kp.PrivateKeyB64()
	cfg.Reality.ServerNames = []string{sni}
	cfg.Reality.Dest = sni + ":443"
	engine, err := NewEngine(&cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Start(); err != nil {
		t.Fatalf("engine Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })

	// 2. 进程内客户端:dokodemo 入口 -> vless outbound(reality)
	dokodemoPort := freePort(t)
	clientInst, err := startClientInstance(t, cfg.ListenPort, uuidStr, shortID, kp, sni, dokodemoPort)
	if err != nil {
		t.Fatalf("client instance: %v", err)
	}
	t.Cleanup(func() { _ = clientInst.Close() })

	// 3. 经隧道请求 https://ifconfig.me,期望返回公网 IP
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (stdnet.Conn, error) {
			return (&stdnet.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp",
				stdnet.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", dokodemoPort)))
		},
		TLSClientConfig: &tls.Config{ServerName: "ifconfig.me"},
	}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	// ifconfig.me 对 curl 类 UA 返回纯 IP,对其他 UA 返回 HTML 首页。
	req, err := http.NewRequest(http.MethodGet, "https://ifconfig.me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "curl/8.0")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET through tunnel: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	ip := string(body)
	if !regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}\s*$`).MatchString(ip) {
		t.Fatalf("ifconfig.me returned %q, want an IP", ip)
	}
	t.Logf("egress OK, public IP = %s", ip)
}

func startClientInstance(t *testing.T, serverPort int, uuidStr, shortID string, kp *KeyPair, sni string, dokodemoPort int) (*core.Instance, error) {
	t.Helper()

	dokodemoInbound := &core.InboundHandlerConfig{
		Tag: "dokodemo-in",
		ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
			PortList: &net.PortList{
				Range: []*net.PortRange{
					{From: uint32(dokodemoPort), To: uint32(dokodemoPort)},
				},
			},
			Listen: net.NewIPOrDomain(net.LocalHostIP),
		}),
		ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
			Address:  net.NewIPOrDomain(net.ParseAddress("ifconfig.me")),
			Port:     443,
			Networks: []net.Network{net.Network_TCP},
		}),
	}

	realityMsg := serial.ToTypedMessage(&reality.Config{
		ServerName:  sni,
		Fingerprint: "chrome",
		PublicKey:   kp.PublicKey,
		ShortId:     mustHexDecode(t, shortID),
		SpiderX:     "/",
	})
	vlessOutbound := &core.OutboundHandlerConfig{
		Tag: "vless-out",
		SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{
			StreamSettings: &internet.StreamConfig{
				ProtocolName:     "tcp",
				SecurityType:     realityMsg.Type,
				SecuritySettings: []*serial.TypedMessage{realityMsg},
			},
		}),
		ProxySettings: serial.ToTypedMessage(&vlessoutbound.Config{
			Vnext: &protocol.ServerEndpoint{
				Address: net.NewIPOrDomain(net.LocalHostIP),
				Port:    uint32(serverPort),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&vless.Account{Id: uuidStr}),
				},
			},
		}),
	}

	instance, err := core.New(&core.Config{
		App:      buildAppMessages("warn"), // 测试期让 xray 内部日志保持安静
		Inbound:  []*core.InboundHandlerConfig{dokodemoInbound},
		Outbound: []*core.OutboundHandlerConfig{vlessOutbound},
	})
	if err != nil {
		return nil, err
	}
	if err := instance.Start(); err != nil {
		return nil, err
	}
	return instance, nil
}

func mustHexDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}
