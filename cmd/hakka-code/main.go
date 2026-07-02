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
	addr := flag.String("addr", "ws://127.0.0.1:8765/ws", "Hakka WebSocket address")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := hakkacode.Config{
		Addr: *addr,
		CWD:  mustGetwd(),
	}

	if err := hakkacode.Run(ctx, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "hakka-code: %v\n", err)
		os.Exit(1)
	}
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "hakka-code: get cwd: %v\n", err)
		os.Exit(1)
	}
	return wd
}
