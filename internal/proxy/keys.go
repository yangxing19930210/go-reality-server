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
	u := uuid.New() // String 是指针接收者,需先落地为可寻址变量
	return u.String()
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
