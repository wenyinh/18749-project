package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wenyinh/18749-project/faults"
	"github.com/wenyinh/18749-project/metrics"
	"github.com/wenyinh/18749-project/server"
)

func main() {
	var (
		replicaID      = flag.String("id", "S1", "server replica ID")
		port           = flag.Int("port", 9001, "server listening port")
		initState      = flag.Int("init", 0, "initial server state")
		newborn        = flag.Bool("newborn", false, "start as newborn (not ready)")
		ckptMs         = flag.Int("ckpt-ms", 5000, "checkpoint broadcast interval in ms")
		metricsPort    = flag.Int("metrics-port", 8080, "metrics HTTP server port")
		metricsInterval = flag.Int("metrics-interval", 1000, "metrics collection interval in ms")
		maxSamples     = flag.Int("max-samples", 1000, "max number of metric samples to keep")
	)
	flag.Parse()

	// Parse backup servers from remaining args
	backups := make(map[string]string)
	args := flag.Args()
	for i := 0; i+1 < len(args); i += 2 {
		backupID := args[i]
		backupAddr := args[i+1]
		backups[backupID] = backupAddr
	}

	addr := fmt.Sprintf(":%d", *port)
	ckptFreq := time.Duration(*ckptMs) * time.Millisecond

	log.Printf("=====================================")
	log.Printf("Server with Black-Box Metrics")
	log.Printf("=====================================")
	log.Printf("Server ID: %s", *replicaID)
	log.Printf("Server Address: %s", addr)
	log.Printf("Initial State: %d", *initState)
	log.Printf("Newborn: %v", *newborn)
	log.Printf("Checkpoint Interval: %v", ckptFreq)
	log.Printf("Metrics Port: %d", *metricsPort)
	log.Printf("Metrics Interval: %dms", *metricsInterval)
	log.Printf("Backups: %v", backups)
	log.Printf("=====================================")

	// Create metrics collector
	collector := metrics.NewCollector(
		time.Duration(*metricsInterval)*time.Millisecond,
		*maxSamples,
	)
	collector.Start()
	log.Printf("[METRICS] Metrics collector started")

	// Create fault injector
	injector := faults.NewInjector()
	log.Printf("[METRICS] Fault injector initialized")

	// Start metrics HTTP server
	metricsServer := metrics.NewServer(collector, injector, *metricsPort)
	go func() {
		if err := metricsServer.Start(); err != nil {
			log.Fatalf("[METRICS] Failed to start metrics server: %v", err)
		}
	}()

	// Create and start the actual server
	srv := server.NewServer(addr, *replicaID, *initState, *newborn, backups, ckptFreq)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("\n[SHUTDOWN] Received shutdown signal")
		collector.Stop()
		injector.StopFault()
		log.Printf("[SHUTDOWN] Cleaned up metrics and fault injection")
		os.Exit(0)
	}()

	// Run the server (blocking)
	if err := srv.Run(); err != nil {
		log.Fatalf("[SERVER] Server failed: %v", err)
	}
}
