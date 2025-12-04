package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wenyinh/18749-project/monitor"
)

type streamEvent struct {
	Sample        monitor.Sample            `json:"sample"`
	Baseline      monitor.BaselineSignature `json:"baseline"`
	BaselineReady bool                      `json:"baseline_ready"`
	Threshold     float64                   `json:"threshold"`
}

type broadcaster struct {
	mu   sync.Mutex
	subs map[chan streamEvent]struct{}
}

type faultManager struct {
	mu     sync.Mutex
	handle *monitor.FaultHandle
	kind   string
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan streamEvent]struct{})}
}

func (b *broadcaster) subscribe() chan streamEvent {
	ch := make(chan streamEvent, 8)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan streamEvent) {
	b.mu.Lock()
	if _, ok := b.subs[ch]; ok {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

func (b *broadcaster) publish(ev streamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			// drop if subscriber is slow
		}
	}
}

func (f *faultManager) start(kind string, duration time.Duration, memMB int) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handle != nil {
		f.handle.Stop()
		f.handle = nil
		f.kind = ""
	}
	handle := monitor.StartFault(kind, duration, memMB)
	if handle == nil {
		return "", fmt.Errorf("unknown fault kind: %s", kind)
	}
	f.handle = handle
	f.kind = kind
	return fmt.Sprintf("%s fault running (duration=%v, memMB=%d)", kind, duration, memMB), nil
}

func (f *faultManager) stop() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.handle != nil {
		f.handle.Stop()
		f.handle = nil
		stopped := f.kind
		f.kind = ""
		return fmt.Sprintf("stopped %s fault", stopped)
	}
	return "no active fault to stop"
}

func main() {
	interval := flag.Duration("interval", 2*time.Second, "sampling interval")
	baseline := flag.Duration("baseline", 20*time.Second, "time window for learning baseline signature")
	threshold := flag.Float64("threshold", 2.5, "stddev multiplier used for anomaly detection")
	listen := flag.String("listen", ":8081", "address for live monitoring UI")
	duration := flag.Duration("duration", 0, "optional total runtime (0 means run until Ctrl+C)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[monitor-live] starting on %s (interval=%v baseline=%v threshold=%.2fσ)", *listen, *interval, *baseline, *threshold)

	start := time.Now()
	collector := monitor.NewCollector(start, *baseline, *threshold)
	bcast := newBroadcaster()
	fmgr := &faultManager{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if *duration > 0 {
		go func() {
			<-time.After(*duration)
			log.Printf("[monitor-live] reached duration %v, shutting down", *duration)
			cancel()
		}()
	}

	// Graceful stop on Ctrl+C.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		<-sigCh
		log.Printf("[monitor-live] received interrupt, shutting down")
		cancel()
	}()

	// Sampling loop.
	go func() {
		ticker := time.NewTicker(*interval)
		defer ticker.Stop()

		// Initial sample so UI has something immediately.
		if s, err := monitor.CollectSample(); err == nil {
			processed := collector.Process(s, time.Now())
			baselineSnap, ready := collector.BaselineSnapshot()
			bcast.publish(streamEvent{Sample: processed, Baseline: baselineSnap, BaselineReady: ready, Threshold: *threshold})
		} else {
			log.Printf("[monitor-live] collect error: %v", err)
		}

		for {
			select {
			case now := <-ticker.C:
				sample, err := monitor.CollectSample()
				if err != nil {
					log.Printf("[monitor-live] collect error: %v", err)
					continue
				}
				processed := collector.Process(sample, now)
				if processed.Anomaly {
					log.Printf("[ANOMALY] %s :: %v", processed.Timestamp.Format(time.RFC3339), processed.Reasons)
				}
				baselineSnap, ready := collector.BaselineSnapshot()
				bcast.publish(streamEvent{Sample: processed, Baseline: baselineSnap, BaselineReady: ready, Threshold: *threshold})
			case <-ctx.Done():
				return
			}
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(liveHTML))
	})

	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := bcast.subscribe()
		defer bcast.unsubscribe(ch)

		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(ev)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	mux.HandleFunc("/fault", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			kind := r.FormValue("kind")
			if kind == "" {
				http.Error(w, "missing kind", http.StatusBadRequest)
				return
			}
			durStr := r.FormValue("duration")
			memStr := r.FormValue("mem_mb")

			dur := 20 * time.Second
			if durStr != "" {
				if parsed, err := time.ParseDuration(durStr); err == nil {
					dur = parsed
				}
			}
			memMB := 512
			if memStr != "" {
				if parsed, err := strconv.Atoi(memStr); err == nil && parsed > 0 {
					memMB = parsed
				}
			}
			msg, err := fmgr.start(kind, dur, memMB)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			log.Printf("[monitor-live] %s", msg)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": msg})
		case http.MethodDelete:
			msg := fmgr.stop()
			log.Printf("[monitor-live] %s", msg)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": msg})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	server := &http.Server{
		Addr:    *listen,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	log.Printf("[monitor-live] UI available at http://%s", strings.TrimPrefix(*listen, ":"))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[monitor-live] server error: %v", err)
	}
}

