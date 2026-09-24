# go-reality-server

嵌入 [xray-core](https://github.com/XTLS/Xray-core) 的 VLESS + REALITY 代理服务端,单二进制、systemd 守护、无外部依赖。

## 快速开始(开发机)

```bash
# 1. 生成密钥(推荐带 -server 一并生成客户端分享链接)
go run ./cmd/server keys generate -server <你的服务器IP或域名>

# 2. 将输出填入 config.yaml(从 config.example.yaml 复制)
cp config.example.yaml config.yaml

# 3. 运行
go run ./cmd/server -config config.yaml
```

## 部署到 Linux 服务器

```bash
# 本机交叉编译
./scripts/cross-build.sh
# 上传并安装(在服务器上)
scp bin/go-reality-server-linux-amd64 user@server:/tmp/
scp config.example.yaml deploy/install.sh deploy/systemd/go-reality.service user@server:/tmp/
ssh user@server
sudo bash /tmp/install.sh /tmp/go-reality-server-linux-amd64
# 编辑 /etc/go-reality/config.yaml 填入密钥,重启
sudo systemctl restart go-reality
```

若服务器开了 UFW:`sudo ufw allow 443/tcp`

## 客户端接入

`keys generate -server ...` 输出的 ShareLink 可直接导入:
- Clash Meta / Mihomo(支持 vless+reality)
- v2rayN(Windows)
- NekoBox(Android)

也可用输出手工拼客户端配置:`security=reality`、`pbk=PublicKey`、`sid=ShortID`、`sni=server_names[0]`、`fp=chrome`、`spx=/`。

## 配置

完整字段与含义见 `config.example.yaml` 注释。YAML 中任何字段都可用环境变量覆盖:

| 环境变量 | 字段 |
|---|---|
| `LISTEN_PORT` | `listen_port` |
| `VLESS_UUID` | `vless.uuid` |
| `VLESS_SHORT_ID` | `vless.short_id` |
| `REALITY_PRIVATE_KEY` | `reality.private_key` |
| `REALITY_DEST` | `reality.dest` |
| `HTTP_API_PORT` | `http_api.listen_port` |
| `LOG_LEVEL` | `log_level` |

`reality.dest` 需为支持 TLS 1.3 的目标;当前锁定依赖下 microsoft.com 不兼容(记录超限),默认 www.apple.com 已实测可用。

### 安全建议

私钥等敏感字段建议放 `/etc/go-reality/env`(权限 `0600`,由 systemd 在切换用户前以 root 读取,unit 已配置 `EnvironmentFile`),而非默认 `0644`、所有用户可读的 `config.yaml`:

```
REALITY_PRIVATE_KEY=<base64>
VLESS_UUID=<uuid>
VLESS_SHORT_ID=<16位hex>
```

`listen_address` 默认 `::`(IPv6/双栈,同时接受 IPv4 连接);主机无 IPv6 时请改为 `0.0.0.0`。

## 运维

- 健康检查:`curl http://127.0.0.1:9090/healthz`(存活)、`/ready`(就绪)
- 指标:`curl http://127.0.0.1:9090/metrics`(Prometheus 格式)
- 日志:`journalctl -u go-reality -f -n 50`
- 服务 enable 后若未填密钥即 start/重启,会因配置非法每 5s 重试刷日志(unit 配置 `Restart=on-failure` + `RestartSec=5s`)——属预期行为,填完密钥 `systemctl restart go-reality` 即可
- 集成测试(真实出网,需可访问 www.apple.com 与 ifconfig.me):

```bash
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v
```
