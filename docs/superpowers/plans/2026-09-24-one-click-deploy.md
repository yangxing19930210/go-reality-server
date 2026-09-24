# 服务器侧一键部署 + 权限修正 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `make package` 产出单文件部署包;服务器上 `sudo ./bootstrap.sh` 一条命令完成部署(含现场生成密钥、打印分享链接);密钥只落 0600 env,config 无秘密。

**Architecture:** keys 子命令增加 `-format text|env|clash` 机器可读输出(env 格式用 systemd EnvironmentFile 合法的 `#` 注释附带分享链接,单次调用同时产出服务端密钥与客户端资料);bootstrap.sh 重写为幂等的装→生成→启→验→打印流程;package.sh 组装双架构二进制+脚本为 tar.gz;删除旧 install.sh,README 重写。

**Tech Stack:** Go 1.22+(仅标准库+现有依赖)、bash(`set -euo pipefail`)、systemd、Makefile。

**Spec:** `docs/superpowers/specs/2026-09-24-one-click-deploy-design.md`

## Global Constraints

- 所有 shell 脚本 LF 结尾(`.gitattributes` 已覆盖 `*.sh`,勿改)
- 脚本统一 `set -euo pipefail`;注释与用户输出用中文,风格与现有 `deploy/bootstrap.sh` 一致
- `-format` 默认 `text`,输出与现状完全一致(向后兼容,现有测试不得改断言)
- 权限模型:`/etc/go-reality/` 0755、`config.yaml` 0644(无秘密)、`env` 0600、`client.txt` 0600,全部 root:root
- 开发机为 Windows git-bash:package.sh 只用 `cp/chmod/tar`(不用 `install`,git-bash 无此命令);`install` 仅在服务器侧脚本用
- 环境变量覆盖名单见 `internal/config/env_override.go`:env 文件里多余键(如 `REALITY_PUBLIC_KEY`)不被消费,无害
- Go 代码改动仅限 `cmd/server/`;`internal/` 一律不动

---

### Task 1: `keys generate` 增加 `-format text|env|clash`

**Files:**
- Create: `cmd/server/keys_format.go`
- Modify: `cmd/server/main.go:25-38`(子命令分发)、`cmd/server/main.go:107-133`(runKeysGenerate)
- Test: `cmd/server/keys_format_test.go`(新建)

**Interfaces:**
- Produces(TASK 2 的 bootstrap.sh 依赖此输出契约):
  - `-format text`(默认):现有 `UUID: ...` 四行 + 有 `-server` 时 `ShareLink: vless://...`
  - `-format env`:恰好 4 行 `KEY=VALUE`(`VLESS_UUID`、`VLESS_SHORT_ID`、`REALITY_PRIVATE_KEY`、`REALITY_PUBLIC_KEY`);带 `-server` 时追加 `#` 注释区:`# ShareLink: vless://...`、`# --- Clash Meta proxies: ---`、`# proxies:` 及逐行 `# ` 前缀的 clash 条目
  - `-format clash`:要求必须给 `-server`(否则报错);输出 `proxies:` + 缩进条目 + 尾行 `# ShareLink: ...`
  - Go 函数:`renderKeysText(id, sid string, kp *proxy.KeyPair) string`、`renderKeysEnv(id, sid string, kp *proxy.KeyPair) string`、`renderClashProxy(server string, port int, id, sid, pubKeyB64, sni, name string) string`

- [ ] **Step 1: 写失败测试**

创建 `cmd/server/keys_format_test.go`:

```go
// cmd/server/keys_format_test.go
package main

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"go-reality-server/internal/proxy"
)

func genMaterial(t *testing.T) (string, string, *proxy.KeyPair) {
	t.Helper()
	kp, err := proxy.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	return proxy.GenerateUUID(), proxy.GenerateShortID(), kp
}

func TestRenderKeysEnv(t *testing.T) {
	id, sid, kp := genMaterial(t)
	out := renderKeysEnv(id, sid, kp)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("env format must be exactly 4 lines, got %d:\n%s", len(lines), out)
	}
	want := map[string]*regexp.Regexp{
		"VLESS_UUID":         regexp.MustCompile(`^VLESS_UUID=` + id + `$`),
		"VLESS_SHORT_ID":     regexp.MustCompile(`^VLESS_SHORT_ID=` + sid + `$`),
		"REALITY_PRIVATE_KEY": regexp.MustCompile(`^REALITY_PRIVATE_KEY=` + kp.PrivateKeyB64() + `$`),
		"REALITY_PUBLIC_KEY":  regexp.MustCompile(`^REALITY_PUBLIC_KEY=` + kp.PublicKeyB64() + `$`),
	}
	for _, line := range lines {
		key := strings.SplitN(line, "=", 2)[0]
		if re, ok := want[key]; !ok || !re.MatchString(line) {
			t.Errorf("bad line %q", line)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Errorf("missing keys: %v", want)
	}
}

func TestRenderClashProxy(t *testing.T) {
	out := renderClashProxy("1.2.3.4", 443, "uuid-x", "sid-x", "pbk-x", "www.apple.com", "n1")
	for _, sub := range []string{
		"- name: \"n1\"", "type: vless", "server: 1.2.3.4", "port: 443",
		"uuid: uuid-x", "servername: www.apple.com",
		"public-key: pbk-x", "short-id: sid-x", "client-fingerprint: chrome",
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("clash block missing %q:\n%s", sub, out)
		}
	}
}

func TestRunKeysGenerateFormatEnvWithServer(t *testing.T) {
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, []string{"-format", "env", "-server", "1.2.3.4"}); err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	out := buf.String()
	envLines := []string{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if !strings.HasPrefix(line, "#") {
			envLines = append(envLines, line)
		}
	}
	if len(envLines) != 4 || !strings.HasPrefix(envLines[0], "VLESS_UUID=") {
		t.Errorf("non-comment lines must be exactly the 4 KEY=VALUE lines:\n%s", out)
	}
	if !strings.Contains(out, "# ShareLink: vless://") {
		t.Errorf("env+server must carry share link as comment:\n%s", out)
	}
	if !strings.Contains(out, "# proxies:") || !strings.Contains(out, "#   - name:") {
		t.Errorf("env+server must carry clash block as comments:\n%s", out)
	}
}

func TestRunKeysGenerateFormatEnvNoServer(t *testing.T) {
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, []string{"-format", "env"}); err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	if strings.Contains(buf.String(), "#") {
		t.Errorf("env without -server must have no comment lines:\n%s", buf.String())
	}
}

func TestRunKeysGenerateFormatClash(t *testing.T) {
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, []string{"-format", "clash", "-server", "1.2.3.4"}); err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "proxies:\n") {
		t.Errorf("clash format must start with 'proxies:':\n%s", out)
	}
	if !strings.Contains(out, "type: vless") || !strings.Contains(out, "# ShareLink: vless://") {
		t.Errorf("clash format missing fields:\n%s", out)
	}
}

func TestRunKeysGenerateFormatErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, []string{"-format", "xml"}); err == nil {
		t.Error("unknown format must error")
	}
	if err := runKeysGenerate(&buf, []string{"-format", "clash"}); err == nil {
		t.Error("clash without -server must error")
	}
}
```

- [ ] **Step 2: 运行确认失败**

```bash
cd "C:\Users\Administrator\Desktop\go-reality-server" && go test ./cmd/server/ -run 'TestRender|TestRunKeysGenerateFormat' -v
```

预期:编译失败,`undefined: renderKeysEnv`、`undefined: renderClashProxy`(以及 `-format` 参数被忽略导致的断言失败)。

- [ ] **Step 3: 实现 keys_format.go**

创建 `cmd/server/keys_format.go`:

```go
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
```

- [ ] **Step 4: 改 main.go——子命令分发与 -format**

`cmd/server/main.go` 的 `main()`(25-38 行)改为:

```go
func main() {
	// 子命令分发:目前只有 keys;其余参数交给 run 的 flag 解析。
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keys":
			if err := runKeysGenerate(os.Stdout, keysGenerateArgs(os.Args[2:])); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				os.Exit(2)
			}
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

`runKeysGenerate`(替换原 107-133 行整体):

```go
// runKeysGenerate 生成 UUID/short_id/X25519 密钥对并按 -format 渲染。
// env 格式带 -server 时以 '#' 注释区附分享链接与 Clash 块——'#' 是
// systemd EnvironmentFile 的合法注释语法,bootstrap 将非注释行原样落盘
// 为 env 文件、注释行剥前缀后归入 client.txt,单次调用两用。
func runKeysGenerate(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("keys generate", flag.ContinueOnError)
	server := fs.String("server", "", "server address for share link, e.g. 1.2.3.4")
	port := fs.Int("port", 443, "server port for share link")
	sni := fs.String("sni", "www.apple.com", "SNI/server_name for share link")
	name := fs.String("name", "go-reality-server", "name for share link")
	format := fs.String("format", "text", "output format: text | env | clash")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *format == "clash" && *server == "" {
		return fmt.Errorf("-format clash 需要同时指定 -server")
	}

	id := proxy.GenerateUUID()
	sid := proxy.GenerateShortID()
	kp, err := proxy.GenerateKeyPair()
	if err != nil {
		return err
	}
	link := ""
	if *server != "" {
		link = proxy.GenerateShareLink(*server, *port, id, sid, kp.PublicKeyB64(), *sni, *name)
	}

	switch *format {
	case "text":
		fmt.Fprint(w, renderKeysText(id, sid, kp))
		if link != "" {
			fmt.Fprintf(w, "ShareLink:   %s\n", link)
		}
	case "env":
		fmt.Fprint(w, renderKeysEnv(id, sid, kp))
		if link != "" {
			fmt.Fprintf(w, "# ShareLink: %s\n", link)
			fmt.Fprintf(w, "# --- Clash Meta proxies: ---\n# proxies:\n")
			for _, l := range strings.Split(renderClashProxy(*server, *port, id, sid, kp.PublicKeyB64(), *sni, *name), "\n") {
				fmt.Fprintf(w, "# %s\n", l)
			}
		}
	case "clash":
		fmt.Fprintf(w, "proxies:\n%s\n", renderClashProxy(*server, *port, id, sid, kp.PublicKeyB64(), *sni, *name))
		fmt.Fprintf(w, "# ShareLink: %s\n", link)
	default:
		return fmt.Errorf("unknown -format %q (text | env | clash)", *format)
	}
	return nil
}
```

注意:`main.go` 的 import 需补 `"strings"`。

- [ ] **Step 5: 运行新测试与全量旧测试**

```bash
go test ./cmd/server/ -v
```

预期:全部 PASS——包括原有 `TestRunKeysGenerate`/`TestRunKeysGenerateWithShareLink`/`TestKeysGenerateArgs`(text 默认输出未变,不改这些断言)。

- [ ] **Step 6: 提交**

```bash
git add cmd/server/keys_format.go cmd/server/keys_format_test.go cmd/server/main.go
git commit -m "feat: keys generate -format text|env|clash——env 可直接落盘 EnvironmentFile

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: deploy/bootstrap.sh 重写(服务器一条命令)

**Files:**
- Modify: `deploy/bootstrap.sh`(整文件重写)

**Interfaces:**
- Consumes: Task 1 的 `-format env` 输出契约(非注释行=4 KEY=VALUE;`#` 注释区含 `ShareLink: vless://...` 与 clash 块);包内同目录文件 `go-reality-server-linux-<amd64|arm64>`、`config.yaml`、`go-reality.service`(由 Task 3 的 package.sh 组装)
- Consumes: `curl http://127.0.0.1:9090/ready` 返回 200 `ready`(`internal/httpapi/server.go:53`)
- Produces: `/etc/go-reality/env`(0600)、`/etc/go-reality/client.txt`(0600)、systemd 服务 `go-reality`

