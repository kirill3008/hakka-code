package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"hakka-code/internal/hakkacode"
)

func main() {
	addr := flag.String("addr", "", "Hakka WebSocket address (default: config file, then ws://127.0.0.1:8765/ws)")
	enableTags := flag.String("enable-tags", "", "Tool name or #tag to enable on startup (default: config file, then #all)")
	configPath := flag.String("config", hakkacode.DefaultConfigPath(), "Path to hakka-code config file")
	flag.Parse()

	fileCfg, err := hakkacode.LoadFileConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "hakka-code: read config %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	// os.Interrupt (Ctrl+C) is deliberately NOT wired into this context —
	// the App handles it itself, scoped to the lifetime of an in-flight
	// turn (cancel the turn), falling back to Go's default
	// terminate-the-process behavior whenever no turn is running (e.g.
	// at the idle REPL prompt).
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	cfg := hakkacode.Config{
		Addr:       firstNonEmpty(*addr, fileCfg.Addr, "ws://127.0.0.1:8765/ws"),
		EnableTags: firstNonEmpty(*enableTags, fileCfg.EnableTags),
		CWD:        mustGetwd(),
	}

	if err := hakkacode.Run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "hakka-code: %v\n", err)
		os.Exit(1)
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hakka-code: get cwd: %v\n", err)
		os.Exit(1)
	}
	return wd
}
