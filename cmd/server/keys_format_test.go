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
		"VLESS_UUID":          regexp.MustCompile(`^VLESS_UUID=` + id + `$`),
		"VLESS_SHORT_ID":      regexp.MustCompile(`^VLESS_SHORT_ID=` + sid + `$`),
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
