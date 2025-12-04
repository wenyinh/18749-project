package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/monitoring"
)

// Standalone monitoring server for black-box failure diagnosis
func main() {
	dashboardAddr := flag.String("dashboard", "localhost:9090", "Dashboard HTTP address")
	metricsInterval := flag.Duration("interval", 2*time.Second, "Metrics collection interval")
	maxHistory := flag.Int("history", 500, "Maximum number of metrics samples to keep")
	thresholdSigma := flag.Float64("threshold", 2.0, "Standard deviations threshold for anomaly detection")
	criticalSigma := flag.Float64("critical", 4.0, "Standard deviations threshold for critical anomalies")
	componentID := flag.String("id", "MONITOR", "Component ID for logging")
	autoBaseline := flag.Duration("auto-baseline", 30*time.Second, "Auto-establish baseline after this duration (0 to disable)")

	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[%s] Starting Black-Box Monitoring System", *componentID)
	log.Printf("[%s] Dashboard: http://%s", *componentID, *dashboardAddr)
	log.Printf("[%s] Metrics interval: %v", *componentID, *metricsInterval)
	log.Printf("[%s] Anomaly threshold: %.1f standard deviations", *componentID, *thresholdSigma)

	// Create monitoring components
	collector := monitoring.NewMetricsCollector(*componentID, *metricsInterval, *maxHistory)
	detector := monitoring.NewAnomalyDetector(*thresholdSigma, *criticalSigma, 100)
	injector := monitoring.NewFaultInjector()

	// Start metrics collection
	collector.Start()
	log.Printf("[%s] Metrics collection started", *componentID)

	// Auto-establish baseline if configured
	if *autoBaseline > 0 {
		go func() {
			log.Printf("[%s] Will auto-establish baseline after %v", *componentID, *autoBaseline)
			time.Sleep(*autoBaseline)

			metrics := collector.GetAllMetrics()
			if len(metrics) >= 10 {
				if err := detector.EstablishBaseline(metrics); err != nil {
					log.Printf("[%s] Failed to establish baseline: %v", *componentID, err)
				} else {
					log.Printf("[%s] ✅ Baseline established automatically", *componentID)
					detector.PrintBaseline()
				}
			} else {
				log.Printf("[%s] Not enough metrics to establish baseline (need 10, have %d)", *componentID, len(metrics))
			}
		}()
	}

	// Start anomaly detection loop
	go func() {
		ticker := time.NewTicker(*metricsInterval)
		defer ticker.Stop()

		for range ticker.C {
			latest := collector.GetLatestMetrics()
			if latest == nil {
				continue
			}

			anomalies := detector.CheckAnomaly(*latest)
			if len(anomalies) > 0 {
				for _, a := range anomalies {
					log.Printf("[%s] 🚨 ANOMALY DETECTED: [%s] %s", *componentID, a.Severity, a.Description)
				}
			}
		}
	}()

	// Periodic metrics printing (every 10 seconds)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			collector.PrintLatest()
		}
	}()

	// Start dashboard (blocking)
	dashboard := monitoring.NewDashboard(*dashboardAddr, collector, detector, injector)
	if err := dashboard.Start(); err != nil {
		log.Fatalf("[%s] Dashboard failed: %v", *componentID, err)
	}
}