- [ ] **Step 1: 重写 deploy/bootstrap.sh**

整文件替换为:

```bash
#!/usr/bin/env bash
# 服务器一条命令部署:装二进制 →(首次)现场生成密钥 → systemd 启动 →
# 防火墙放行 → 验证 /ready → 打印客户端分享链接。
# 用法:
#   sudo ./bootstrap.sh                     # 架构自动识别,出口 IP 自动探测
#   sudo ./bootstrap.sh --arch arm64        # 手动指定架构
#   sudo ./bootstrap.sh --server 1.2.3.4    # 手动指定分享链接中的服务器地址
# 幂等:重复运行不会重置已有密钥与配置。
set -euo pipefail

PKG_DIR="$(cd "$(dirname "$0")" && pwd)"
BIN_DST="/usr/local/bin/go-reality-server"
CONF_DIR="/etc/go-reality"
ARCH=""
SERVER_ADDR=""
SERVER_GIVEN=0

usage() { sed -n '3,7p' "$0" | sed 's/^# \{0,1\}//'; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --arch)     ARCH="${2:?--arch 需要值}"; shift 2 ;;
    --arch=*)   ARCH="${1#--arch=}"; shift ;;
    --server)   SERVER_ADDR="${2:?--server 需要值}"; SERVER_GIVEN=1; shift 2 ;;
    --server=*) SERVER_ADDR="${1#--server=}"; SERVER_GIVEN=1; shift ;;
    -h|--help)  usage; exit 0 ;;
    *) echo "未知参数: $1" >&2; exit 2 ;;
  esac
done

[[ $EUID -eq 0 ]] || { echo "❌ 请以 root 运行:sudo ./bootstrap.sh"; exit 1; }

if [[ -z "$ARCH" ]]; then
  case "$(uname -m)" in
    x86_64)  ARCH=amd64 ;;
    aarch64) ARCH=arm64 ;;
    *) echo "❌ 无法识别架构 $(uname -m),请用 --arch amd64 或 --arch arm64 指定" >&2; exit 1 ;;
  esac
fi
BIN_SRC="$PKG_DIR/go-reality-server-linux-$ARCH"
if [[ ! -f "$BIN_SRC" ]]; then
  echo "❌ 没找到 $BIN_SRC" >&2
  ls "$PKG_DIR"/go-reality-server-linux-* 2>/dev/null | sed 's/^/   - /' >&2
  exit 1
fi

echo "==> 安装二进制 $BIN_DST"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> 准备 $CONF_DIR(0755:DynamicUser 每次启动随机 uid,须可进入)"
mkdir -p "$CONF_DIR"
chmod 0755 "$CONF_DIR"

# config.yaml:内容无秘密(密钥全走 env),仅首次写入
if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  [[ -f "$PKG_DIR/config.yaml" ]] || { echo "❌ 包内缺 config.yaml" >&2; exit 1; }
  install -m 0644 "$PKG_DIR/config.yaml" "$CONF_DIR/config.yaml"
fi

# env:密钥唯一落点。仅首次生成:0600 root,systemd 降权前以 root 读取
# 警告判定用:脚本启动时 env 是否已存在(生成后无法区分本轮生成还是早已存在)
ENV_PREEXISTED=0
if [[ -f "$CONF_DIR/env" ]]; then ENV_PREEXISTED=1; fi
if [[ ! -f "$CONF_DIR/env" ]]; then
  if [[ -z "$SERVER_ADDR" ]]; then
    SERVER_ADDR="$(ip route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}')" || true
  fi
  if [[ -z "$SERVER_ADDR" ]]; then
    echo "❌ 无法探测服务器出口 IP,请显式指定:sudo ./bootstrap.sh --server <IP或域名>" >&2
    exit 1
  fi
  echo "==> 现场生成密钥(链接地址 $SERVER_ADDR)"
  umask 077
  "$BIN_DST" keys generate -format env -server "$SERVER_ADDR" > "$CONF_DIR/env.new"
  chmod 0600 "$CONF_DIR/env.new"
  mv "$CONF_DIR/env.new" "$CONF_DIR/env"
  # 客户端资料 = env 注释区剥去 '# ' 前缀,同样 0600 root
  grep '^#' "$CONF_DIR/env" | sed 's/^# \{0,1\}//' > "$CONF_DIR/client.txt" || true
  chmod 0600 "$CONF_DIR/client.txt"
  umask 022
fi

# 用户裁决修正(2026-09-24):env 已存在时 --server 无效,须明示而非静默。
# 双重条件:ENV_PREEXISTED(启动快照,防首跑误报)+ SERVER_GIVEN(显式标志,
# SERVER_ADDR 会被自动探测填充)
if [[ "$ENV_PREEXISTED" -eq 1 && "$SERVER_GIVEN" -eq 1 ]]; then
  echo "⚠️ 密钥已存在($CONF_DIR/env),--server $SERVER_ADDR 未生效;如需换新密钥与链接:rm $CONF_DIR/env 后重跑" >&2
fi

echo "==> 安装 systemd unit"
[[ -f "$PKG_DIR/go-reality.service" ]] || { echo "❌ 包内缺 go-reality.service" >&2; exit 1; }
install -m 0644 "$PKG_DIR/go-reality.service" /etc/systemd/system/go-reality.service
systemctl daemon-reload
if systemctl is-enabled --quiet go-reality 2>/dev/null; then
  systemctl restart go-reality
else
  systemctl enable --now go-reality
fi

echo "==> 防火墙放行 443/tcp(仅当防火墙在运行)"
# 用户裁决修正(2026-09-24):ufw 输出会被 gettext 本地化,zh_CN 主机匹配不到
# "Status: active",须 LC_ALL=C 强制英文输出
if command -v ufw >/dev/null && LC_ALL=C ufw status 2>/dev/null | grep -q "Status: active"; then
  ufw allow 443/tcp
elif command -v firewall-cmd >/dev/null && firewall-cmd --state 2>/dev/null | grep -q running; then
  firewall-cmd --permanent --add-port=443/tcp
  firewall-cmd --reload
fi

echo "==> 等待服务就绪(最多 15s)"
ok=""
for _ in $(seq 1 15); do
  if curl -fsS http://127.0.0.1:9090/ready >/dev/null 2>&1; then ok=1; break; fi
  sleep 1
done
if [[ -z "$ok" ]]; then
  echo "❌ 服务未就绪。日志:journalctl -u go-reality -n 50 --no-pager" >&2
  systemctl status go-reality --no-pager >&2 || true
  exit 1
fi

echo ""
echo "✅ 部署完成"
echo "   状态:   systemctl status go-reality --no-pager"
echo "   就绪:   curl http://127.0.0.1:9090/ready"
echo "   日志:   journalctl -u go-reality -f"
if [[ -f "$CONF_DIR/client.txt" ]]; then
  echo "   客户端: sudo cat $CONF_DIR/client.txt"
  SHARE="$(grep -m1 -o 'vless://[^ ]*' "$CONF_DIR/client.txt" || true)"
  if [[ -n "$SHARE" ]]; then
    echo ""
    echo "   分享链接(可直接导入 Clash Meta / v2rayN / NekoBox):"
    echo "   $SHARE"
  fi
fi
```

