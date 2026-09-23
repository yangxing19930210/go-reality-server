package proxy

import (
	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"

	// InboundConfig/OutboundConfig 这两个 App 配置由下面两个包的 init() 注册;
	// 不导入它们,core.New 会报 "*proxyman.InboundConfig is not registered"。
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
)

// buildAppMessages 返回 core.Instance 必需的三要素 App 配置:
// dispatcher、inbound manager、outbound manager。
// (与 xray JSON 配置中 "app" 段的三个条目一一对应。)
func buildAppMessages() []*serial.TypedMessage {
	return []*serial.TypedMessage{
		serial.ToTypedMessage(&dispatcher.Config{}),
		serial.ToTypedMessage(&proxyman.InboundConfig{}),
		serial.ToTypedMessage(&proxyman.OutboundConfig{}),
	}
}
