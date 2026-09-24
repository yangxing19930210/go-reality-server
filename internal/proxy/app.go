package proxy

import (
	"github.com/xtls/xray-core/app/dispatcher"
	xraylog "github.com/xtls/xray-core/app/log"
	"github.com/xtls/xray-core/app/proxyman"
	commonlog "github.com/xtls/xray-core/common/log"
	"github.com/xtls/xray-core/common/serial"

	// InboundConfig/OutboundConfig 这两个 App 配置由下面两个包的 init() 注册;
	// 不导入它们,core.New 会报 "*proxyman.InboundConfig is not registered"。
	_ "github.com/xtls/xray-core/app/proxyman/inbound"
	_ "github.com/xtls/xray-core/app/proxyman/outbound"
)

// buildAppMessages 返回 core.Instance 必需的 App 配置:
// xray 内部日志、dispatcher、inbound manager、outbound manager。
// (与 xray JSON 配置中 "app" 段的条目一一对应。)
// logLevel 决定 xray 内部日志(REALITY 握手失败、dispatcher 错误等)的最低级别;
// 不传 app/log 配置时 xray 用默认 plain writer 打印,不受 log_level 管控。
func buildAppMessages(logLevel string) []*serial.TypedMessage {
	severity := commonlog.Severity_Info
	switch logLevel {
	case "debug":
		severity = commonlog.Severity_Debug
	case "warn":
		severity = commonlog.Severity_Warning
	case "error":
		severity = commonlog.Severity_Error
	}
	return []*serial.TypedMessage{
		// xray 内部日志放最前,启动期即可受控(与 xray JSON 层同款顺序)
		serial.ToTypedMessage(&xraylog.Config{
			ErrorLogType:  xraylog.LogType_Console,
			ErrorLogLevel: severity,
			AccessLogType: xraylog.LogType_None,
		}),
		serial.ToTypedMessage(&dispatcher.Config{}),
		serial.ToTypedMessage(&proxyman.InboundConfig{}),
		serial.ToTypedMessage(&proxyman.OutboundConfig{}),
	}
}
