package monitor

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
)

var metricLabels = map[string]string{
	"cpu_percent":       "CPU Utilization (%)",
	"memory_percent":    "Memory Utilization (%)",
	"memory_used_mb":    "Memory Used (MB)",
	"disk_used_percent": "Disk Utilization (%)",
	"disk_used_gb":      "Disk Used (GB)",
}

// GeneratePlots renders PNG graphs with the collected samples and detected anomalies.
func GeneratePlots(snapshot MetricsSnapshot, anomalies []Anomaly, baseline BaselineSpec, sensitivity float64, outDir string) error {
	if outDir == "" {
		return nil
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("create plot directory: %w", err)
	}
	if len(snapshot.Samples) == 0 {
		return fmt.Errorf("no samples available for plotting")
	}

	start := snapshot.Samples[0].Timestamp
	anomaliesByMetric := make(map[string][]Anomaly)
	for _, a := range anomalies {
		anomaliesByMetric[a.Metric] = append(anomaliesByMetric[a.Metric], a)
	}

	for metric, extractor := range metricExtractors {
		pts := make(plotter.XYs, len(snapshot.Samples))
		for i, s := range snapshot.Samples {
			pts[i].X = s.Timestamp.Sub(start).Seconds()
			pts[i].Y = extractor(s)
		}

		p := plot.New()
		if label, ok := metricLabels[metric]; ok {
			p.Title.Text = label
			p.Y.Label.Text = label
		} else {
			p.Title.Text = metric
			p.Y.Label.Text = metric
		}
		p.X.Label.Text = "Seconds"

		line, err := plotter.NewLine(pts)
		if err != nil {
			return fmt.Errorf("line plot: %w", err)
		}
		line.LineStyle.Width = vg.Points(1.5)
		line.LineStyle.Color = color.RGBA{R: 31, G: 119, B: 180, A: 255}
		p.Add(line)

		if stats, ok := baseline.Metrics[metric]; ok {
			upper, lower := thresholds(stats, sensitivity)
			addThresholdLine(p, upper, color.RGBA{R: 214, G: 39, B: 40, A: 180}, "upper")
			addThresholdLine(p, lower, color.RGBA{R: 44, G: 160, B: 44, A: 180}, "lower")
		}

		if anns := anomaliesByMetric[metric]; len(anns) > 0 {
			scatterPts := make(plotter.XYs, len(anns))
			for i, a := range anns {
				scatterPts[i].X = a.Timestamp.Sub(start).Seconds()
				scatterPts[i].Y = a.Value
			}
			scatter, err := plotter.NewScatter(scatterPts)
			if err != nil {
				return fmt.Errorf("scatter plot: %w", err)
			}
			scatter.GlyphStyle.Color = color.RGBA{R: 214, G: 39, B: 40, A: 255}
			scatter.GlyphStyle.Radius = vg.Points(3)
			p.Add(scatter)
		}

		filename := fmt.Sprintf("%s.png", strings.ReplaceAll(metric, "_", "-"))
		if err := p.Save(6*vg.Inch, 3.5*vg.Inch, filepath.Join(outDir, filename)); err != nil {
			return fmt.Errorf("save plot for %s: %w", metric, err)
		}
	}
	return nil
}

func addThresholdLine(p *plot.Plot, value float64, c color.Color, label string) {
	line := plotter.NewFunction(func(x float64) float64 { return value })
	line.Color = c
	line.Width = vg.Points(0.8)
	line.Dashes = []vg.Length{vg.Points(4), vg.Points(4)}
	p.Add(line)
	if label != "" {
		p.Legend.Add(label, line)
	}
}
