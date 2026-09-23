# Go Reality Server — 设计文档

日期：2026-09-23
状态：已批准（用户确认继续，采用设计稿 v0.1）
范围：MVP v1

## 1. 背景与目标

在 Linux 服务器上部署一个 VLESS + REALITY 协议的高防代理服务端，用 Go 编写、以 systemd 守护进程运行。原参考设计（AI 生成的单文件方案）存在多处硬伤：非法 import 路径、`xtls-rxtls-splice` 是非法 flow、默认伪装域名值畸形、REALITY 缺少 `dest` 字段、VLESS+Reality 配置 mux 不兼容、UUID/私钥硬编码。本设计为生产级重构。

## 2. 核心决策（已与用户确认）

| 决策点 | 选择 |
|---|---|
| 网络栈 | 嵌入 `github.com/xtls/xray-core/core` 作为 Go 库（不用 sing-box internal 包） |
| 参数来源 | YAML 配置文件 + 环境变量覆盖（env 优先） |
| v1 范围 | MVP：单个 VLESS+Reality inbound、`/metrics`、`/healthz`、systemd unit、Makefile、单元+集成测试 |

## 3. 项目结构与模块

- 模块路径：`go-reality-server`（本地模块，发布到 GitHub 前可改名）
- 项目根：`C:\Users\Administrator\Desktop\go-reality-server\`
- Go 版本：1.22+（依赖 `log/slog`）

```
go-reality-server/
├── go.mod / go.sum
├── Makefile                    # build / test / cross / fmt / lint / gen-keys
├── README.md                   # quickstart + 生成密钥 + 客户端分享链接示例
├── LICENSE                     # MIT
├── .gitignore
├── cmd/
│   └── server/
│       └── main.go             # 入口：装配 config + engine + httpapi，信号处理
├── internal/
│   ├── config/
│   │   ├── config.go           # ServerConfig 结构体 + Load()
│   │   ├── config_test.go
│   │   └── env_override.go     # env 覆盖注入
│   ├── proxy/
│   │   ├── engine.go           # Xray core.Server 生命周期包装（New/Start/Wait/Stop）
│   │   ├── engine_test.go
│   │   ├── inbound.go          # 构建 VLESS+Reality inbound proto
│   │   ├── outbound.go         # 构建 direct outbound proto
│   │   └── keys.go             # UUID / short_id / Reality 密钥对生成
│   ├── httpapi/
│   │   ├── server.go           # /metrics + /healthz + /ready
│   │   └── server_test.go
│   └── logging/
│       └── logging.go          # slog JSON handler，journald 友好
├── deploy/
│   ├── systemd/
│   │   └── go-reality.service  # hardened systemd unit
│   └── install.sh              # 服务器端一键安装脚本
├── scripts/
│   ├── cross-build.sh          # linux/amd64 + linux/arm64 交叉编译
│   └── gen-keys.sh             # 生成密钥对并打印到 stdout
├── docs/
│   └── superpowers/
│       └── specs/              # 本文件所在目录
└── config.example.yaml         # 带注释示范配置
```

## 4. 组件职责

### 4.1 `cmd/server/main.go`

只做装配与编排：
1. 若 `os.Args[1] == "keys"`，走密钥生成子命令（`keys generate`）并退出；此判断在 `flag.Parse` 之前
2. 解析 `-config` flag（默认 `config.yaml`）
3. `config.Load()` 读配置 + env 覆盖
4. 初始化 `slog` JSON 日志
5. `proxy.NewEngine(cfg)` → `engine.Start()`
6. 启动 `httpapi.Server`
7. 阻塞等待 SIGINT/SIGTERM，触发优雅关闭（先关 HTTP，再 `engine.Stop()`）
8. 启动失败用错误退出码退出（systemd 据此重启）

### 4.2 `internal/config`

- `ServerConfig` 字段与 `config.example.yaml` 一一对应
- `Load(path)`：读 YAML → env 覆盖 → 校验（UUID 格式、端口范围、short_id 十六进制、私钥非空）→ 返回 `*ServerConfig` 或错误
- 校验失败即启动失败（fail fast）

### 4.3 `internal/proxy`

- `Engine` 包装 `xray-core/core.Server`：
  - `NewEngine(cfg)`：由 `ServerConfig` 拼装 `*core.Config` proto（`InboundHandlerConfig` / `OutboundHandlerConfig`），构造 `core.New()`
  - `Start()`：启动实例
  - `Wait()`：阻塞等待引擎结束（供 main 监听运行期错误）
  - `Stop()`：优雅关闭
  - `IsReady() bool`：供 httpapi 探活
- 所有 Xray proto 拼装细节（`vless.ServerConfig`、`reality.Config`、`tls.Config`、`proxyman.ReceiverConfig` 等）**只允许出现在 inbound.go / outbound.go**，不泄漏到外层包
- `keys.go`：复用 `xray-core/common/uuid` 与 `xray-core/common/protocol` 生成 UUID、short_id、X25519 密钥对，不重复造轮子

### 4.4 `internal/httpapi`

- 独立 `net/http.Server`（标准库，不引第三方路由）
- `GET /metrics`：Prometheus 格式（`github.com/prometheus/client_golang`），注册 Go runtime 指标 + 引擎状态 gauge；上行/下行字节计数尝试接 Xray `stats.Manager`，若版本 API 不可用则降级为仅 runtime + gauge（best effort，不作为验收项）
- `GET /healthz`：进程存活探针，恒 200
- `GET /ready`：就绪探针，引擎 running 时 200，否则 503
- 通过只含 `IsReady()` 的小接口读取引擎状态，避免包循环依赖

### 4.5 `internal/logging`

- `slog.NewJSONHandler(os.Stdout)`，级别来自配置
- 直接输出 stdout，交给 journald 收集（systemd `StandardOutput=journal`）

## 5. 配置 Schema

`config.example.yaml`：

```yaml
listen_address: "::"
listen_port: 443

