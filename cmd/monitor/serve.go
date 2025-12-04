package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/wenyinh/18749-project/monitor"
)

func runServe(interval time.Duration, diskPath, baselinePath, listen string, historySamples int, sensitivity float64) {
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	live := monitor.NewLiveMonitor(interval, diskPath, historySamples, sensitivity)
	if baselinePath != "" {
		baseline, err := monitor.LoadBaseline(baselinePath)
		if err != nil {
			log.Fatalf("load baseline: %v", err)
		}
		live.SetBaseline(baseline)
	}

	go func() {
		if err := live.Run(rootCtx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("live monitor stopped: %v", err)
		}
	}()

	server := &http.Server{
		Addr:    listen,
		Handler: newUIServer(live),
	}

	go func() {
		<-rootCtx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("serve mode ready at %s (interval=%s, history=%d samples)", listen, interval, historySamples)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server: %v", err)
	}
}