- [ ] **Step 2: 语法与静态检查**

```bash
bash -n deploy/bootstrap.sh && echo SYNTAX-OK
command -v shellcheck >/dev/null && shellcheck deploy/bootstrap.sh || echo "shellcheck 不可用,跳过"
```

预期:`SYNTAX-OK`。

- [ ] **Step 3: 逻辑自检(本机无 systemd,仅验证解析分支)**

```bash
./deploy/bootstrap.sh --help; echo "exit=$?"
bash -c 'set -euo pipefail; [[ 1 -eq 1 ]] || exit 1; echo arg-parse-ok'
```

预期:打印用法注释、`exit=0`(help 分支在任何安装动作之前返回,不碰系统)、`arg-parse-ok`。

- [ ] **Step 4: 提交**

```bash
git add deploy/bootstrap.sh
git commit -m "feat: bootstrap.sh 重写——服务器一条命令部署,密钥现场生成,权限模型固化

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: scripts/package.sh + Makefile `package` + .gitignore

**Files:**
- Create: `scripts/package.sh`
- Modify: `Makefile`(.PHONY、package 目标、clean)
- Modify: `.gitignore`(加 `/dist/`)

**Interfaces:**
- Consumes: `scripts/cross-build.sh`(产 `bin/go-reality-server-linux-{amd64,arm64}`)、`deploy/bootstrap.sh`(Task 2)、`deploy/systemd/go-reality.service`、`config.example.yaml`
- Produces: `dist/go-reality-server-linux.tar.gz`,解包为 `go-reality-server/` 目录,内含:两个架构二进制(0755)、`bootstrap.sh`(0755)、`go-reality.service`(0644)、`config.yaml`(0644,无秘密)

- [ ] **Step 1: 创建 scripts/package.sh**

```bash
#!/usr/bin/env bash
# 组装服务器部署包:双架构二进制 + bootstrap.sh + systemd unit + 无秘密
# config.yaml,打成单文件 dist/go-reality-server-linux.tar.gz(一个 scp)。
set -euo pipefail
cd "$(dirname "$0")/.."