// Minimal single-page UI; pure SVG for lightweight live graphs.
const liveHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Live Black-Box Monitor</title>
  <style>
    body { font-family: "Segoe UI", system-ui, -apple-system, sans-serif; margin: 0; padding: 16px; background: #0b1221; color: #e2e8f0; }
    h1 { margin: 0 0 8px; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; }
    .card { background: #0f172a; border-radius: 12px; padding: 12px 14px; box-shadow: 0 10px 25px rgba(0,0,0,0.2); }
    .chart { width: 100%; height: 210px; }
    .pill { display: inline-block; padding: 4px 10px; border-radius: 999px; font-size: 12px; }
    .pill.good { background: #083344; color: #38bdf8; }
    .pill.bad { background: #3f1d2e; color: #f87171; }
    table { width: 100%; border-collapse: collapse; font-size: 13px; }
    th, td { padding: 6px 8px; text-align: left; }
    th { background: #0b162c; }
    tr:nth-child(even) { background: #0e1b32; }
    #status { display: flex; gap: 8px; align-items: center; margin-bottom: 12px; }
    .actions { display: flex; gap: 8px; margin-bottom: 12px; flex-wrap: wrap; }
    button { background: #1d4ed8; color: #e2e8f0; border: none; border-radius: 10px; padding: 8px 12px; cursor: pointer; font-weight: 600; }
    button:hover { background: #2563eb; }
    button.secondary { background: #334155; }
    button.danger { background: #b91c1c; }
  </style>
</head>
<body>
  <h1>Live Black-Box Monitor</h1>
  <div id="status">
    <span class="pill good" id="baseline-pill">Learning baseline...</span>
    <span class="pill good" id="threshold-pill">Threshold: --</span>
    <span class="pill bad" id="anomaly-pill" style="display:none">Anomaly detected</span>
  </div>
  <div class="actions">
    <button onclick="triggerFault('cpu')" title="CPU hog for ~20s">Inject CPU Fault</button>
    <button onclick="triggerFault('mem')" title="Allocates ~800MB for ~20s">Inject Memory Fault</button>
    <button class="secondary" onclick="stopFault()">Stop Fault</button>
  </div>
  <div class="grid">
    <div class="card">
      <h3>CPU Usage (%)</h3>
      <svg id="cpu-chart" class="chart"></svg>
    </div>
    <div class="card">
      <h3>Memory Usage (%)</h3>
      <svg id="mem-chart" class="chart"></svg>
    </div>
    <div class="card">
      <h3>Disk Usage (%)</h3>
      <svg id="disk-chart" class="chart"></svg>
    </div>
  </div>
  <div class="card" style="margin-top:12px;">
    <h3>Latest Anomalies</h3>
    <table>
      <thead><tr><th>Time</th><th>Reasons</th></tr></thead>
      <tbody id="anomaly-rows"></tbody>
    </table>
  </div>

  <script>
    const samples = [];
    const maxPoints = 360;
    let baseline = null;
    let baselineReady = false;
    let threshold = null;

    const evt = new EventSource("/stream");
    evt.onmessage = function(msg) {
      const data = JSON.parse(msg.data);
      if (!data || !data.sample) return;
      samples.push(data.sample);
      if (samples.length > maxPoints) { samples.shift(); }
      baseline = data.baseline;
      baselineReady = data.baseline_ready;
      threshold = data.threshold;
      render();
    };

    function render() {
      updatePills();
      renderMetric("cpu-chart", "cpu_percent", "CPU", "%", baseline ? baseline.cpu : null);
      renderMetric("mem-chart", "mem_percent", "Mem", "%", baseline ? baseline.memory : null);
      renderMetric("disk-chart", "disk_percent", "Disk", "%", baseline ? baseline.disk : null);
      renderAnomalies();
    }

    function updatePills() {
      const base = document.getElementById("baseline-pill");
      if (baselineReady) {
        base.textContent = "Baseline locked";
        base.className = "pill good";
      } else {
        base.textContent = "Learning baseline...";
        base.className = "pill good";
      }
      const thr = document.getElementById("threshold-pill");
      thr.textContent = "Threshold: " + (threshold ? threshold.toFixed(2) + "σ" : "--");

      const anomalyPill = document.getElementById("anomaly-pill");
      const latest = samples.length ? samples[samples.length-1] : null;
      if (latest && latest.anomaly) {
        anomalyPill.style.display = "inline-block";
        anomalyPill.textContent = "Anomaly: " + (latest.reasons || []).join("; ");
      } else {
        anomalyPill.style.display = "none";
      }
    }

    function renderMetric(svgId, field, label, unit, baseStats) {
      const svg = document.getElementById(svgId);
      if (!svg) return;
      const width = svg.clientWidth || 600;
      const height = svg.clientHeight || 210;
      const pad = 42;
      svg.setAttribute("viewBox", "0 0 " + width + " " + height);
      svg.innerHTML = "";
      if (!samples.length) return;

      const times = samples.map(s => new Date(s.timestamp).getTime());
      const values = samples.map(s => s[field]);
      const minT = Math.min.apply(null, times);
      const maxT = Math.max.apply(null, times);
      const tRange = Math.max(maxT - minT, 1);
      const minV = Math.min.apply(null, values);
      const maxV = Math.max.apply(null, values);
      const vRange = Math.max(maxV - minV, 0.01);

      const scaleX = t => pad + ((t - minT) / tRange) * (width - 2 * pad);
      const scaleY = v => (height - pad) - ((v - minV) / vRange) * (height - 2 * pad);

      svg.innerHTML += '<line x1="' + pad + '" y1="' + (height - pad) + '" x2="' + (width - pad/2) + '" y2="' + (height - pad) + '" stroke="#1f2937" />';
      svg.innerHTML += '<line x1="' + pad + '" y1="' + (pad/2) + '" x2="' + pad + '" y2="' + (height - pad) + '" stroke="#1f2937" />';

      const path = samples.map(function(s, idx) {
        const x = scaleX(new Date(s.timestamp).getTime()).toFixed(2);
        const y = scaleY(s[field]).toFixed(2);
        return (idx === 0 ? "M" : "L") + x + "," + y;
      }).join(" ");
      svg.innerHTML += '<path d="' + path + '" fill="none" stroke="#38bdf8" stroke-width="2.2" />';

      if (baseStats && typeof baseStats.mean === "number") {
        const meanY = scaleY(baseStats.mean).toFixed(2);
        svg.innerHTML += '<line x1="' + pad + '" y1="' + meanY + '" x2="' + (width - pad/2) + '" y2="' + meanY + '" stroke="#22d3ee" stroke-dasharray="6 4" />';
      }
      if (baseStats && baseStats.stddev > 0 && threshold) {
        const upper = baseStats.mean + threshold * baseStats.stddev;
        const upperY = scaleY(upper).toFixed(2);
        svg.innerHTML += '<line x1="' + pad + '" y1="' + upperY + '" x2="' + (width - pad/2) + '" y2="' + upperY + '" stroke="#f97316" stroke-dasharray="3 6" />';
      }

      samples.forEach(function(s) {
        if (!s.anomaly) return;
        const x = scaleX(new Date(s.timestamp).getTime()).toFixed(2);
        const y = scaleY(s[field]).toFixed(2);
        svg.innerHTML += '<circle cx="' + x + '" cy="' + y + '" r="4" fill="#ef4444"><title>' + label + " " + s[field].toFixed(2) + unit + '\n' + (s.reasons||[]).join("; ") + '</title></circle>';
      });

      const minLabel = new Date(minT).toLocaleTimeString();
      const maxLabel = new Date(maxT).toLocaleTimeString();
      svg.innerHTML += '<text x="' + pad + '" y="' + (height - pad + 16) + '" font-size="11" fill="#94a3b8">' + minLabel + '</text>';
      svg.innerHTML += '<text x="' + (width - pad) + '" y="' + (height - pad + 16) + '" font-size="11" fill="#94a3b8" text-anchor="end">' + maxLabel + '</text>';
    }

    function renderAnomalies() {
      const tbody = document.getElementById("anomaly-rows");
      if (!tbody) return;
      const anomalies = samples.filter(s => s.anomaly).slice(-8).reverse();
      if (!anomalies.length) {
        tbody.innerHTML = "<tr><td colspan='2'>No anomalies yet</td></tr>";
        return;
      }
      tbody.innerHTML = anomalies.map(function(s) {
        const ts = new Date(s.timestamp).toLocaleTimeString();
        const reasons = (s.reasons || []).join("; ");
        return "<tr><td>" + ts + "</td><td>" + reasons + "</td></tr>";
      }).join("");
    }

    function triggerFault(kind) {
      const params = new URLSearchParams();
      params.append("kind", kind);
      if (kind === "mem") {
        params.append("mem_mb", "800");
      }
      params.append("duration", "20s");
      fetch("/fault", { method: "POST", body: params, headers: { "Content-Type": "application/x-www-form-urlencoded" } })
        .then(r => r.json())
        .then(res => console.log(res.status))
        .catch(err => console.error(err));
    }

    function stopFault() {
      fetch("/fault", { method: "DELETE" })
        .then(r => r.json())
        .then(res => console.log(res.status))
        .catch(err => console.error(err));
    }
  </script>
</body>
</html>`
