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
	"strings"
	"syscall"
	"time"

	"go-reality-server/internal/config"
	"go-reality-server/internal/httpapi"
	"go-reality-server/internal/logging"
	"go-reality-server/internal/proxy"
)

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

// keysGenerateArgs 剥离可选的 "generate" 动词后再交给 flag 解析。
// flag 解析会在第一个非 flag 参数处停止,若把 "generate" 一起传入,
// 其后的 -server/-port 等参数会被静默忽略(分享链接不会输出)。
func keysGenerateArgs(args []string) []string {
	if len(args) > 0 && args[0] == "generate" {
		return args[1:]
	}
	return args
}

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
