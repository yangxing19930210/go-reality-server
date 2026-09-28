# go-reality-server

[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Platform](https://img.shields.io/badge/Platform-Linux%20amd64%20%7C%20arm64-lightgrey)](#)
[![Xray-core](https://img.shields.io/badge/Xray--core-v1.260327.0-8A2BE2)](https://github.com/XTLS/Xray-core)
[![systemd](https://img.shields.io/badge/systemd-DynamicUser-3C6EB4?logo=systemd&logoColor=white)](#)

> 嵌入 [xray-core](https://github.com/XTLS/Xray-core) 的 **VLESS + REALITY** 代理服务端
> 单二进制 · systemd 守护 · 零外部依赖 · 密钥永不出服务器

## ✨ 特性

- **单文件部署** — 一个二进制 + 一个脚本,服务器上两条命令上线
- **密钥零出域** — 私钥在服务器现场生成,仅存于 `0600` 的 env 文件,永不落开发机
- **systemd 加固** — DynamicUser(每次启动随机 uid)运行,开机自启、崩溃自愈
- **幂等升级** — 重复执行 bootstrap 只更新二进制与 unit,不重置密钥
- **双架构** — 自动识别 x86_64 / aarch64,`--arch` 可覆盖
- **开箱即用** — 部署完成自动生成 VLESS 分享链接与 Clash Meta 配置
- **可观测** — 内置 `/healthz` `/ready` 健康检查与 Prometheus `/metrics`

## 🚀 一键部署

**① 开发机打包**(任意有 bash + go 的机器):

```bash
make package        # → dist/go-reality-server-linux.tar.gz
```

**② 服务器上线**:

```bash
tar xzf go-reality-server-linux.tar.gz
sudo ./go-reality-server/bootstrap.sh
```

`bootstrap.sh` 全自动:安装二进制 → **现场生成密钥** → 启动 systemd 服务 → 防火墙放行 443 → 验证就绪 → 打印客户端分享链接。

- 分享链接中的服务器地址默认取出口 IP,可用 `--server <IP或域名>` 覆盖
- 架构自动识别(x86_64 / aarch64),可用 `--arch amd64|arm64` 覆盖
- **幂等**:重复执行只更新二进制与 unit,不会重置密钥;升级即重新打包上传再跑一次
- 客户端资料(分享链接 + Clash Meta 配置)保存在 `/etc/go-reality/client.txt`(0600,仅 root):

```bash
sudo cat /etc/go-reality/client.txt
```

> [!WARNING]
> NAT/内网主机:自动探测取到的是内网地址,请务必用 `--server <公网IP或域名>` 指定。

## 🔧 本地开发

```bash
cp config.example.yaml config.yaml
# 生成密钥并注入当前 shell 环境(密钥不落盘)
set -a; source <(go run ./cmd/server keys generate -format env); set +a
go run ./cmd/server -config config.yaml
```

要客户端分享链接时加 `-server`:

```bash
go run ./cmd/server keys generate -format env -server 127.0.0.1   # 注释区附 ShareLink 与 Clash 块
```

## 📱 客户端接入

部署完成后 `sudo cat /etc/go-reality/client.txt` 拿到分享链接,可直接导入:

- [Clash Meta / Mihomo](https://github.com/MetaCubeX/mihomo)(支持 vless + reality)
- [v2rayN](https://github.com/2dust/v2rayN)(Windows)
- [NekoBox](https://github.com/MatsuriDayo/NekoBoxForAndroid)(Android)

也可用输出手工拼客户端配置:`security=reality`、`pbk=PublicKey`、`sid=ShortID`、`sni=server_names[0]`、`fp=chrome`、`spx=/`。

## ⚙️ 配置

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

> [!NOTE]
> `reality.dest` 需为支持 TLS 1.3 的目标;当前锁定依赖下 microsoft.com 不兼容(记录超限),默认 www.apple.com 已实测可用。

### 🔐 权限模型(部署后)

| 路径 | 权限 | 说明 |
|---|---|---|
| `/etc/go-reality/` | 0755 root | 服务以 DynamicUser(每次启动随机 uid)运行,目录须可进入 |
| `config.yaml` | 0644 root | **不含任何密钥** |
| `env` | 0600 root | 密钥唯一落点;systemd 以 root 读取后才降权 |
| `client.txt` | 0600 root | 分享链接与 Clash 配置,仅 root 可读 |

> [!NOTE]
> `listen_address` 默认 `::`(IPv6/双栈,同时接受 IPv4 连接);主机无 IPv6 时请改为 `0.0.0.0`。

## 🩺 运维

| 操作 | 命令 |
|---|---|
| 健康检查(存活) | `curl http://127.0.0.1:9090/healthz` |
| 健康检查(就绪) | `curl http://127.0.0.1:9090/ready` |
| 指标 | `curl http://127.0.0.1:9090/metrics`(Prometheus 格式) |
| 日志 | `journalctl -u go-reality -f -n 50` |

- **升级**:开发机 `make package` → 上传 → `tar xzf` → `sudo ./go-reality-server/bootstrap.sh`(幂等,密钥不变)
- **集成测试**(真实出网,需可访问 www.apple.com 与 ifconfig.me):

```bash
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v
```

## 📄 许可

本项目代码以 [MIT](LICENSE) 协议开源;内嵌依赖 [xray-core](https://github.com/XTLS/Xray-core) 采用 MPL-2.0 协议。

---

如果这个项目帮到了你,欢迎点一个 ⭐ Star!
