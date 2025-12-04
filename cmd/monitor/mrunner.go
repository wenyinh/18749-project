package main

import (
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/monitor"
)

// go run ./cmd/monitor -duration 60s -baseline 20s -fault cpu -fault-after 25s -fault-duration 20s
func main() {
	duration := flag.Duration("duration", 60*time.Second, "total monitoring duration")
	baseline := flag.Duration("baseline", 20*time.Second, "time window for learning baseline signature")
	interval := flag.Duration("interval", 2*time.Second, "sampling interval")
	threshold := flag.Float64("threshold", 2.5, "stddev multiplier used for anomaly detection")
	fault := flag.String("fault", "none", "inject a slowdown fault: none|cpu|mem")
	faultAfter := flag.Duration("fault-after", 25*time.Second, "delay before injecting the fault")
	faultDuration := flag.Duration("fault-duration", 25*time.Second, "duration of injected fault (0 means until shutdown)")
	faultMem := flag.Int("fault-mem-mb", 512, "target memory footprint when fault=mem")
	jsonOut := flag.String("json", "monitor/metrics.json", "path to write raw samples JSON")
	htmlOut := flag.String("report", "report.html", "path to write HTML report")

	flag.Parse()

	start := time.Now()
	end := start.Add(*duration)
	collector := monitor.NewCollector(start, *baseline, *threshold)

	log.Printf("[monitor] collecting for %v (baseline=%v, interval=%v, threshold=%.2fσ)", *duration, *baseline, *interval, *threshold)

	// Take an immediate first sample so short runs still yield data.
	now := time.Now()
	firstSample, err := monitor.CollectSample()
	if err != nil {
		log.Printf("[monitor] collect error: %v", err)
	}
	collector.Process(firstSample, now)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	var faultHandle *monitor.FaultHandle
	faultStarted := false
	faultLabel := ""

	for tick := range ticker.C {
		if tick.After(end) {
			break
		}

		sample, err := monitor.CollectSample()
		if err != nil {
			log.Printf("[monitor] collect error: %v", err)
		}
		processed := collector.Process(sample, tick)

		if processed.Anomaly {
			log.Printf("[ANOMALY] %s :: %v", processed.Timestamp.Format(time.RFC3339), processed.Reasons)
		}

		if !faultStarted && *fault != "none" && tick.After(start.Add(*faultAfter)) {
			faultHandle = monitor.StartFault(*fault, *faultDuration, *faultMem)
			if faultHandle != nil {
				faultStarted = true
				faultLabel = *fault
				if *faultDuration > 0 {
					log.Printf("[monitor] injected %s fault for %v", *fault, *faultDuration)
				} else {
					log.Printf("[monitor] injected %s fault (until shutdown)", *fault)
				}
			} else {
				log.Printf("[monitor] fault '%s' not recognized; skipping injection", *fault)
			}
		}
	}

	if faultHandle != nil {
		faultHandle.Stop()
	}

	report := collector.Report(time.Now(), *interval, faultLabel)

	if err := monitor.WriteJSON(*jsonOut, report); err != nil {
		log.Printf("[monitor] failed to write JSON to %s: %v", *jsonOut, err)
	} else {
		log.Printf("[monitor] wrote samples to %s", *jsonOut)
	}

	if err := monitor.GenerateHTMLReport(report, *htmlOut); err != nil {
		log.Printf("[monitor] failed to render HTML report: %v", err)
	} else {
		log.Printf("[monitor] wrote HTML report to %s", *htmlOut)
	}
}
