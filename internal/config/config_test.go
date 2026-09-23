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
		{"vision flow rejected", func(c *ServerConfig) { c.VLESS.Flow = "xtls-rprx-vision" }},
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
