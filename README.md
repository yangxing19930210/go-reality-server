# go-reality-server

嵌入 [xray-core](https://github.com/XTLS/Xray-core) 的 VLESS + REALITY 代理服务端,单二进制、systemd 守护、无外部依赖。

## 一键部署(推荐)

开发机(任意有 bash + go 的机器)打包:

```bash
make package        # → dist/go-reality-server-linux.tar.gz
```

服务器上两条命令:

```bash
tar xzf go-reality-server-linux.tar.gz
sudo ./go-reality-server/bootstrap.sh
```

bootstrap.sh 全自动:安装二进制 → **现场生成密钥**(密钥永不出服务器)→ 启动 systemd 服务 → 防火墙放行 443 → 验证就绪 → 打印客户端分享链接。

- 架构自动识别(x86_64/aarch64),可用 `--arch amd64|arm64` 覆盖
- 分享链接中的服务器地址默认取出口 IP,可用 `--server <IP或域名>` 覆盖
- **幂等**:重复执行只更新二进制与 unit,不会重置密钥;升级即重新打包上传再跑一次
- 客户端资料(分享链接 + Clash Meta 配置)保存在 `/etc/go-reality/client.txt`(0600,仅 root):`sudo cat /etc/go-reality/client.txt`

## 快速开始(开发机本地跑)

```bash
cp config.example.yaml config.yaml
# 生成密钥并注入当前 shell 环境(密钥不落盘)
set -a; source <(go run ./cmd/server keys generate -format env); set +a
go run ./cmd/server -config config.yaml
```

要客户端分享链接时加 `-server`:`go run ./cmd/server keys generate -format env -server 127.0.0.1`(注释区附 ShareLink 与 Clash 块)。

## 客户端接入

部署完成后 `sudo cat /etc/go-reality/client.txt` 拿到分享链接,可直接导入:

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

### 权限模型(部署后)

| 路径 | 权限 | 说明 |
|---|---|---|
| `/etc/go-reality/` | 0755 root | 服务以 DynamicUser(每次启动随机 uid)运行,目录须可进入 |
| `config.yaml` | 0644 root | **不含任何密钥** |
| `env` | 0600 root | 密钥唯一落点;systemd 以 root 读取后才降权 |
| `client.txt` | 0600 root | 分享链接与 Clash 配置,仅 root 可读 |

`listen_address` 默认 `::`(IPv6/双栈,同时接受 IPv4 连接);主机无 IPv6 时请改为 `0.0.0.0`。

## 运维

- 健康检查:`curl http://127.0.0.1:9090/healthz`(存活)、`/ready`(就绪)
- 指标:`curl http://127.0.0.1:9090/metrics`(Prometheus 格式)
- 日志:`journalctl -u go-reality -f -n 50`
- 升级:开发机 `make package` → 上传 → `tar xzf` → `sudo ./go-reality-server/bootstrap.sh`(幂等,密钥不变)
- 集成测试(真实出网,需可访问 www.apple.com 与 ifconfig.me):

```bash
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v
```