echo "==> 交叉编译 linux/amd64 与 linux/arm64"
./scripts/cross-build.sh

PKG=dist/go-reality-server
echo "==> 组装 $PKG/"
rm -rf "$PKG" dist/go-reality-server-linux.tar.gz
mkdir -p "$PKG"
# 注意:不用 install——开发机可能是 git-bash(无 coreutils install)
cp bin/go-reality-server-linux-amd64 bin/go-reality-server-linux-arm64 "$PKG/"
chmod 0755 "$PKG"/go-reality-server-linux-*
cp deploy/bootstrap.sh "$PKG/bootstrap.sh"
chmod 0755 "$PKG/bootstrap.sh"
cp deploy/systemd/go-reality.service "$PKG/"
cp config.example.yaml "$PKG/config.yaml"

echo "==> 打包 dist/go-reality-server-linux.tar.gz"
tar czf dist/go-reality-server-linux.tar.gz -C dist go-reality-server

echo "✅ dist/go-reality-server-linux.tar.gz"
echo "   服务器: scp 上去后 tar xzf && cd go-reality-server && sudo ./bootstrap.sh"
```

- [ ] **Step 2: Makefile 加 package 目标**

`Makefile` 全文替换为:

```make
BINARY := bin/go-reality-server

.PHONY: build test test-integration cross package fmt vet gen-keys clean

build:
	go build -trimpath -o $(BINARY) ./cmd/server

test:
	go test ./...

test-integration:
	go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v

cross:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server

package:
	./scripts/package.sh

fmt:
	gofmt -w .

vet:
	go vet ./...

gen-keys:
	go run ./cmd/server keys generate

clean:
	rm -rf bin dist
```

- [ ] **Step 3: .gitignore 加 dist/**

`.gitignore` 追加一行 `/dist/`(全文变为):

```
/bin/
*.exe
config.yaml
/dist/
```

- [ ] **Step 4: 真实运行验证(本机 git-bash 可全程执行)**

```bash
chmod +x scripts/package.sh 2>/dev/null; ./scripts/package.sh
tar -tzf dist/go-reality-server-linux.tar.gz
```

预期:package.sh 依次输出交叉编译两架构、组装、打包;`tar -tzf` 列出 5 个文件(`go-reality-server/` 前缀 + 两个二进制 + bootstrap.sh + go-reality.service + config.yaml)。

- [ ] **Step 5: 抽查包内脚本头(确认 LF 与内容)**

```bash
tar -xzf dist/go-reality-server-linux.tar.gz -O go-reality-server/bootstrap.sh | head -8
```

预期:输出重写后的 bootstrap.sh 头部注释,无 `^M`。

- [ ] **Step 6: 提交**

```bash
git add scripts/package.sh Makefile .gitignore
git commit -m "feat: make package——组装单文件服务器部署包(双架构+bootstrap+unit+无秘密config)

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: 删除 install.sh、config.example.yaml 注释、README 重写

