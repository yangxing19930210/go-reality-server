# go-reality-server 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 一个嵌入 xray-core 的 VLESS+REALITY 代理服务端二进制,配置来自 YAML+env,暴露 /metrics /healthz /ready,可被 systemd 守护,附密钥生成子命令与交叉编译脚本。

**Architecture:** 单 Go module `go-reality-server`(本地模块)。`internal/config` 负责配置装载与校验;`internal/proxy` 拼装 xray-core proto 并包装实例生命周期;`internal/httpapi` 提供健康检查与 Prometheus 指标;`cmd/server` 装配并处理信号;集成测试用进程内第二个 xray 实例当客户端跑真实出网。

**Tech Stack:** Go 1.27(本机 1.27.1);github.com/xtls/xray-core v26.3.27(固定);gopkg.in/yaml.v3 v3.0.1;github.com/prometheus/client_golang;标准库 net/http + log/slog。

## Global Constraints

- 模块名 `go-reality-server`,Go 指令 `go 1.27`。xray-core 固定在 `v26.3.27`。
- 只允许 import xray-core 的公开包:`core`、`app/dispatcher`、`app/proxyman`、`proxy/vless`(+`/inbound`、`/outbound`)、`proxy/freedom`、`proxy/dokodemo`(仅集成测试)、`common/net`、`common/protocol`、`common/serial`、`common/uuid`、`transport/internet`(+`/reality`)。
- VLESS flow 置空(REALITY 不用 flow);不开 mux;不配 fallback。
- 每个 commit message 必须以 `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>` 结尾。
- 与 spec 的三处偏差(经查证 xray-core 实际 API 后确定,已在计划中实现):
  1. spec §4.3 的 `Engine.Wait()` 省略——`core.Instance` 没有运行期错误通道,`Start()` 返回即全部启动,main 直接阻塞信号。
  2. Reality 私钥编码:生成用 base64 RawURLEncoding(xray CLI 同款),读取时 RawURL 失败再试 Std,两种都接受。
  3. metrics 只实现自定义 collector + Go runtime 指标,不接 Xray stats.Manager(spec 允许降级,YAGNI)。
- 运行环境是 Windows Git Bash;所有验证命令均为 `go` 命令(不依赖 make,make 只是便利封装)。
- 测试绑定 127.0.0.1 高位随机端口,不碰 443,不要求管理员。

---

### Task 1: config 包 + 项目骨架

**Files:**
- Create: `go.mod`(go mod init 生成)、`.gitignore`、`LICENSE`、`config.example.yaml`
- Create: `internal/config/config.go`
- Create: `internal/config/env_override.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: 无
- Produces(后续任务依赖,签名不可变):
  - `func Defaults() ServerConfig`
  - `func Load(path string) (*ServerConfig, error)`
  - `func (c *ServerConfig) Validate() error`
  - `func (r RealityConfig) PrivateKeyBytes() ([]byte, error)` — 私钥字符串 → 32 字节
  - `func (c *ServerConfig) ShortIDBytesList() ([][]byte, error)` — reality.short_ids 为空时回退 vless.short_id
  - 结构体 `ServerConfig{ListenAddress string; ListenPort int; VLESS VLESSConfig; Reality RealityConfig; HTTPAPI HTTPAPIConfig; LogLevel string}` 及子结构体,字段与 config.example.yaml 一一对应

- [ ] **Step 1: 初始化模块与依赖**

```bash
cd /c/Users/Administrator/Desktop/go-reality-server
go mod init go-reality-server
go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: 写失败测试**

```go
// internal/config/config_test.go
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// validPrivKey 返回 32 个零字节的合法私钥 base64url 串(由标准库生成,不手算)。
func validPrivKey() string {
	return base64.RawURLEncoding.EncodeToString(make([]byte, 32))
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func minimalYAML() string {
	return fmt.Sprintf("vless:\n  uuid: 93a6f3b4-71a2-4a11-b7bf-e73a6a9b43bf\n  short_id: \"0123456789abcdef\"\nreality:\n  private_key: %s\n", validPrivKey())
}

func TestLoadDefaults(t *testing.T) {
	p := writeTemp(t, minimalYAML())
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenPort != 443 {
		t.Errorf("ListenPort = %d, want 443", cfg.ListenPort)
	}
	if cfg.ListenAddress != "::" {
		t.Errorf("ListenAddress = %q, want \"::\"", cfg.ListenAddress)
	}
	if cfg.Reality.Dest != "www.microsoft.com:443" {
		t.Errorf("Dest = %q, want default", cfg.Reality.Dest)
	}
	if cfg.HTTPAPI.ListenPort != 9090 {
		t.Errorf("HTTPAPI.ListenPort = %d, want 9090", cfg.HTTPAPI.ListenPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	p := writeTemp(t, minimalYAML())
	t.Setenv("LISTEN_PORT", "8443")
	t.Setenv("VLESS_UUID", "11111111-2222-3333-4444-555555555555")
	t.Setenv("LOG_LEVEL", "debug")
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ListenPort != 8443 {
		t.Errorf("ListenPort = %d, want 8443 (env wins)", cfg.ListenPort)
	}
	if cfg.VLESS.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("UUID = %q, want env value", cfg.VLESS.UUID)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want debug", cfg.LogLevel)
	}
}

func TestLoadMissingFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("Load of missing file should fail")
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*ServerConfig)
	}{
		{"bad uuid", func(c *ServerConfig) { c.VLESS.UUID = "not-a-uuid" }},
		{"empty short_id", func(c *ServerConfig) { c.VLESS.ShortID = "" }},
		{"bad short_id length", func(c *ServerConfig) { c.VLESS.ShortID = "abcd" }},
		{"bad private key", func(c *ServerConfig) { c.Reality.PrivateKey = "!!!" }},
		{"port out of range", func(c *ServerConfig) { c.ListenPort = 0 }},
		{"bad flow", func(c *ServerConfig) { c.VLESS.Flow = "xtls-rxtls-splice" }},
		{"empty server_names", func(c *ServerConfig) { c.Reality.ServerNames = []string{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.VLESS.UUID = "93a6f3b4-71a2-4a11-b7bf-e73a6a9b43bf"
			cfg.VLESS.ShortID = "0123456789abcdef"
			cfg.Reality.PrivateKey = validPrivKey()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatalf("Validate() should fail for %s", tc.name)
			}
		})
	}
}

func TestPrivateKeyBytes(t *testing.T) {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = byte(i) // 非零字节,避免全零样本掩盖解码错误
	}
	std := base64.StdEncoding.EncodeToString(raw)
	rawURL := base64.RawURLEncoding.EncodeToString(raw)

	for name, s := range map[string]string{"std": std, "rawurl": rawURL} {
		t.Run(name, func(t *testing.T) {
			b, err := (RealityConfig{PrivateKey: s}).PrivateKeyBytes()
			if err != nil {
				t.Fatalf("%s encoding: %v", name, err)
			}
			if len(b) != 32 || string(b) != string(raw) {
				t.Fatalf("decoded = %v, want %v", b, raw)
			}
		})
	}
}

func TestPrivateKeyBytesWrongLen(t *testing.T) {
	short := base64.RawURLEncoding.EncodeToString(make([]byte, 16))
	if _, err := (RealityConfig{PrivateKey: short}).PrivateKeyBytes(); err == nil {
		t.Fatal("16-byte key should be rejected")
	}
}

func TestShortIDBytesListFallback(t *testing.T) {
	cfg := Defaults()
	cfg.VLESS.ShortID = "0123456789abcdef"
	list, err := cfg.ShortIDBytesList()
	if err != nil {
		t.Fatalf("ShortIDBytesList: %v", err)
	}
	if len(list) != 1 || len(list[0]) != 8 {
		t.Fatalf("fallback list = %v, want one 8-byte id", list)
	}
	cfg.Reality.ShortIDs = []string{"0123456789abcdef", "fedcba9876543210"}
	list, err = cfg.ShortIDBytesList()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("explicit list len = %d, want 2", len(list))
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./internal/config/
```
Expected: FAIL — `undefined: Defaults` 等编译错误。

