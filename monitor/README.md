# Black-box Failure Diagnosis Toolkit

This directory implements the Extra-Credit Option #1 requirements:

* **Black-box instrumentation** – `cmd/monitor` samples CPU %, memory usage, and disk utilization directly from the OS via `github.com/shirou/gopsutil/v3`.
* **Normal-behavior signature** – `monitor.ComputeBaseline` derives per-metric means/std-devs from fault-free runs and persists them as `baseline.json`.
* **Anomaly detection + diagnosis** – `monitor.DetectAnomalies` compares any run against the baseline, reports deviations, and `monitor.GeneratePlots` renders PNG graphs that highlight anomalies. The collector can inject CPU or memory pressure to emulate fail-slow faults, so you can demonstrate how anomalies manifest on the metrics.

## Usage workflow

1. **Collect a baseline dataset**
   ```bash
   go run ./cmd/monitor -mode collect -duration 45s -interval 1s -output data/normal.json
   ```
   Run the workload under normal load while the collector observes OS metrics.

2. **Derive the baseline signature**
   ```bash
   go run ./cmd/monitor -mode baseline -input data/normal.json -baseline data/baseline.json
   ```
   The resulting `baseline.json` stores the statistical signature of CPU, memory, and disk usage for normal behavior.

3. **Collect a run with an injected performance fault**
   ```bash
   go run ./cmd/monitor -mode collect -duration 60s -interval 1s \
     -fault cpu -fault_after 20s -fault_duration 25s \
     -output data/cpu_fault.json
   ```
   Supported faults: `cpu` (busy loop on every core) and `memory` (gradual allocations). `-fault_after` controls when the fault starts so you capture both healthy and degraded periods; `-fault_duration` bounds how long the fault runs (or let it last for the rest of the collection).

4. **Detect anomalies, generate plots, and record a report**
   ```bash
   go run ./cmd/monitor -mode detect \
     -input data/cpu_fault.json -baseline data/baseline.json \
     -sensitivity 2.5 -graphs graphs/cpu_fault -report reports/cpu_fault.json
   ```
   The detector prints anomalies to stdout, produces a JSON report you can drop into your submission, and renders PNG graphs per metric. Each graph overlays the baseline thresholds and marks anomalous points so you can visually diagnose when/why the system slowed down.

## Files of interest

* `cmd/monitor/main.go` – CLI entry point that wires together collection, baseline derivation, anomaly detection, reporting, and visualization.
* `monitor/metrics.go` – metric sampling pipeline plus snapshot serialization helpers.
* `monitor/faults.go` – CPU/memory fault injectors that emulate fail-slow conditions.
* `monitor/analyzer.go` – normal-behavior signature calculator and anomaly detector.
* `monitor/plot.go` – Gonum-based graph generator used to highlight anomalies.

This tooling is self-contained and does not require modifying the existing LFD/GFD/server code. Point it at any workload (including your replicated service) to demonstrate black-box failure diagnosis end-to-end.
