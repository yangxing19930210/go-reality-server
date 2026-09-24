package proxy

import (
	"testing"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/proxy/freedom"
	"github.com/xtls/xray-core/proxy/vless"
	vlessinbound "github.com/xtls/xray-core/proxy/vless/inbound"
	"github.com/xtls/xray-core/transport/internet/reality"
	"google.golang.org/protobuf/proto"

	"go-reality-server/internal/config"
)

func testServerConfig(t *testing.T) *config.ServerConfig {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.ListenAddress = "127.0.0.1"
	cfg.ListenPort = 14443
	cfg.VLESS.UUID = GenerateUUID()
	cfg.VLESS.ShortID = GenerateShortID()
	cfg.Reality.PrivateKey = kp.PrivateKeyB64()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

// unmarshalTyped 把 TypedMessage 解到 out,并顺带断言 TypedMessage 的类型名与
// out 的 proto 类型一致(即"确实是期望的那种配置",而非碰巧能解出来)。
func unmarshalTyped(t *testing.T, tm *serial.TypedMessage, out proto.Message) {
	t.Helper()
	if tm == nil {
		t.Fatal("typed message is nil")
	}
	if want := serial.GetMessageType(out); tm.Type != want {
		t.Fatalf("typed message type = %q, want %q", tm.Type, want)
	}
	if err := proto.Unmarshal(tm.Value, out); err != nil {
		t.Fatalf("unmarshal %s: %v", tm.Type, err)
	}
}

func TestBuildVLESSInbound(t *testing.T) {
	cfg := testServerConfig(t)
	ic, err := buildVLESSInbound(cfg)
	if err != nil {
		t.Fatalf("buildVLESSInbound: %v", err)
	}
	if ic.Tag != "vless-in" {
		t.Errorf("tag = %q", ic.Tag)
	}

	// ProxySettings 必须是 vless inbound Config,含一个用户、UUID 正确
	// (v1.260327.0 的 Config 里该字段叫 Clients,JSON/proto 名同为 clients。)
	var proxy vlessinbound.Config
	unmarshalTyped(t, ic.ProxySettings, &proxy)
	if len(proxy.Clients) != 1 {
		t.Fatalf("users = %d, want 1", len(proxy.Clients))
	}
	var account vless.Account
	unmarshalTyped(t, proxy.Clients[0].Account, &account)
	if account.Id != cfg.VLESS.UUID {
		t.Errorf("account id = %q, want %q", account.Id, cfg.VLESS.UUID)
	}
	if account.Flow != "" {
		t.Errorf("flow = %q, want empty (REALITY)", account.Flow)
	}

	// ReceiverSettings 必须监听目标端口
	var receiver proxyman.ReceiverConfig
	unmarshalTyped(t, ic.ReceiverSettings, &receiver)
	if receiver.PortList == nil || len(receiver.PortList.Range) != 1 ||
		receiver.PortList.Range[0].From != 14443 || receiver.PortList.Range[0].To != 14443 {
		t.Errorf("port list = %+v, want 14443", receiver.PortList)
	}

	// Stream security 必须是 reality 类型,字段与配置一致
	ss := receiver.StreamSettings
	if ss == nil {
		t.Fatal("stream settings nil")
	}
	if len(ss.SecuritySettings) != 1 {
		t.Fatalf("security settings = %d, want 1", len(ss.SecuritySettings))
	}
	var rc reality.Config
	unmarshalTyped(t, ss.SecuritySettings[0], &rc)
	if rc.Dest != "www.apple.com:443" {
		t.Errorf("reality dest = %q", rc.Dest)
	}
	if len(rc.ServerNames) != 1 || rc.ServerNames[0] != "www.apple.com" {
		t.Errorf("server names = %v", rc.ServerNames)
	}
	privKey, err := cfg.Reality.PrivateKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(rc.PrivateKey) != string(privKey) {
		t.Error("reality private key mismatch")
	}
	if len(rc.ShortIds) != 1 || len(rc.ShortIds[0]) != 8 {
		t.Errorf("short ids = %v, want one 8-byte id", rc.ShortIds)
	}
}

func TestBuildFreedomOutbound(t *testing.T) {
	oc := buildFreedomOutbound()
	if oc.Tag != "direct-out" {
		t.Errorf("tag = %q", oc.Tag)
	}
	var fr freedom.Config
	unmarshalTyped(t, oc.ProxySettings, &fr)
	var sender proxyman.SenderConfig
	unmarshalTyped(t, oc.SenderSettings, &sender)
}

func TestBuildAppMessages(t *testing.T) {
	msgs := buildAppMessages("info")
	if len(msgs) != 4 {
		t.Fatalf("app messages = %d, want 4 (xray-log, dispatcher, proxyman-in, proxyman-out)", len(msgs))
	}
	types := map[string]bool{}
	for _, m := range msgs {
		types[m.Type] = true
	}
	for _, want := range []string{
		"xray.app.log.Config",
		"xray.app.dispatcher.Config",
		"xray.app.proxyman.InboundConfig",
		"xray.app.proxyman.OutboundConfig",
	} {
		if !types[want] {
			t.Errorf("missing app type %q, got %v", want, types)
		}
	}
}
