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
