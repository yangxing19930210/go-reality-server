package proxy

import (
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/freedom"
)

// buildFreedomOutbound 返回直连 freedom outbound,所有出站流量直接走系统网络。
func buildFreedomOutbound() *core.OutboundHandlerConfig {
	return &core.OutboundHandlerConfig{
		Tag:            "direct-out",
		SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{}),
		ProxySettings:  serial.ToTypedMessage(&freedom.Config{}),
	}
}