vless:
  uuid: ""              # 必填，空则启动报错
  flow: ""              # Reality 通常置空
  short_id: ""          # 必填，16 个十六进制字符

reality:
  private_key: ""       # 必填，X25519 私钥（base64 std）
  dest: "www.microsoft.com:443"
  server_names: ["www.microsoft.com"]
  short_ids: [""]       # 默认复用 vless.short_id
  x25519_pub: ""        # 非必填，仅用于生成客户端分享链接

http_api:
  listen_address: "127.0.0.1"   # 只监听本机，不对外暴露
  listen_port: 9090              # 0 = 关闭

log_level: "info"
```

环境变量覆盖（优先于 yaml）：

| 变量 | 映射到 |
|---|---|
| `LISTEN_PORT` | `listen_port` |
| `VLESS_UUID` | `vless.uuid` |
| `VLESS_SHORT_ID` | `vless.short_id` |
| `REALITY_PRIVATE_KEY` | `reality.private_key` |
| `REALITY_DEST` | `reality.dest` |
| `HTTP_API_PORT` | `http_api.listen_port` |
| `LOG_LEVEL` | `log_level` |

## 6. 构建、部署、运维

- `make build` → `bin/go-reality-server`（本机）
- `make cross` → `bin/go-reality-server-linux-{amd64,arm64}`（`-ldflags="-s -w"`）
- `make gen-keys` → 运行 `go-reality-server keys generate`，输出 UUID / short_id / 私钥 / 公钥 / vless 分享链接，直接粘进配置文件
- `deploy/install.sh`：在目标服务器上执行——安装二进制到 `/usr/local/bin`、创建 `/etc/go-reality/`（config + env，600 权限）、安装 systemd unit、enable + start

systemd unit 要点：

- `Type=simple`、`Restart=on-failure`、`RestartSec=5s`
- `EnvironmentFile=/etc/go-reality/env`（密钥不进 unit 文件）
- `AmbientCapabilities=CAP_NET_BIND_SERVICE`（非 root 绑 443）
- `NoNewPrivileges=yes`、`ProtectSystem=strict`、`ProtectHome=yes`、`PrivateTmp=yes`
- `DynamicUser=yes` + `ConfigurationDirectory=go-reality`

## 7. 测试与验收

### 单元测试

- `config`：Load 成功/缺文件/YAML 错误/校验失败/env 覆盖优先级
- `keys`：UUID 格式、short_id 十六进制、X25519 公私钥配对
- `httpapi`：三个端点用 httptest 打（metrics 输出含 Prometheus 格式）

### 集成测试（build tag `integration`，默认不跑）

- 进程内再起一个 xray-core 客户端实例（同库、`vless` outbound + Reality TLS），经本地服务端 dial `ifconfig.me`，断言拿到出口 IP

### 验收清单

1. `make build` 产出可执行二进制
2. `bin/go-reality-server -config config.example.yaml` 启动并绑定 443
3. SIGTERM 优雅关闭 5s 内完成，无半开连接
4. `curl 127.0.0.1:9090/metrics` 返回 Prometheus 格式
5. Clash Meta / v2rayN / NekoBox 导入 vless 分享链接可直连成功

## 8. 已锁定的技术细节

- 密钥生成：`xray-core/common/uuid` + `xray-core/common/protocol`
- metrics：`github.com/prometheus/client_golang`
- YAML：`gopkg.in/yaml.v3`
- HTTP：标准库 `net/http`
- 日志：`log/slog` JSON handler
- VLESS flow 置空（REALITY 不用 flow）
- 不开 mux（REALITY 与 mux 不兼容）

## 9. 明确不做（YAGNI）

- Web 管理 UI、多用户/多 inbound、配置热重载（SIGHUP/HTTP reload）、TLS 证书管理、Docker 镜像、客户端订阅 API、流量限速/审计

## 10. 风险与对策

| 风险 | 对策 |
|---|---|
| xray-core API 跨版本变动 | go.mod 锁定版本；仅用 `core`、`proxy/vless`、`internet/tls`、`internet/reality` 公共导出 |
| 端口 443 与已有服务冲突 | 配置化端口 + 启动错误明确提示 |
| 客户端不兼容 | 分享链接采用 VLESS 通用格式，README 记录 Clash Meta / v2rayN / NekoBox 验证步骤 |
