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
