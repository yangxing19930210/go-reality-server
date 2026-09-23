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
