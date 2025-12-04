package monitor

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"time"
)

// GenerateHTMLReport renders a standalone HTML page with charts and anomaly callouts.
func GenerateHTMLReport(report Report, outPath string) error {
	jsonData, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	model := struct {
		Report       Report
		ReportJSON   template.JS
		SampleCount  int
		AnomalyCount int
	}{
		Report:       report,
		ReportJSON:   template.JS(string(jsonData)),
		SampleCount:  len(report.Samples),
		AnomalyCount: countAnomalies(report.Samples),
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	tpl := template.Must(template.New("report").Parse(reportTemplate))
	return tpl.Execute(f, model)
}

func countAnomalies(samples []Sample) int {
	total := 0
	for _, s := range samples {
		if s.Anomaly {
			total++
		}
	}
	return total
}

const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Black-Box Monitoring Report</title>
  <style>
    body { font-family: "Segoe UI", system-ui, -apple-system, sans-serif; color: #0f172a; margin: 0; padding: 24px; background: linear-gradient(135deg,#f8fafc,#eef2ff); }
    h1 { margin-top: 0; font-size: 28px; }
    h2 { margin: 24px 0 12px; }
    section { background: #fff; border-radius: 12px; padding: 16px 20px; margin-bottom: 18px; box-shadow: 0 10px 30px rgba(15,23,42,0.08); }
    .pill { display: inline-block; padding: 4px 10px; border-radius: 999px; font-size: 12px; background: #e0f2fe; color: #0ea5e9; }
    .pill.bad { background: #fee2e2; color: #dc2626; }
    table { width: 100%; border-collapse: collapse; font-size: 14px; }
    th, td { text-align: left; padding: 8px; }
    th { background: #f8fafc; }
    tr:nth-child(even) { background: #f9fafb; }
    .chart { width: 100%; height: 260px; }
    .legend { font-size: 12px; color: #475569; margin-top: 6px; }
    .summary-grid { display: grid; grid-template-columns: repeat(auto-fit,minmax(180px,1fr)); gap: 10px; }
    .summary-item { background:#f8fafc; border-radius: 10px; padding: 12px; }
    code { background: #f1f5f9; padding: 2px 5px; border-radius: 6px; }
  </style>
</head>
<body>
  <h1>Black-Box Monitoring Report</h1>
  <section>
    <div class="summary-grid">
      <div class="summary-item"><strong>Samples</strong><br>{{ .SampleCount }}</div>
      <div class="summary-item"><strong>Anomalies</strong><br><span class="pill {{ if gt .AnomalyCount 0 }}bad{{ end }}">{{ .AnomalyCount }}</span></div>
      <div class="summary-item"><strong>Start</strong><br>{{ .Report.StartTime }}</div>
      <div class="summary-item"><strong>End</strong><br>{{ .Report.EndTime }}</div>
      <div class="summary-item"><strong>Interval</strong><br>{{ .Report.IntervalMs }} ms</div>
      <div class="summary-item"><strong>Duration</strong><br>{{ printf "%.1f" .Report.DurationSec }} s</div>
      <div class="summary-item"><strong>Threshold</strong><br>{{ printf "%.2fσ" .Report.Threshold }}</div>
      <div class="summary-item"><strong>Fault</strong><br>{{ if .Report.FaultInjected }}{{ .Report.FaultInjected }}{{ else }}none{{ end }}</div>
    </div>
  </section>

  <section>
    <h2>Baseline (Normal Behavior)</h2>
    <div class="summary-grid">
      <div class="summary-item"><strong>CPU</strong><br>μ={{ printf "%.2f" .Report.Baseline.CPU.Mean }}% | σ={{ printf "%.2f" .Report.Baseline.CPU.StdDev }}</div>
      <div class="summary-item"><strong>Memory</strong><br>μ={{ printf "%.2f" .Report.Baseline.Memory.Mean }}% | σ={{ printf "%.2f" .Report.Baseline.Memory.StdDev }}</div>
      <div class="summary-item"><strong>Disk</strong><br>μ={{ printf "%.2f" .Report.Baseline.Disk.Mean }}% | σ={{ printf "%.2f" .Report.Baseline.Disk.StdDev }}</div>
      <div class="summary-item"><strong>Samples Used</strong><br>{{ .Report.Baseline.Count }}</div>
      <div class="summary-item"><strong>Comment</strong><br>{{ .Report.Baseline.Comment }}</div>
    </div>
  </section>

  <section>
    <h2>CPU Usage</h2>
    <svg id="cpu-chart" class="chart"></svg>
    <div class="legend">Line = sample values, dashed = baseline mean, dotted = anomaly threshold, red dots = flagged anomalies</div>
  </section>
  <section>
    <h2>Memory Usage</h2>
    <svg id="mem-chart" class="chart"></svg>
    <div class="legend">Memory tracked as percent of total (and used MB in tooltip)</div>
  </section>
  <section>
    <h2>Disk Usage</h2>
    <svg id="disk-chart" class="chart"></svg>
  </section>

  <section>
    <h2>Detected Anomalies</h2>
    <table>
      <thead>
        <tr><th>Timestamp</th><th>Reasons</th></tr>
      </thead>
      <tbody id="anomaly-rows"></tbody>
    </table>
  </section>

  <script>
    const report = {{ .ReportJSON }};

    function renderMetric(svgId, field, label, unit, baseline) {
      const svg = document.getElementById(svgId);
      const samples = report.samples;
      if (!svg || !samples.length) return;

      const width = svg.clientWidth || 900;
      const height = svg.clientHeight || 260;
      const pad = 48;
      svg.setAttribute("viewBox", "0 0 " + width + " " + height);
      svg.innerHTML = "";

      const times = samples.map(s => new Date(s.timestamp).getTime());
      const values = samples.map(s => s[field]);
      const minT = Math.min(...times);
      const maxT = Math.max(...times);
      const tRange = Math.max(maxT - minT, 1);
      const minV = Math.min(...values);
      const maxV = Math.max(...values);
      const vRange = Math.max(maxV - minV, 0.001);

      const scaleX = t => pad + ((t - minT) / tRange) * (width - 2 * pad);
      const scaleY = v => (height - pad) - ((v - minV) / vRange) * (height - 2 * pad);

      // Axes
      svg.innerHTML += '<line x1="' + pad + '" y1="' + (height - pad) + '" x2="' + (width - pad/2) + '" y2="' + (height - pad) + '" stroke="#cbd5e1" />';
      svg.innerHTML += '<line x1="' + pad + '" y1="' + (pad/2) + '" x2="' + pad + '" y2="' + (height - pad) + '" stroke="#cbd5e1" />';

      // Path
      const path = samples.map((s, idx) => {
        const t = new Date(s.timestamp).getTime();
        const x = scaleX(t).toFixed(2);
        const y = scaleY(s[field]).toFixed(2);
        return (idx === 0 ? "M" : "L") + x + "," + y;
      }).join(" ");
      svg.innerHTML += '<path d="' + path + '" fill="none" stroke="#2563eb" stroke-width="2.2" />';

      // Baseline mean
      if (baseline && baseline.Mean !== undefined) {
        const meanY = scaleY(baseline.Mean).toFixed(2);
        svg.innerHTML += '<line x1="' + pad + '" y1="' + meanY + '" x2="' + (width - pad/2) + '" y2="' + meanY + '" stroke="#0ea5e9" stroke-dasharray="6 4" />';
      }
      // Baseline + threshold
      if (baseline && baseline.StdDev !== undefined && baseline.StdDev > 0) {
        const upper = baseline.Mean + report.threshold * baseline.StdDev;
        const upperY = scaleY(upper).toFixed(2);
        svg.innerHTML += '<line x1="' + pad + '" y1="' + upperY + '" x2="' + (width - pad/2) + '" y2="' + upperY + '" stroke="#f97316" stroke-dasharray="3 6" />';
      }

      // Anomaly markers
      samples.forEach(s => {
        if (!s.anomaly) return;
        const t = new Date(s.timestamp).getTime();
        const x = scaleX(t).toFixed(2);
        const y = scaleY(s[field]).toFixed(2);
        svg.innerHTML += '<circle cx="' + x + '" cy="' + y + '" r="4" fill="#dc2626"><title>' + label + " " + s[field].toFixed(2) + unit + '\n' + (s.reasons||[]).join("; ") + "</title></circle>";
      });

      // Labels
      const minLabel = new Date(minT).toLocaleTimeString();
      const maxLabel = new Date(maxT).toLocaleTimeString();
      svg.innerHTML += '<text x="' + pad + '" y="' + (height - pad + 18) + '" font-size="11" fill="#475569">' + minLabel + "</text>";
      svg.innerHTML += '<text x="' + (width - pad) + '" y="' + (height - pad + 18) + '" font-size="11" fill="#475569" text-anchor="end">' + maxLabel + "</text>";
      svg.innerHTML += '<text x="' + pad + '" y="' + (pad/1.5) + '" font-size="12" fill="#0f172a">' + label + " (" + unit + ")</text>";
    }

    function renderAnomalyTable() {
      const tbody = document.getElementById("anomaly-rows");
      if (!tbody) return;
      const anomalies = report.samples.filter(s => s.anomaly);
      if (!anomalies.length) {
        tbody.innerHTML = "<tr><td colspan='2'>No anomalies detected</td></tr>";
        return;
      }
      tbody.innerHTML = anomalies.map(s => {
        const ts = new Date(s.timestamp).toLocaleString();
        const reasons = (s.reasons || []).join("; ");
        return "<tr><td>" + ts + "</td><td>" + reasons + "</td></tr>";
      }).join("");
    }

    document.addEventListener("DOMContentLoaded", () => {
      renderMetric("cpu-chart", "cpu_percent", "CPU Usage", "%", report.baseline.cpu);
      renderMetric("mem-chart", "mem_percent", "Memory Usage", "%", report.baseline.memory);
      renderMetric("disk-chart", "disk_percent", "Disk Usage", "%", report.baseline.disk);
      renderAnomalyTable();
    });
  </script>
</body>
</html>`

// HumanTime formats a time in a predictable way for template outputs.
func HumanTime(t time.Time) string {
	return t.Format(time.RFC3339)
}
