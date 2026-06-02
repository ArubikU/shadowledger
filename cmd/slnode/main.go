// Command slnode runs a ShadowLedger node daemon.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ArubikU/shadowledger/internal/node"
)

func main() {
	cfgPath := flag.String("config", "node.yaml", "path to node config YAML")
	flag.Parse()

	cfg, err := node.LoadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	n, err := node.New(cfg)
	if err != nil {
		log.Fatalf("init node: %v", err)
	}
	log.Printf("ShadowLedger node %s starting (data=%s)", n.Address(), cfg.DataDir)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := n.Run(ctx); err != nil {
		log.Fatalf("run: %v", err)
	}
	log.Printf("shutdown complete")
}
