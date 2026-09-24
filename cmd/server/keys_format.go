// cmd/server/keys_format.go
// keys generate 的输出渲染:text(人读)/ env(systemd EnvironmentFile
// 可直接落盘)/ clash(Clash Meta 配置块)。
package main

import (
	"fmt"
	"strings"

	"go-reality-server/internal/proxy"
)

// renderKeysText 人读四行,与历史输出逐字节一致(向后兼容)。
func renderKeysText(id, sid string, kp *proxy.KeyPair) string {
	var b strings.Builder
	fmt.Fprintf(&b, "UUID:        %s\n", id)
	fmt.Fprintf(&b, "ShortID:     %s\n", sid)
	fmt.Fprintf(&b, "PrivateKey:  %s\n", kp.PrivateKeyB64())
	fmt.Fprintf(&b, "PublicKey:   %s\n", kp.PublicKeyB64())
	return b.String()
}

// renderKeysEnv 输出恰好 4 行 KEY=VALUE,可直接落盘为 EnvironmentFile。
// REALITY_PUBLIC_KEY 不被服务端消费(见 internal/config/env_override.go
// 的固定名单),仅供 bootstrap 从 env 文件取公钥组客户端链接。
func renderKeysEnv(id, sid string, kp *proxy.KeyPair) string {
	var b strings.Builder
	fmt.Fprintf(&b, "VLESS_UUID=%s\n", id)
	fmt.Fprintf(&b, "VLESS_SHORT_ID=%s\n", sid)
	fmt.Fprintf(&b, "REALITY_PRIVATE_KEY=%s\n", kp.PrivateKeyB64())
	fmt.Fprintf(&b, "REALITY_PUBLIC_KEY=%s\n", kp.PublicKeyB64())
	return b.String()
}

// renderClashProxy 输出 Clash Meta proxies 列表的单个条目(两空格缩进)。
func renderClashProxy(server string, port int, id, sid, pubKeyB64, sni, name string) string {
	if name == "" {
		name = "go-reality-server"
	}
	return fmt.Sprintf(`  - name: %q
    type: vless
    server: %s
    port: %d
    uuid: %s
    network: tcp
    tls: true
    udp: true
    flow: ""
    client-fingerprint: chrome
    servername: %s
    reality-opts:
      public-key: %s
      short-id: %s`,
		name, server, port, id, sni, pubKeyB64, sid)
}
