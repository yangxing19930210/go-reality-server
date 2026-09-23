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

// TestKeysGenerateArgs 回归测试:CLI 形态 `keys generate -server X` 中
// "generate" 必须在交给 flag 解析前剥离,否则 flag 会在该非 flag 参数处
// 停止解析,-server 被静默忽略、分享链接丢失。
func TestKeysGenerateArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"with generate verb", []string{"generate", "-server", "1.2.3.4"}, "-server 1.2.3.4"},
		{"without generate verb", []string{"-server", "1.2.3.4"}, "-server 1.2.3.4"},
		{"verb only", []string{"generate"}, ""},
		{"empty", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := strings.Join(keysGenerateArgs(tc.in), " "); got != tc.want {
				t.Errorf("keysGenerateArgs(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	// 端到端:完整 CLI 参数形态必须产出分享链接
	var buf bytes.Buffer
	if err := runKeysGenerate(&buf, keysGenerateArgs([]string{"generate", "-server", "1.2.3.4"})); err != nil {
		t.Fatalf("runKeysGenerate: %v", err)
	}
	if !strings.Contains(buf.String(), "ShareLink") {
		t.Errorf("CLI form `keys generate -server X` lost share link:\n%s", buf.String())
	}
}
