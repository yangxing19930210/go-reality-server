package proxy

import (
	"fmt"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/vless"
	vlessinbound "github.com/xtls/xray-core/proxy/vless/inbound"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"

	"go-reality-server/internal/config"
)

// buildVLESSInbound 拼装 VLESS+REALITY inbound 的完整 proto 配置。
// 所有 xray proto 细节只允许出现在本包。
func buildVLESSInbound(cfg *config.ServerConfig) (*core.InboundHandlerConfig, error) {
	privKey, err := cfg.Reality.PrivateKeyBytes()
	if err != nil {
		return nil, fmt.Errorf("decode reality private key: %w", err)
	}
	shortIDs, err := cfg.ShortIDBytesList()
	if err != nil {
		return nil, fmt.Errorf("decode reality short ids: %w", err)
	}

	// VLESS 用户:Account 消息挂在 protocol.User 上
	user := &protocol.User{
		Account: serial.ToTypedMessage(&vless.Account{
			Id:   cfg.VLESS.UUID,
			Flow: cfg.VLESS.Flow, // REALITY 下为空
		}),
	}
	proxySettings := serial.ToTypedMessage(&vlessinbound.Config{
		Clients:    []*protocol.User{user},
		Decryption: "none",
	})

	// REALITY 安全层:挂 StreamConfig.SecuritySettings,类型名取自 TypedMessage.Type
	realityMsg := serial.ToTypedMessage(&reality.Config{
		Dest:        cfg.Reality.Dest,
		ServerNames: cfg.Reality.ServerNames,
		PrivateKey:  privKey,
		ShortIds:    shortIDs,
	})
	streamSettings := &internet.StreamConfig{
		ProtocolName:     "tcp",
		SecurityType:     realityMsg.Type,
		SecuritySettings: []*serial.TypedMessage{realityMsg},
	}

	receiverSettings := &proxyman.ReceiverConfig{
		PortList: &net.PortList{
			Range: []*net.PortRange{
				{From: uint32(cfg.ListenPort), To: uint32(cfg.ListenPort)},
			},
		},
		Listen:         net.NewIPOrDomain(net.ParseAddress(cfg.ListenAddress)),
		StreamSettings: streamSettings,
	}

	return &core.InboundHandlerConfig{
		Tag:              "vless-in",
		ReceiverSettings: serial.ToTypedMessage(receiverSettings),
		ProxySettings:    proxySettings,
	}, nil
}