- [ ] **Step 4: 实现 config.go**

```go
// Package config 负责服务端配置的装载、默认值、环境变量覆盖与校验。
package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

type VLESSConfig struct {
	UUID    string `yaml:"uuid"`
	Flow    string `yaml:"flow"`
	ShortID string `yaml:"short_id"`
}

type RealityConfig struct {
	PrivateKey  string   `yaml:"private_key"`
	Dest        string   `yaml:"dest"`
	ServerNames []string `yaml:"server_names"`
	ShortIDs    []string `yaml:"short_ids"`
	X25519Pub   string   `yaml:"x25519_pub"`
}

type HTTPAPIConfig struct {
	ListenAddress string `yaml:"listen_address"`
	ListenPort    int    `yaml:"listen_port"`
}

type ServerConfig struct {
	ListenAddress string        `yaml:"listen_address"`
	ListenPort    int           `yaml:"listen_port"`
	VLESS         VLESSConfig   `yaml:"vless"`
	Reality       RealityConfig `yaml:"reality"`
	HTTPAPI       HTTPAPIConfig `yaml:"http_api"`
	LogLevel      string        `yaml:"log_level"`
}

var (
	uuidPattern    = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	shortIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{16}$`)
)

// Defaults 返回内置默认配置,Load 在此之上叠加 YAML 与 env。
func Defaults() ServerConfig {
	return ServerConfig{
		ListenAddress: "::",
		ListenPort:    443,
		Reality: RealityConfig{
			Dest:        "www.microsoft.com:443",
			ServerNames: []string{"www.microsoft.com"},
		},
		HTTPAPI: HTTPAPIConfig{
			ListenAddress: "127.0.0.1",
			ListenPort:    9090,
		},
		LogLevel: "info",
	}
}

