# 服务器侧一键部署 + 权限修正 — 设计

日期:2026-09-24
状态:已获用户批准

## 背景与问题

当前部署链路断在中间:`deploy/bootstrap.sh` 期望一个"包目录"(内含预生成的
config.yaml、env、keys.txt),但仓库没有任何脚本负责组装该包;README 仍在教
7 步手工流程(交叉编译→scp 三件套→install.sh→手工填密钥→重启)。
`deploy/install.sh` 与 `bootstrap.sh` 职责重叠。

权限隐患:`/etc/go-reality/config.yaml` 装成 0644 全局可读,而 README 快速开始
引导用户把私钥直接填进 config.yaml——私钥对所有本机用户可读。

## 目标体验

```bash
# 开发机(一次):
make package          # → dist/go-reality-server-linux.tar.gz

# 服务器(两条命令):
tar xzf go-reality-server-linux.tar.gz
sudo ./bootstrap.sh   # 装二进制→现场生成密钥→启动→验证→打印客户端链接
```

密钥在服务器上现场生成、当场输出,永不经手开发机。重复执行幂等,不重置已有密钥。

## 组件设计

### 1. 打包:scripts/package.sh(新)+ Makefile `package` 目标

- 调 `scripts/cross-build.sh` 产 linux/amd64 与 linux/arm64 双二进制
- 组装 `dist/pkg/`:
  - `go-reality-server-linux-amd64` / `-arm64`
  - `bootstrap.sh`(自 deploy/ 拷入)
  - `go-reality.service`(自 deploy/systemd/ 拷入)
  - `config.yaml`(由 config.example.yaml 拷贝;内容无秘密)
- `tar czf dist/go-reality-server-linux.tar.gz -C dist/pkg .`
- 单文件 scp 上服务器

### 2. deploy/bootstrap.sh 重写(核心)

执行序(全程 `set -euo pipefail`,幂等):

1. root 检查(最先做)→ 自动识别架构(`uname -m` 映射 x86_64→amd64、
   aarch64→arm64;`--arch` 可覆盖;`--server <addr>` 指定分享链接地址)
2. `install -m 0755` 二进制 → `/usr/local/bin/go-reality-server`
3. 建目录;**若 `/etc/go-reality/env` 不存在** → 用刚装的二进制
   `go-reality-server keys generate -format env -server <出口IP>` 现场生成,
   写入 `/etc/go-reality/env`(0600)。出口 IP 默认取默认路由接口地址,
   `--server` 可覆盖
4. 装 `config.yaml`(0644,仅首次)+ systemd unit → `daemon-reload`;
   首次 `enable --now`,重跑时 `restart`
5. 防火墙:ufw(active 时)`allow 443/tcp`;firewalld(running 时)
   `--permanent --add-port=443/tcp && --reload`
6. 验证:轮询 `curl http://127.0.0.1:9090/ready`(总超时 ~15s);失败打印
   journalctl 指引并退出非零
7. 打印 ShareLink + Clash Meta 配置片段,并落盘
   `/etc/go-reality/client.txt`(0600 root)

### 3. Go 代码改动:keys generate 机器可读输出

`cmd/server/main.go` 的 `runKeysGenerate` 增加 `-format text|env|clash`:

- `text`(默认):现有人读格式,向后兼容
- `env`:`VLESS_UUID=...`、`VLESS_SHORT_ID=...`、`REALITY_PRIVATE_KEY=...`
  (及 `REALITY_PUBLIC_KEY=...` 供 bootstrap 组链接用)键值对,可直接落盘为
  EnvironmentFile
- `clash`:可直接粘贴的 Clash Meta proxies 配置块

顺带:main.go 的 `os.Args[1]=="keys"` 硬编码分发改为标准 `switch` 子命令
结构,便于后续扩展。

注意:env 输出里的额外键(如 REALITY_PUBLIC_KEY)需确认 env_override 只消费
已知键、多余键无害(现状即按名单映射,无害)。

### 4. 权限模型(修正后全表)

| 路径 | 权限 | 理由 |
|---|---|---|
| `/etc/go-reality/` | 0755 root | DynamicUser(随机 uid)需 traverse;固化 700→755 修复 |
| `config.yaml` | 0644 root | 内容无任何秘密(密钥全在 env);0640+group 对 DynamicUser 不可行(每次启动随机 GID) |
| `env` | 0600 root | 私钥唯一落点;systemd(PID 1,root)在降权前读 EnvironmentFile |
| `client.txt` | 0600 root | 含公钥+UUID+链接,root 才能看 |

## 清理

- 删除 `deploy/install.sh`(bootstrap 完全覆盖其职责)
- README:
  - 部署章节重写为 make package → scp → sudo ./bootstrap.sh 流程
  - 快速开始(开发机)不再引导"把密钥填进 config.yaml";敏感字段走 env 的
    安全指引保留并更新
  - 旧手工流程(交叉编译+scp 三件套+install.sh)移除
- Makefile:加 `package` 目标;`clean` 同时清 `dist/`

## 测试

- `keys -format`:Go 单测覆盖三种输出(格式正确、env 可解析出三个已知变量、
  clash 块含关键字段)
- `package.sh` / `bootstrap.sh`:`bash -n` 语法检查;package.sh 在本机
  (git-bash)全程可跑(内部就是 go 交叉编译 + tar)
- bootstrap.sh 的 Linux 运行时行为无法在本机真跑,靠幂等逻辑审读;README
  部署节注明验证步骤(`curl /ready`)

## 非目标

- 开发机侧 `make deploy HOST=...`(scp+ssh 编排)——用户明确选择仅服务器侧
- curl|bash 远程安装器(需发布渠道,暂无)
- 自动升级/版本管理