**Files:**
- Delete: `deploy/install.sh`
- Modify: `config.example.yaml:1-2`(头部注释)
- Modify: `README.md`(部署章节、快速开始、安全建议)

**Interfaces:**
- Consumes: Task 1-3 产出的最终流程(`make package` → `sudo ./bootstrap.sh`;`keys generate -format env`;`/etc/go-reality/client.txt`)

- [ ] **Step 1: 删除 deploy/install.sh**

```bash
git rm deploy/install.sh
```

- [ ] **Step 2: 更新 config.example.yaml 头部注释**

前两行:

```yaml
# go-reality-server 示例配置
# 复制为 config.yaml 并填入密钥:go run ./cmd/server keys generate
```

替换为:

```yaml
# go-reality-server 示例配置
# 密钥(uuid/short_id/private_key)建议经环境变量注入、不落本文件:
#   go run ./cmd/server keys generate -format env
# 环境变量覆盖名单见 README;服务器部署由 deploy/bootstrap.sh 全自动处理。
```

- [ ] **Step 3: 重写 README.md**

`README.md` 全文替换为:

````markdown
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
````

- [ ] **Step 4: 确认无 install.sh 残留引用**

```bash
grep -rn "install.sh" --include="*.md" --include="*.sh" --include="Makefile" --include="*.yaml" . | grep -v docs/superpowers || echo "无残留"
```

预期:`无残留`(docs/superpowers 下的历史 spec/plan 不改,那是存档)。

- [ ] **Step 5: 提交**

```bash
git add -A
git commit -m "docs: README 重写为一键部署流程;删除 install.sh;config 示例密钥走 env

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: 全量验证

**Files:**
- 无新改动(只跑验证;发现问题回上游任务修)

**Interfaces:**
- Consumes: Task 1-4 的全部产物

- [ ] **Step 1: Go 全量测试 + 静态检查**

```bash
gofmt -l . && go vet ./... && go test ./...
```

预期:`gofmt -l` 无输出(全部已格式化)、vet 与全部测试 PASS。

- [ ] **Step 2: 端到端演练(本机可跑的部分)**

```bash
rm -rf dist && ./scripts/package.sh
tar -xzf dist/go-reality-server-linux.tar.gz -C /tmp/
bash -n /tmp/go-reality-server/bootstrap.sh && echo BOOTSTRAP-SYNTAX-OK
# 用包内 amd64 二进制验证 -format env 输出契约(Windows 上跑本机版二进制):
go run ./cmd/server keys generate -format env -server 1.2.3.4 | grep -c '^[^#]'   # 预期: 4
go run ./cmd/server keys generate -format env -server 1.2.3.4 | grep -c '^# ShareLink'  # 预期: 1
```

预期:`BOOTSTRAP-SYNTAX-OK`;两个计数分别为 4 与 1。

- [ ] **Step 3: 模拟 bootstrap 的 client.txt 抽取逻辑**

```bash
go run ./cmd/server keys generate -format env -server 1.2.3.4 \
  | grep '^#' | sed 's/^# \{0,1\}//' | head -5
```

预期:首行 `ShareLink: vless://...`,其后 `--- Clash Meta proxies: ---`、`proxies:`、`  - name: ...`——即 bootstrap.sh 写入 client.txt 的内容形态。

- [ ] **Step 4: 收尾提交(如验证中产生修正)**

若前面任务有遗漏导致本任务修改了文件,逐项修复并:

```bash
git add -A
git commit -m "fix: 全量验证修正

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

无修改则跳过本步。