// Load 读 YAML、叠加环境变量、校验,任何一步失败都返回错误(fail fast)。
func Load(path string) (*ServerConfig, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if err := applyEnv(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

// Validate 检查所有必填项与格式。校验失败即启动失败。
func (c *ServerConfig) Validate() error {
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return fmt.Errorf("listen_port %d out of range 1-65535", c.ListenPort)
	}
	if c.HTTPAPI.ListenPort < 0 || c.HTTPAPI.ListenPort > 65535 {
		return fmt.Errorf("http_api.listen_port %d out of range 0-65535", c.HTTPAPI.ListenPort)
	}
	if net.ParseIP(c.ListenAddress) == nil {
		return fmt.Errorf("listen_address %q is not an IP literal", c.ListenAddress)
	}
	if !uuidPattern.MatchString(c.VLESS.UUID) {
		return fmt.Errorf("vless.uuid %q is not a valid UUID", c.VLESS.UUID)
	}
	if c.VLESS.Flow != "" && c.VLESS.Flow != "xtls-rprx-vision" {
		return fmt.Errorf("vless.flow %q unsupported (REALITY 应留空)", c.VLESS.Flow)
	}
	if !shortIDPattern.MatchString(c.VLESS.ShortID) {
		return fmt.Errorf("vless.short_id %q must be 16 hex chars", c.VLESS.ShortID)
	}
	if _, err := c.Reality.PrivateKeyBytes(); err != nil {
		return err
	}
	if _, _, err := net.SplitHostPort(c.Reality.Dest); err != nil {
		return fmt.Errorf("reality.dest %q must be host:port: %w", c.Reality.Dest, err)
	}
	if len(c.Reality.ServerNames) == 0 {
		return fmt.Errorf("reality.server_names must not be empty")
	}
	for _, sid := range c.Reality.ShortIDs {
		if !shortIDPattern.MatchString(sid) {
			return fmt.Errorf("reality.short_ids entry %q must be 16 hex chars", sid)
		}
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level %q must be debug|info|warn|error", c.LogLevel)
	}
	return nil
}

// PrivateKeyBytes 解码 X25519 私钥。先试 base64 RawURLEncoding(xray 惯例),
// 失败再试 StdEncoding;必须是 32 字节。
func (r RealityConfig) PrivateKeyBytes() ([]byte, error) {
	b, err := base64.RawURLEncoding.DecodeString(r.PrivateKey)
	if err != nil {
		b, err = base64.StdEncoding.DecodeString(r.PrivateKey)
	}
	if err != nil {
		return nil, fmt.Errorf("reality.private_key is not valid base64: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("reality.private_key must decode to 32 bytes, got %d", len(b))
	}
	return b, nil
}

// ShortIDBytesList 返回 Reality 接受的 short_id 字节列表;
// reality.short_ids 为空时回退到 vless.short_id。
func (c *ServerConfig) ShortIDBytesList() ([][]byte, error) {
	ids := c.Reality.ShortIDs
	if len(ids) == 0 {
		ids = []string{c.VLESS.ShortID}
	}
	out := make([][]byte, 0, len(ids))
	for _, s := range ids {
		if !shortIDPattern.MatchString(s) {
			return nil, fmt.Errorf("short_id %q must be 16 hex chars", s)
		}
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}
```

- [ ] **Step 5: 实现 env_override.go**

```go
package config

import (
	"fmt"
	"os"
	"strconv"
)

// applyEnv 将环境变量覆盖到配置上(优先于 YAML)。
func applyEnv(c *ServerConfig) error {
	setters := []struct {
		key string
		set func(string) error
	}{
		{"LISTEN_PORT", func(v string) error { return setInt(&c.ListenPort, v) }},
		{"VLESS_UUID", func(v string) error { c.VLESS.UUID = v; return nil }},
		{"VLESS_SHORT_ID", func(v string) error { c.VLESS.ShortID = v; return nil }},
		{"REALITY_PRIVATE_KEY", func(v string) error { c.Reality.PrivateKey = v; return nil }},
		{"REALITY_DEST", func(v string) error { c.Reality.Dest = v; return nil }},
		{"HTTP_API_PORT", func(v string) error { return setInt(&c.HTTPAPI.ListenPort, v) }},
		{"LOG_LEVEL", func(v string) error { c.LogLevel = v; return nil }},
	}
	for _, s := range setters {
		if v, ok := os.LookupEnv(s.key); ok && v != "" {
			if err := s.set(v); err != nil {
				return fmt.Errorf("env %s: %w", s.key, err)
			}
		}
	}
	return nil
}

func setInt(dst *int, v string) error {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("invalid integer %q", v)
	}
	*dst = n
	return nil
}
```

- [ ] **Step 6: 运行测试确认通过**

```bash
go test ./internal/config/ -v
```
Expected: PASS,全部用例。

- [ ] **Step 7: 写 config.example.yaml**

```yaml
# go-reality-server 示例配置
# 复制为 config.yaml 并填入密钥:go run ./cmd/server keys generate
listen_address: "::"
listen_port: 443

vless:
  uuid: ""                    # 必填,UUID 格式
  flow: ""                    # REALITY 不使用 flow,保持为空
  short_id: ""                # 必填,16 个十六进制字符

reality:
  private_key: ""             # 必填,X25519 私钥(base64url 或 std)
  dest: "www.microsoft.com:443"   # 握手伪装目标
  server_names: ["www.microsoft.com"]
  short_ids: []               # 留空则复用 vless.short_id
  x25519_pub: ""              # 非必填,仅用于生成客户端分享链接

http_api:
  listen_address: "127.0.0.1" # 只监听本机
  listen_port: 9090           # 0 = 关闭

log_level: "info"             # debug | info | warn | error
```

- [ ] **Step 8: 写 .gitignore 与 LICENSE**

`.gitignore`:
```
/bin/
*.exe
config.yaml
```

`LICENSE`(MIT,版权行写 `Copyright (c) 2026 go-reality-server contributors`):
```
MIT License

Copyright (c) 2026 go-reality-server contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 9: 全量验证并提交**

```bash
go vet ./...
go test ./...
git add go.mod go.sum .gitignore LICENSE config.example.yaml internal/config/
git commit -m "feat: config 包——YAML 装载、env 覆盖、校验与项目骨架

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: proxy/keys.go 密钥生成

**Files:**
- Create: `internal/proxy/keys.go`
- Test: `internal/proxy/keys_test.go`

**Interfaces:**
- Consumes: 无(引入 xray-core 依赖)
- Produces:
  - `func GenerateUUID() string`
  - `func GenerateShortID() string`
  - `func GenerateKeyPair() (*KeyPair, error)`,`type KeyPair struct{ PrivateKey, PublicKey []byte }`
  - `func (k *KeyPair) PrivateKeyB64() string` / `func (k *KeyPair) PublicKeyB64() string`(RawURLEncoding)
  - `func GenerateShareLink(serverAddr string, port int, uuidStr, shortID, pubKeyB64, sni, name string) string`

- [ ] **Step 1: 引入 xray-core**

```bash
go get github.com/xtls/xray-core@v26.3.27
```
Expected: 下载依赖树(首次较久,数分钟)。xray-core v26.3.27 要求 go 1.27,本机 1.27.1 满足。

- [ ] **Step 2: 写失败测试**

```go
// internal/proxy/keys_test.go
package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/xtls/xray-core/common/uuid"
)

var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func TestGenerateUUID(t *testing.T) {
	id := GenerateUUID()
	if !uuidRe.MatchString(id) {
		t.Fatalf("GenerateUUID() = %q, not a UUID", id)
	}
	if _, err := uuid.ParseString(id); err != nil {
		t.Fatalf("xray uuid.ParseString rejects %q: %v", id, err)
	}
	if GenerateUUID() == id {
		t.Fatal("two calls returned the same UUID")
	}
}

func TestGenerateShortID(t *testing.T) {
	sid := GenerateShortID()
	if len(sid) != 16 {
		t.Fatalf("len = %d, want 16", len(sid))
	}
	if _, err := hex.DecodeString(sid); err != nil {
		t.Fatalf("not hex: %v", err)
	}
}

func TestGenerateKeyPair(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	if len(kp.PrivateKey) != 32 || len(kp.PublicKey) != 32 {
		t.Fatalf("key lens = %d/%d, want 32/32", len(kp.PrivateKey), len(kp.PublicKey))
	}
	// 用私钥重新派生公钥,必须一致
	derived, err := x25519PubFromPriv(kp.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !equalBytes(derived, kp.PublicKey) {
		t.Fatal("public key does not match private key")
	}
	// base64 输出可解码回 32 字节
	if b, err := base64.RawURLEncoding.DecodeString(kp.PrivateKeyB64()); err != nil || len(b) != 32 {
		t.Fatalf("PrivateKeyB64 not decodable: %v, len %d", err, len(b))
	}
	if b, err := base64.RawURLEncoding.DecodeString(kp.PublicKeyB64()); err != nil || len(b) != 32 {
		t.Fatalf("PublicKeyB64 not decodable: %v, len %d", err, len(b))
	}
}

func TestGenerateShareLink(t *testing.T) {
	link := GenerateShareLink("1.2.3.4", 443, "93a6f3b4-71a2-4a11-b7bf-e73a6a9b43bf", "0123456789abcdef", "cHVia2V5cHVia2V5cHVia2V5cHVia2V5cHVia2V5", "www.microsoft.com", "my-server")
	if !strings.HasPrefix(link, "vless://") {
		t.Fatalf("link = %q, want vless:// prefix", link)
	}
	u, err := url.Parse(link)
	if err != nil {
		t.Fatal(err)
	}
	if u.User.Username() != "93a6f3b4-71a2-4a11-b7bf-e73a6a9b43bf" {
		t.Errorf("username = %q", u.User.Username())
	}
	if u.Host != "1.2.3.4:443" {
		t.Errorf("host = %q", u.Host)
	}
	q := u.Query()
	for k, want := range map[string]string{
		"type": "tcp", "security": "reality", "fp": "chrome",
		"sni": "www.microsoft.com", "sid": "0123456789abcdef",
		"pbk": "cHVia2V5cHVia2V5cHVia2V5cHVia2V5cHVia2V5", "spx": "/",
	} {
		if q.Get(k) != want {
			t.Errorf("query %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if u.Fragment != "my-server" {
		t.Errorf("fragment = %q", u.Fragment)
	}
}
```

测试里用到两个辅助(与实现共用的 `x25519PubFromPriv` 见 Step 4,`equalBytes` 写死在测试里):

```go
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./internal/proxy/ -run 'TestGenerate|TestKeyPair' -v
```
Expected: FAIL — `undefined: GenerateUUID`。

- [ ] **Step 4: 实现 keys.go**

```go
package proxy

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"

	"github.com/xtls/xray-core/common/uuid"
)

// GenerateUUID 返回随机 UUID v4 字符串(复用 xray 的实现)。
func GenerateUUID() string {
	return uuid.New().String()
}

// GenerateShortID 返回 16 个十六进制字符(8 随机字节),REALITY 惯例。
func GenerateShortID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand 失败意味着系统熵源坏了,无法继续
	}
	return hex.EncodeToString(b)
}

// KeyPair 是 REALITY 握手用的 X25519 密钥对,字段均为 32 字节原始字节。
type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

// GenerateKeyPair 生成带 X25519 标准 clamping 的密钥对
// (与 xray 自带 `xray x25519` 命令同款算法)。
func GenerateKeyPair() (*KeyPair, error) {
	priv := make([]byte, 32)
	if _, err := rand.Read(priv); err != nil {
		return nil, err
	}
	priv[0] &= 248
	priv[31] &= 127
	priv[31] |= 64
	key, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return &KeyPair{PrivateKey: key.Bytes(), PublicKey: key.PublicKey().Bytes()}, nil
}

// PrivateKeyB64 返回 base64 RawURLEncoding 的私钥字符串。
func (k *KeyPair) PrivateKeyB64() string {
	return base64.RawURLEncoding.EncodeToString(k.PrivateKey)
}

// PublicKeyB64 返回 base64 RawURLEncoding 的公钥字符串。
func (k *KeyPair) PublicKeyB64() string {
	return base64.RawURLEncoding.EncodeToString(k.PublicKey)
}

// x25519PubFromPriv 从私钥字节派生公钥,测试用。
func x25519PubFromPriv(priv []byte) ([]byte, error) {
	key, err := ecdh.X25519().NewPrivateKey(priv)
	if err != nil {
		return nil, err
	}
	return key.PublicKey().Bytes(), nil
}

// GenerateShareLink 生成 vless:// 分享链接,可直接导入
// Clash Meta / v2rayN / NekoBox 等客户端。
func GenerateShareLink(serverAddr string, port int, uuidStr, shortID, pubKeyB64, sni, name string) string {
	q := url.Values{}
	q.Set("type", "tcp")
	q.Set("security", "reality")
	q.Set("pbk", pubKeyB64)
	q.Set("fp", "chrome")
	q.Set("sni", sni)
	q.Set("sid", shortID)
	q.Set("spx", "/")
	if name == "" {
		name = "go-reality-server"
	}
	return fmt.Sprintf("vless://%s@%s:%d?%s#%s", uuidStr, serverAddr, port, q.Encode(), url.PathEscape(name))
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/proxy/ -run 'TestGenerate|TestKeyPair' -v
```
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/proxy/
git commit -m "feat: keys 子命令——UUID/short_id/X25519 密钥对与分享链接生成

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: inbound/outbound proto 拼装

**Files:**
- Create: `internal/proxy/inbound.go`
- Create: `internal/proxy/outbound.go`
- Create: `internal/proxy/app.go`
- Test: `internal/proxy/inbound_test.go`

**Interfaces:**
- Consumes: Task 1 的 `config.ServerConfig` 与字节解码方法;Task 2 的 `GenerateKeyPair/GenerateUUID/GenerateShortID`(测试用)
- Produces:
  - `func buildVLESSInbound(cfg *config.ServerConfig) (*core.InboundHandlerConfig, error)`
  - `func buildFreedomOutbound() *core.OutboundHandlerConfig`
  - `func buildAppMessages() []*serial.TypedMessage`

- [ ] **Step 1: 写失败测试**

```go
// internal/proxy/inbound_test.go
package proxy

import (
	"testing"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/freedom"
	"github.com/xtls/xray-core/proxy/vless"
	vlessinbound "github.com/xtls/xray-core/proxy/vless/inbound"
	"github.com/xtls/xray-core/transport/internet/reality"
	"google.golang.org/protobuf/proto"

	"go-reality-server/internal/config"
)

func testServerConfig(t *testing.T) *config.ServerConfig {
	t.Helper()
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	cfg.ListenAddress = "127.0.0.1"
	cfg.ListenPort = 14443
	cfg.VLESS.UUID = GenerateUUID()
	cfg.VLESS.ShortID = GenerateShortID()
	cfg.Reality.PrivateKey = kp.PrivateKeyB64()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	return &cfg
}

func unmarshalTyped(t *testing.T, tm *serial.TypedMessage, out proto.Message) {
	t.Helper()
	if tm == nil {
		t.Fatal("typed message is nil")
	}
	if err := tm.UnmarshalTo(out); err != nil {
		t.Fatalf("unmarshal %s: %v", tm.Type, err)
	}
}

func TestBuildVLESSInbound(t *testing.T) {
	cfg := testServerConfig(t)
	ic, err := buildVLESSInbound(cfg)
	if err != nil {
		t.Fatalf("buildVLESSInbound: %v", err)
	}
	if ic.Tag != "vless-in" {
		t.Errorf("tag = %q", ic.Tag)
	}

	// ProxySettings 必须是 vless inbound Config,含一个用户、UUID 正确
	var proxy vlessinbound.Config
	unmarshalTyped(t, ic.ProxySettings, &proxy)
	if len(proxy.Users) != 1 {
		t.Fatalf("users = %d, want 1", len(proxy.Users))
	}
	var account vless.Account
	unmarshalTyped(t, proxy.Users[0].Account, &account)
	if account.Id != cfg.VLESS.UUID {
		t.Errorf("account id = %q, want %q", account.Id, cfg.VLESS.UUID)
	}
	if account.Flow != "" {
		t.Errorf("flow = %q, want empty (REALITY)", account.Flow)
	}

	// ReceiverSettings 必须监听目标端口
	var receiver proxyman.ReceiverConfig
	unmarshalTyped(t, ic.ReceiverSettings, &receiver)
	if receiver.PortList == nil || len(receiver.PortList.Range) != 1 ||
		receiver.PortList.Range[0].From != 14443 || receiver.PortList.Range[0].To != 14443 {
		t.Errorf("port list = %+v, want 14443", receiver.PortList)
	}

	// Stream security 必须是 reality 类型,字段与配置一致
	ss := receiver.StreamSettings
	if ss == nil {
		t.Fatal("stream settings nil")
	}
	if len(ss.SecuritySettings) != 1 {
		t.Fatalf("security settings = %d, want 1", len(ss.SecuritySettings))
	}
	var rc reality.Config
	unmarshalTyped(t, ss.SecuritySettings[0], &rc)
	if rc.Dest != "www.microsoft.com:443" {
		t.Errorf("reality dest = %q", rc.Dest)
	}
	if len(rc.ServerNames) != 1 || rc.ServerNames[0] != "www.microsoft.com" {
		t.Errorf("server names = %v", rc.ServerNames)
	}
	privKey, err := cfg.Reality.PrivateKeyBytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(rc.PrivateKey) != string(privKey) {
		t.Error("reality private key mismatch")
	}
	if len(rc.ShortIds) != 1 || len(rc.ShortIds[0]) != 8 {
		t.Errorf("short ids = %v, want one 8-byte id", rc.ShortIds)
	}
}

func TestBuildFreedomOutbound(t *testing.T) {
	oc := buildFreedomOutbound()
	if oc.Tag != "direct-out" {
		t.Errorf("tag = %q", oc.Tag)
	}
	var fr freedom.Config
	unmarshalTyped(t, oc.ProxySettings, &fr)
	var sender proxyman.SenderConfig
	unmarshalTyped(t, oc.SenderSettings, &sender)
}

func TestBuildAppMessages(t *testing.T) {
	msgs := buildAppMessages()
	if len(msgs) != 3 {
		t.Fatalf("app messages = %d, want 3 (dispatcher, proxyman-in, proxyman-out)", len(msgs))
	}
	types := map[string]bool{}
	for _, m := range msgs {
		types[m.Type] = true
	}
	for _, want := range []string{
		"xray.app.dispatcher.Config",
		"xray.app.proxyman.InboundConfig",
		"xray.app.proxyman.OutboundConfig",
	} {
		if !types[want] {
			t.Errorf("missing app type %q, got %v", want, types)
		}
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/proxy/ -run 'TestBuild' -v
```
Expected: FAIL — `undefined: buildVLESSInbound`。

- [ ] **Step 3: 实现 inbound.go**

```go
package proxy

import (
	"fmt"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/vless"
	vlessinbound "github.com/xtls/xray-core/proxy/vless/inbound"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"

	"go-reality-server/internal/config"
)

// buildVLESSInbound 拼装 VLESS+REALITY inbound 的完整 proto 配置。
// 所有 xray proto 细节只允许出现在本包。
func buildVLESSInbound(cfg *config.ServerConfig) (*core.InboundHandlerConfig, error) {
	privKey, err := cfg.Reality.PrivateKeyBytes()
	if err != nil {
		return nil, fmt.Errorf("decode reality private key: %w", err)
	}
	shortIDs, err := cfg.ShortIDBytesList()
	if err != nil {
		return nil, fmt.Errorf("decode reality short ids: %w", err)
	}

	// VLESS 用户:Account 消息挂在 protocol.User 上
	user := &protocol.User{
		Account: serial.ToTypedMessage(&vless.Account{
			Id:   cfg.VLESS.UUID,
			Flow: cfg.VLESS.Flow, // REALITY 下为空
		}),
	}
	proxySettings := serial.ToTypedMessage(&vlessinbound.Config{
		Users:      []*protocol.User{user},
		Decryption: "none",
	})

	// REALITY 安全层:挂 StreamConfig.SecuritySettings,类型名取自 TypedMessage.Type
	realityMsg := serial.ToTypedMessage(&reality.Config{
		Dest:        cfg.Reality.Dest,
		ServerNames: cfg.Reality.ServerNames,
		PrivateKey:  privKey,
		ShortIds:    shortIDs,
	})
	streamSettings := &internet.StreamConfig{
		ProtocolName:     "tcp",
		SecurityType:     realityMsg.Type,
		SecuritySettings: []*serial.TypedMessage{realityMsg},
	}

	receiverSettings := &proxyman.ReceiverConfig{
		PortList: &net.PortList{
			Range: []*net.PortRange{
				{From: uint32(cfg.ListenPort), To: uint32(cfg.ListenPort)},
			},
		},
		Listen:         net.NewIPOrDomain(net.ParseAddress(cfg.ListenAddress)),
		StreamSettings: streamSettings,
	}

	return &core.InboundHandlerConfig{
		Tag:              "vless-in",
		ReceiverSettings: serial.ToTypedMessage(receiverSettings),
		ProxySettings:    proxySettings,
	}, nil
}
```

- [ ] **Step 4: 实现 outbound.go 与 app.go**

```go
// internal/proxy/outbound.go
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
```

```go
// internal/proxy/app.go
package proxy

import (
	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/serial"
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
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/proxy/ -run 'TestBuild' -v
```
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add internal/proxy/
git commit -m "feat: xray proto 拼装——VLESS+REALITY inbound、freedom outbound、app 要素

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: proxy/engine.go 生命周期

**Files:**
- Create: `internal/proxy/engine.go`
- Test: `internal/proxy/engine_test.go`

**Interfaces:**
- Consumes: Task 1 `config.ServerConfig`;Task 3 的 `buildVLESSInbound/buildFreedomOutbound/buildAppMessages`
- Produces:
  - `func NewEngine(cfg *config.ServerConfig) (*Engine, error)`
  - `func (e *Engine) Start() error` / `func (e *Engine) Stop() error` / `func (e *Engine) IsReady() bool`
  - `type Engine struct{...}`(字段私有;httpapi 通过 `IsReady()` 接口消费)

- [ ] **Step 1: 写失败测试**

```go
// internal/proxy/engine_test.go
package proxy

import (
	"net"
	"strconv"
	"testing"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestEngineLifecycle(t *testing.T) {
	cfg := testServerConfig(t)
	cfg.ListenPort = freePort(t)

	engine, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if engine.IsReady() {
		t.Fatal("IsReady before Start should be false")
	}

	if err := engine.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !engine.IsReady() {
		t.Fatal("IsReady after Start should be true")
	}
	// 端口确实在监听
	if err := pingPort(cfg.ListenPort); err != nil {
		t.Fatalf("port %d not listening: %v", cfg.ListenPort, err)
	}

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if engine.IsReady() {
		t.Fatal("IsReady after Stop should be false")
	}
	// 二次 Stop 幂等
	if err := engine.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestEngineNewFailsOnBadConfig(t *testing.T) {
	cfg := testServerConfig(t)
	cfg.Reality.PrivateKey = "not-base64!"
	if _, err := NewEngine(cfg); err == nil {
		t.Fatal("NewEngine with bad private key should fail")
	}
}

func pingPort(port int) error {
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./internal/proxy/ -run 'TestEngine' -v
```
Expected: FAIL — `undefined: NewEngine`。
(注:xray 实例初始化需要数秒,测试整体约 10-30s,属正常。)

- [ ] **Step 3: 实现 engine.go**

```go
package proxy

import (
	"errors"
	"sync"

	core "github.com/xtls/xray-core/core"

	"go-reality-server/internal/config"
)

// Engine 包装嵌入的 xray-core 实例生命周期。
// xray 实例进程内同时只能有一个在运行,互斥锁保证 Start/Stop 串行。
type Engine struct {
	mu       sync.Mutex
	instance *core.Instance
	started  bool
	stopped  bool
}

// NewEngine 由 ServerConfig 拼装 proto 并创建(未启动的)xray 实例。
func NewEngine(cfg *config.ServerConfig) (*Engine, error) {
	inboundConfig, err := buildVLESSInbound(cfg)
	if err != nil {
		return nil, err
	}
	xrayConfig := &core.Config{
		App:      buildAppMessages(),
		Inbound:  []*core.InboundHandlerConfig{inboundConfig},
		Outbound: []*core.OutboundHandlerConfig{buildFreedomOutbound()},
	}
	instance, err := core.New(xrayConfig)
	if err != nil {
		return nil, err
	}
	return &Engine{instance: instance}, nil
}

// Start 启动引擎,幂等失败:重复启动返回错误。
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.started {
		return errors.New("engine already started")
	}
	if err := e.instance.Start(); err != nil {
		return err
	}
	e.started = true
	return nil
}

// Stop 优雅关闭引擎。幂等,可安全重复调用。
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stopped {
		return nil
	}
	e.stopped = true
	if !e.started {
		return nil
	}
	return e.instance.Close()
}

// IsReady 报告引擎是否正在运行(供 httpapi 探活)。
func (e *Engine) IsReady() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.started && !e.stopped
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
go test ./internal/proxy/ -v
```
Expected: PASS(keys/inbound/engine 全部)。

- [ ] **Step 5: 提交**

```bash
git add internal/proxy/
git commit -m "feat: Engine 生命周期包装——New/Start/Stop/IsReady 幂等实现

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: httpapi 健康检查与指标

**Files:**
- Create: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`

**Interfaces:**
- Consumes: 无(通过自定义小接口 `EngineState` 与 engine 解耦)
- Produces:
  - `type EngineState interface{ IsReady() bool }`
  - `func New(addr string, engine EngineState, registerer prometheus.Registerer) *Server`
  - `func (s *Server) ListenAndServe() error` / `func (s *Server) Shutdown(ctx context.Context) error`

- [ ] **Step 1: 引入 prometheus 客户端**

```bash
go get github.com/prometheus/client_golang@latest
```

- [ ] **Step 2: 写失败测试**

```go
// internal/httpapi/server_test.go
package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

type mockEngine struct{ ready bool }

func (m mockEngine) IsReady() bool { return m.ready }

func testServer(t *testing.T, engine EngineState) *httptest.Server {
	t.Helper()
	reg := prometheus.NewRegistry()
	srv := New("127.0.0.1:0", engine, reg)
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return ts
}

func get(t *testing.T, ts *httptest.Server, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(body)
}

func TestHealthz(t *testing.T) {
	ts := testServer(t, mockEngine{ready: false})
	code, body := get(t, ts, "/healthz")
	if code != http.StatusOK {
		t.Errorf("healthz code = %d, want 200", code)
	}
	if body != "ok" {
		t.Errorf("healthz body = %q, want ok", body)
	}
}

func TestReady(t *testing.T) {
	ts := testServer(t, mockEngine{ready: true})
	code, body := get(t, ts, "/ready")
	if code != http.StatusOK || body != "ready" {
		t.Errorf("ready = %d %q, want 200 ready", code, body)
	}
}

func TestNotReady(t *testing.T) {
	ts := testServer(t, mockEngine{ready: false})
	code, body := get(t, ts, "/ready")
	if code != http.StatusServiceUnavailable || body != "not ready" {
		t.Errorf("ready = %d %q, want 503 not ready", code, body)
	}
}

func TestMetrics(t *testing.T) {
	ts := testServer(t, mockEngine{ready: true})
	code, body := get(t, ts, "/metrics")
	if code != http.StatusOK {
		t.Fatalf("metrics code = %d, want 200", code)
	}
	for _, want := range []string{"go_reality_engine_ready", "go_goroutines"} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	// ready 引擎的 gauge 值为 1
	if !strings.Contains(body, "go_reality_engine_ready 1") {
		t.Error("engine gauge should be 1 when ready")
	}
}
```

- [ ] **Step 3: 运行测试确认失败**

```bash
go test ./internal/httpapi/ -v
```
Expected: FAIL — `undefined: New`。

- [ ] **Step 4: 实现 server.go**

```go
// Package httpapi 提供健康检查与 Prometheus 指标端点。
package httpapi

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EngineState 是 HTTP API 所需的引擎最小视图,避免包循环依赖。
type EngineState interface {
	IsReady() bool
}

// Server 暴露 /metrics、/healthz、/ready。
type Server struct {
	http *http.Server
}

// New 构建 HTTP API 服务。registerer 传 nil 时用 prometheus.DefaultRegisterer。
// 注意:同一 registerer 只能注册一次,main 中只创建一个 Server。
func New(addr string, engine EngineState, registerer prometheus.Registerer) *Server {
	if registerer == nil {
		registerer = prometheus.DefaultRegisterer
	}
	registerer.MustRegister(
		collectors.NewGoCollector(),
		newReadyCollector(engine),
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registerer, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		if engine.IsReady() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})

	return &Server{http: &http.Server{Addr: addr, Handler: mux}}
}

// ListenAndServe 阻塞服务,直到 Shutdown。
func (s *Server) ListenAndServe() error {
	return s.http.ListenAndServe()
}

// Shutdown 优雅关闭 HTTP 服务。
func (s *Server) Shutdown(ctx context.Context) error {
	return s.http.Shutdown(ctx)
}

// handler 暴露给测试用 httptest 包装。
func (s *Server) handler() http.Handler {
	return s.http.Handler
}

// readyCollector 在抓取时实时读取引擎状态,免去同步 gauge 的复杂度。
type readyCollector struct {
	engine EngineState
	desc   *prometheus.Desc
}

func newReadyCollector(engine EngineState) *readyCollector {
	return &readyCollector{
		engine: engine,
		desc: prometheus.NewDesc(
			"go_reality_engine_ready",
			"1 when the proxy engine is running, 0 otherwise.",
			nil, nil,
		),
	}
}

func (c *readyCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *readyCollector) Collect(ch chan<- prometheus.Metric) {
	v := 0.0
	if c.engine.IsReady() {
		v = 1.0
	}
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, v)
}
```

- [ ] **Step 5: 运行测试确认通过**

```bash
go test ./internal/httpapi/ -v
```
Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum internal/httpapi/
git commit -m "feat: httpapi——/metrics /healthz /ready 与实时引擎状态 collector

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: logging + cmd/server 入口

**Files:**
- Create: `internal/logging/logging.go`
- Create: `cmd/server/main.go`
- Test: `cmd/server/main_test.go`

**Interfaces:**
- Consumes: Task 1 `config.Load`;Task 2 `GenerateUUID/GenerateShortID/GenerateKeyPair/GenerateShareLink`;Task 4 `NewEngine/Start/Stop`;Task 5 `httpapi.New`
- Produces:
  - `func logging.New(level string) *slog.Logger`
  - 可执行入口 `cmd/server`,子命令 `keys generate`

- [ ] **Step 1: 写失败测试(main 的 keys 子命令可测函数)**

```go
// cmd/server/main_test.go
package main

import (
	"bytes"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"
)

func TestRunKeysGenerate(t *testing.T) {
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, []string{}); err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	out := buf.String()
	t.Logf("output:\n%s", out)

	lines := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			lines[parts[0]] = strings.TrimSpace(parts[1])
		}
	}
	for _, k := range []string{"UUID", "ShortID", "PrivateKey", "PublicKey"} {
		if lines[k] == "" {
			t.Errorf("missing key %q in output", k)
		}
	}
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`).MatchString(lines["UUID"]) {
		t.Errorf("UUID format bad: %q", lines["UUID"])
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(lines["ShortID"]) {
		t.Errorf("ShortID format bad: %q", lines["ShortID"])
	}
	for _, k := range []string{"PrivateKey", "PublicKey"} {
		b, err := base64.RawURLEncoding.DecodeString(lines[k])
		if err != nil || len(b) != 32 {
			t.Errorf("%s not 32-byte base64url: %v", k, err)
		}
	}
	// 不带 -server 时不输出分享链接
	if strings.Contains(out, "ShareLink") {
		t.Error("ShareLink should be absent without -server flag")
	}
}

func TestRunKeysGenerateWithShareLink(t *testing.T) {
	var buf bytes.Buffer
	err := runKeysGenerate(&buf, []string{"-server", "1.2.3.4", "-port", "443", "-name", "test-node"})
	if err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "ShareLink") || !strings.Contains(out, "vless://") {
		t.Errorf("output missing share link:\n%s", out)
	}
	if !strings.Contains(out, "1.2.3.4:443") {
		t.Errorf("share link missing server addr:\n%s", out)
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
go test ./cmd/server/ -v
```
Expected: FAIL — `undefined: runKeysGenerate`。

- [ ] **Step 3: 实现 logging.go**

```go
// Package logging 提供 journald 友好的 JSON 日志。
package logging

import (
	"log/slog"
	"os"
	"strings"
)

// New 返回写到 stdout 的 JSON slog logger,级别来自配置字符串。
func New(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}
```

- [ ] **Step 4: 实现 main.go**

```go
// go-reality-server:嵌入 xray-core 的 VLESS+REALITY 代理服务端。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"go-reality-server/internal/config"
	"go-reality-server/internal/httpapi"
	"go-reality-server/internal/logging"
	"go-reality-server/internal/proxy"
)

func main() {
	// 子命令判断必须在 flag.Parse 之前
	if len(os.Args) > 1 && os.Args[1] == "keys" {
		if err := runKeysGenerate(os.Stdout, os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(2)
		}
		return
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("go-reality-server", flag.ContinueOnError)
	configPath := fs.String("config", "config.yaml", "path to config file")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)
	slog.SetDefault(logger)

	engine, err := proxy.NewEngine(cfg)
	if err != nil {
		return fmt.Errorf("create engine: %w", err)
	}
	if err := engine.Start(); err != nil {
		return fmt.Errorf("start engine: %w", err)
	}
	logger.Info("engine started",
		"listen", net.JoinHostPort(cfg.ListenAddress, strconv.Itoa(cfg.ListenPort)),
		"uuid", cfg.VLESS.UUID[:8]+"...",
	)

	var api *httpapi.Server
	if cfg.HTTPAPI.ListenPort > 0 {
		addr := net.JoinHostPort(cfg.HTTPAPI.ListenAddress, strconv.Itoa(cfg.HTTPAPI.ListenPort))
		api = httpapi.New(addr, engine, nil)
		go func() {
			if err := api.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("http api failed", "error", err)
			}
		}()
		logger.Info("http api listening", "addr", addr)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("signal received, shutting down", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if api != nil {
		_ = api.Shutdown(ctx)
	}
	if err := engine.Stop(); err != nil {
		logger.Error("engine stop failed", "error", err)
		return err
	}
	logger.Info("shutdown complete")
	return nil
}

// runKeysGenerate 生成 UUID/short_id/X25519 密钥对,带 -server 时附分享链接。
func runKeysGenerate(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("keys generate", flag.ContinueOnError)
	server := fs.String("server", "", "server address for share link, e.g. 1.2.3.4")
	port := fs.Int("port", 443, "server port for share link")
	sni := fs.String("sni", "www.microsoft.com", "SNI/server_name for share link")
	name := fs.String("name", "go-reality-server", "name for share link")
	if err := fs.Parse(args); err != nil {
		return err
	}

	id := proxy.GenerateUUID()
	sid := proxy.GenerateShortID()
	kp, err := proxy.GenerateKeyPair()
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "UUID:        %s\n", id)
	fmt.Fprintf(w, "ShortID:     %s\n", sid)
	fmt.Fprintf(w, "PrivateKey:  %s\n", kp.PrivateKeyB64())
	fmt.Fprintf(w, "PublicKey:   %s\n", kp.PublicKeyB64())
	if *server != "" {
		link := proxy.GenerateShareLink(*server, *port, id, sid, kp.PublicKeyB64(), *sni, *name)
		fmt.Fprintf(w, "ShareLink:   %s\n", link)
	}
	return nil
}
```

- [ ] **Step 5: 运行测试确认通过并编译**

```bash
go test ./cmd/server/ -v
go build ./...
```
Expected: PASS + build 成功。

- [ ] **Step 6: 手动冒烟——keys 子命令**

```bash
go run ./cmd/server keys generate -server 127.0.0.1
```
Expected: 输出 UUID/ShortID/PrivateKey/PublicKey/ShareLink 五行,格式如上测试断言。

- [ ] **Step 7: 提交**

```bash
git add cmd/ internal/logging/
git commit -m "feat: 主入口——装配、信号处理、优雅关闭与 keys generate 子命令

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: 部署资产(Makefile、脚本、systemd、README)

**Files:**
- Create: `Makefile`
- Create: `scripts/cross-build.sh`
- Create: `scripts/gen-keys.sh`
- Create: `deploy/systemd/go-reality.service`
- Create: `deploy/install.sh`
- Create: `README.md`

**Interfaces:**
- Consumes: Task 6 的二进制与子命令
- Produces: 无代码接口,交付部署文件

- [ ] **Step 1: 写 Makefile**

```makefile
BINARY := bin/go-reality-server

.PHONY: build test test-integration cross fmt vet gen-keys clean

build:
	go build -trimpath -o $(BINARY) ./cmd/server

test:
	go test ./...

test-integration:
	go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v

cross:
	GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
	GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server

fmt:
	gofmt -w .

vet:
	go vet ./...

gen-keys:
	go run ./cmd/server keys generate

clean:
	rm -rf bin
```

- [ ] **Step 2: 写 scripts/cross-build.sh 与 scripts/gen-keys.sh**

```bash
#!/usr/bin/env bash
# 交叉编译 linux/amd64 与 linux/arm64 产物到 bin/。
set -euo pipefail
cd "$(dirname "$0")/.."

go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
echo "built bin/go-reality-server-linux-amd64"

go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server
echo "built bin/go-reality-server-linux-arm64"
```

```bash
#!/usr/bin/env bash
# 生成 UUID / short_id / X25519 密钥对,可透传 -server 等参数生成分享链接。
set -euo pipefail
cd "$(dirname "$0")/.."
exec go run ./cmd/server keys generate "$@"
```

- [ ] **Step 3: 写 systemd unit**

```ini
[Unit]
Description=go-reality-server VLESS REALITY proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/go-reality-server -config /etc/go-reality/config.yaml
EnvironmentFile=/etc/go-reality/env
Restart=on-failure
RestartSec=5s
DynamicUser=yes
ConfigurationDirectory=go-reality
AmbientCapabilities=CAP_NET_BIND_SERVICE
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
```

- [ ] **Step 4: 写 deploy/install.sh**

```bash
#!/usr/bin/env bash
# 服务器端一键安装:把交叉编译产物装为 systemd 服务。
# 用法: sudo bash install.sh /path/to/go-reality-server-linux-amd64
set -euo pipefail

BIN="${1:-go-reality-server-linux-amd64}"
CONF_DIR="/etc/go-reality"

echo "==> 安装二进制到 /usr/local/bin"
install -m 0755 "$BIN" /usr/local/bin/go-reality-server

echo "==> 创建配置目录 $CONF_DIR"
mkdir -p "$CONF_DIR"
if [[ ! -f "$CONF_DIR/config.yaml" ]]; then
  install -m 0600 config.example.yaml "$CONF_DIR/config.yaml"
  echo "    已写入示例配置,请编辑 $CONF_DIR/config.yaml 填入密钥"
fi
if [[ ! -f "$CONF_DIR/env" ]]; then
  touch "$CONF_DIR/env"
fi
chmod 0600 "$CONF_DIR/env"

echo "==> 安装 systemd unit"
install -m 0644 deploy/systemd/go-reality.service /etc/systemd/system/go-reality.service
systemctl daemon-reload
systemctl enable --now go-reality

echo "==> 完成。填入密钥后执行: sudo systemctl restart go-reality"
echo "    查看日志: journalctl -u go-reality -f -n 50"
```

- [ ] **Step 5: 写 README.md**

```markdown
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

## 运维

- 健康检查:`curl http://127.0.0.1:9090/healthz`(存活)、`/ready`(就绪)
- 指标:`curl http://127.0.0.1:9090/metrics`(Prometheus 格式)
- 日志:`journalctl -u go-reality -f -n 50`
- 集成测试(真实出网,需可访问 www.microsoft.com 与 ifconfig.me):

```bash
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegration -v
```
```

- [ ] **Step 6: 验证构建与交叉编译**

```bash
go vet ./...
go build -trimpath -o bin/go-reality-server ./cmd/server
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-amd64 ./cmd/server
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o bin/go-reality-server-linux-arm64 ./cmd/server
ls -la bin/
```
Expected: 三个二进制生成成功(本机 + linux/amd64 + linux/arm64)。xray-core 纯 Go,无 CGO 依赖,交叉编译可直接过。

- [ ] **Step 7: 提交**

```bash
git add Makefile scripts/ deploy/ README.md
git commit -m "docs: 部署资产——Makefile、交叉编译、systemd unit、安装脚本、README

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: 集成测试(真实出网)

**Files:**
- Test: `internal/proxy/integration_test.go`(build tag `integration`)

**Interfaces:**
- Consumes: Task 2 的密钥生成;Task 4 的 `NewEngine/Start/Stop`;Task 3 的 `buildAppMessages`
- Produces: 无

- [ ] **Step 1: 写集成测试**

```go
//go:build integration

package proxy

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	core "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/proxy/dokodemo"
	"github.com/xtls/xray-core/proxy/vless"
	vlessoutbound "github.com/xtls/xray-core/proxy/vless/outbound"
	"github.com/xtls/xray-core/transport/internet"
	"github.com/xtls/xray-core/transport/internet/reality"

	"go-reality-server/internal/config"
)

// TestIntegrationEgress 完整链路:本地 dokodemo 入口 -> 进程内 vless+reality 客户端
// -> 本服务端 -> freedom 直连 ifconfig.me,断言拿到出口 IP。
// 需要可访问 www.microsoft.com(REALITY 握手)与 ifconfig.me。
func TestIntegrationEgress(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	uuidStr, shortID, sni := GenerateUUID(), GenerateShortID(), "www.microsoft.com"

	// 1. 服务端
	cfg := config.Defaults()
	cfg.ListenAddress = "127.0.0.1"
	cfg.ListenPort = freePort(t)
	cfg.VLESS.UUID = uuidStr
	cfg.VLESS.ShortID = shortID
	cfg.Reality.PrivateKey = kp.PrivateKeyB64()
	cfg.Reality.ServerNames = []string{sni}
	engine, err := NewEngine(&cfg)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if err := engine.Start(); err != nil {
		t.Fatalf("engine Start: %v", err)
	}
	t.Cleanup(func() { _ = engine.Stop() })

	// 2. 进程内客户端:dokodemo 入口 -> vless outbound(reality)
	dokodemoPort := freePort(t)
	clientInst, err := startClientInstance(t, cfg.ListenPort, uuidStr, shortID, kp, sni, dokodemoPort)
	if err != nil {
		t.Fatalf("client instance: %v", err)
	}
	t.Cleanup(func() { _ = clientInst.Close() })

	// 3. 经隧道请求 https://ifconfig.me,期望返回公网 IP
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp",
				net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", dokodemoPort)))
		},
		TLSClientConfig: &tls.Config{ServerName: "ifconfig.me"},
	}
	client := &http.Client{Transport: transport, Timeout: 45 * time.Second}
	resp, err := client.Get("https://ifconfig.me")
	if err != nil {
		t.Fatalf("GET through tunnel: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	ip := string(body)
	if !regexp.MustCompile(`^\d{1,3}(\.\d{1,3}){3}\s*$`).MatchString(ip) {
		t.Fatalf("ifconfig.me returned %q, want an IP", ip)
	}
	t.Logf("egress OK, public IP = %s", ip)
}

func startClientInstance(t *testing.T, serverPort int, uuidStr, shortID string, kp *KeyPair, sni string, dokodemoPort int) (*core.Instance, error) {
	t.Helper()

	dokodemoInbound := &core.InboundHandlerConfig{
		Tag: "dokodemo-in",
		ReceiverSettings: serial.ToTypedMessage(&proxyman.ReceiverConfig{
			PortList: &net.PortList{
				Range: []*net.PortRange{
					{From: uint32(dokodemoPort), To: uint32(dokodemoPort)},
				},
			},
			Listen: net.NewIPOrDomain(net.LocalHostIP),
		}),
		ProxySettings: serial.ToTypedMessage(&dokodemo.Config{
			Address:     net.NewIPOrDomain(net.ParseAddress("ifconfig.me")),
			Port:        443,
			NetworkList: []*net.Network{{Network: net.Network_TCP}},
		}),
	}

	realityMsg := serial.ToTypedMessage(&reality.Config{
		ServerName:  sni,
		Fingerprint: "chrome",
		PublicKey:   kp.PublicKey,
		ShortId:     mustHexDecode(t, shortID),
		SpiderX:     "/",
	})
	vlessOutbound := &core.OutboundHandlerConfig{
		Tag: "vless-out",
		SenderSettings: serial.ToTypedMessage(&proxyman.SenderConfig{
			StreamSettings: &internet.StreamConfig{
				ProtocolName:     "tcp",
				SecurityType:     realityMsg.Type,
				SecuritySettings: []*serial.TypedMessage{realityMsg},
			},
		}),
		ProxySettings: serial.ToTypedMessage(&vlessoutbound.Config{
			Vnext: &protocol.ServerEndpoint{
				Address: net.NewIPOrDomain(net.LocalHostIP),
				Port:    uint32(serverPort),
				User: &protocol.User{
					Account: serial.ToTypedMessage(&vless.Account{Id: uuidStr}),
				},
			},
		}),
	}

	instance, err := core.New(&core.Config{
		App:      buildAppMessages(),
		Inbound:  []*core.InboundHandlerConfig{dokodemoInbound},
		Outbound: []*core.OutboundHandlerConfig{vlessOutbound},
	})
	if err != nil {
		return nil, err
	}
	if err := instance.Start(); err != nil {
		return nil, err
	}
	return instance, nil
}

func mustHexDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex decode %q: %v", s, err)
	}
	return b
}
```

- [ ] **Step 2: 运行集成测试(需要外网)**

```bash
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegrationEgress -v
```
Expected: PASS,日志打印 `egress OK, public IP = x.x.x.x`。若网络访问 microsoft.com/ifconfig.me 不通,测试会明确失败,属环境问题而非代码问题。

- [ ] **Step 3: 确认默认测试不包含集成用例**

```bash
go test ./... -count=1
```
Expected: PASS,且不运行 TestIntegrationEgress(build tag 隔离)。

- [ ] **Step 4: 提交**

```bash
git add internal/proxy/integration_test.go
git commit -m "test: 集成测试——进程内客户端经 REALITY 隧道真实出网

Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>"
```

---

## 最终验收(全部任务完成后)

```bash
go vet ./... && go test ./... -count=1          # 单测全绿
go build -trimpath -o bin/go-reality-server ./cmd/server   # 本机二进制
./scripts/cross-build.sh                        # linux/amd64 + linux/arm64
go run ./cmd/server keys generate -server <服务器地址>      # 密钥 + 分享链接
go test -tags=integration -count=1 ./internal/proxy/ -run TestIntegrationEgress -v  # 真实出网
```

对应 spec §7 验收清单:
1. `make build` 产出二进制 → Task 7 Step 6 ✓
2. 启动绑定 443 → 配置默认 443,Engine 测试用高位端口验证监听逻辑 ✓
3. SIGTERM 优雅关闭 → main.go 信号处理 + Engine.Stop 幂等 ✓
4. `/metrics` Prometheus 格式 → Task 5 测试 ✓
5. 客户端直连 → Task 8 集成测试 + README 分享链接说明 ✓

## Self-Review 记录

- **Spec 覆盖**:§4.1 装配/信号 → Task 6;§4.2 配置 → Task 1;§4.3 engine → Task 4;§4.4 httpapi → Task 5;§4.5 logging → Task 6;§5 schema → Task 1 + config.example.yaml;§6 构建/部署 → Task 7;§7 测试 → 各 Task 测试 + Task 8;§9 YAGNI 清单 → 无相关实现。
- **偏差记录**(已在 Global Constraints):Engine.Wait() 省略(API 不存在);私钥双编码兼容;metrics 不做 stats.Manager。
- **占位符扫描**:无 TBD/TODO;所有代码块完整。
- **类型一致性**:`config.ServerConfig`/`PrivateKeyBytes`/`ShortIDBytesList` 在 Task 1 定义、Task 3/4 消费,签名一致;`GenerateShareLink` 参数顺序在 Task 2 定义、Task 6 消费一致;`httpapi.EngineState` 与 `Engine.IsReady()` 匹配。

---

## 执行补充说明(2026-09-23,执行前实测)

1. **GOPROXY 必须指向阿里云镜像**:本机无法直连 proxy.golang.org 与 github git 协议。所有 `go get` / `go mod tidy` 命令加 `GOPROXY=https://mirrors.aliyun.com/goproxy,direct`(实测完整依赖树可下载并编译通过,go.sum 校验经代理 sumdb 正常)。
2. **xray-core 的 Go module 版本号是 `v1.260327.0`**:仓库 release 是 v26.3.27,但模块路径无 `/v26` 后缀,xray 官方为 Go module 消费者发布了平行 tag `v1.260327.0`(即 2026-03-27)。`go get ...@v26.3.27` 会解析为该版本。go.mod 中固定 `v1.260327.0`,API 与计划一致(已验证 `core.New`、`reality.Config.ShortIds`、`uuid.New`)。
3. 本机模块缓存已预填充完整 xray-core 依赖树,后续任务无需重复下载。
4. Task 3 执行时发现三处 API 与计划不符(已按 v1.260327.0 发布版修正,详见 .superpowers/sdd/progress.md):vlessinbound.Config 字段是 `Clients` 非 `Users`;`TypedMessage` 无 `UnmarshalTo` 方法;`app.go` 必须 blank import `app/proxyman/inbound` 与 `app/proxyman/outbound`(xray 的 config 注册依赖 init,缺了 core.New 会报 not registered)。
