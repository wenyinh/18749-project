package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/wenyinh/18749-project/monitor"
)

func main() {
	mode := flag.String("mode", "collect", "collect|baseline|detect")
	interval := flag.Duration("interval", time.Second, "sampling interval for collection")
	duration := flag.Duration("duration", 30*time.Second, "how long to collect metrics (collect mode)")
	diskPath := flag.String("disk_path", "/", "disk path for usage stats")
	output := flag.String("output", "", "path to write collected snapshot (collect mode)")
	input := flag.String("input", "", "metrics snapshot file (baseline/detect modes)")
	baselinePath := flag.String("baseline", "", "baseline file path")
	graphsDir := flag.String("graphs", "", "directory for PNG graphs (detect mode)")
	sensitivity := flag.Float64("sensitivity", 3, "stddev multiplier used for anomaly detection")
	faultType := flag.String("fault", "", "fault to inject during collect (cpu|memory)")
	faultDelay := flag.Duration("fault_after", 0, "delay before triggering the fault (collect mode)")
	faultDuration := flag.Duration("fault_duration", 0, "duration of the fault; 0=until collection ends")
	reportPath := flag.String("report", "", "optional path to store anomaly report JSON (detect mode)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	switch *mode {
	case "collect":
		runCollect(*interval, *duration, *diskPath, *faultType, *faultDelay, *faultDuration, *output)
	case "baseline":
		runBaseline(*input, *baselinePath)
	case "detect":
		runDetect(*input, *baselinePath, *graphsDir, *reportPath, *sensitivity)
	default:
		log.Fatalf("unknown mode: %s", *mode)
	}
}

func runCollect(interval, duration time.Duration, diskPath, faultType string, faultDelay, faultDuration time.Duration, output string) {
	if output == "" {
		log.Fatal("-output path is required in collect mode")
	}
	ctx := context.Background()
	snapshot, err := monitor.RunCollection(ctx, monitor.CollectOptions{
		Interval:      interval,
		Duration:      duration,
		DiskPath:      diskPath,
		FaultType:     faultType,
		FaultDelay:    faultDelay,
		FaultDuration: faultDuration,
	})
	if err != nil {
		log.Fatalf("collection failed: %v", err)
	}
	if err := monitor.SaveSnapshot(output, snapshot); err != nil {
		log.Fatalf("failed to save snapshot: %v", err)
	}
	faultDesc := snapshot.Metadata.FaultInjected
	if faultDesc == "" {
		faultDesc = "none"
	}
	log.Printf("collected %d samples over %.1fs (fault=%s) -> %s",
		snapshot.Metadata.SampleCount,
		snapshot.Metadata.DurationSeconds,
		faultDesc,
		output,
	)
}

func runBaseline(input, baselinePath string) {
	if input == "" {
		log.Fatal("-input snapshot is required for baseline mode")
	}
	if baselinePath == "" {
		log.Fatal("-baseline output path is required for baseline mode")
	}
	snapshot, err := monitor.LoadSnapshot(input)
	if err != nil {
		log.Fatalf("load snapshot: %v", err)
	}
	baseline := monitor.ComputeBaseline(snapshot)
	if err := monitor.SaveBaseline(baselinePath, baseline); err != nil {
		log.Fatalf("save baseline: %v", err)
	}
	log.Printf("saved baseline (%d samples, interval=%dms) -> %s",
		baseline.SampleCount, baseline.IntervalMillis, baselinePath)
}

func runDetect(input, baselinePath, graphsDir, reportPath string, sensitivity float64) {
	if input == "" {
		log.Fatal("-input snapshot is required for detect mode")
	}
	if baselinePath == "" {
		log.Fatal("-baseline is required for detect mode")
	}
	snapshot, err := monitor.LoadSnapshot(input)
	if err != nil {
		log.Fatalf("load snapshot: %v", err)
	}
	baseline, err := monitor.LoadBaseline(baselinePath)
	if err != nil {
		log.Fatalf("load baseline: %v", err)
	}
	anomalies := monitor.DetectAnomalies(snapshot, baseline, sensitivity)
	log.Printf("detected %d anomalies (sensitivity=%.2f)", len(anomalies), sensitivity)
	for _, a := range anomalies {
		log.Printf("  [%s] t=%s idx=%d val=%.2f threshold=%.2f (%s)",
			a.Metric, a.Timestamp.Format(time.RFC3339), a.SampleIdx, a.Value, a.Threshold, a.Direction)
	}

	if graphsDir != "" {
		if err := monitor.GeneratePlots(snapshot, anomalies, baseline, sensitivity, graphsDir); err != nil {
			log.Fatalf("generate plots: %v", err)
		}
		log.Printf("wrote graphs to %s", graphsDir)
	}

	if reportPath != "" {
		report := monitor.AnomalyReport{
			GeneratedAt:      time.Now(),
			Sensitivity:      sensitivity,
			BaselineSummary:  baseline,
			SnapshotMetadata: snapshot.Metadata,
			Anomalies:        anomalies,
		}
		if err := monitor.SaveAnomalyReport(reportPath, report); err != nil {
			log.Fatalf("save anomaly report: %v", err)
		}
		log.Printf("wrote anomaly report to %s", reportPath)
	}
}
