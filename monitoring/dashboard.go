package monitoring

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// Dashboard provides an HTTP server for monitoring and control
type Dashboard struct {
	collector *MetricsCollector
	detector  *AnomalyDetector
	injector  *FaultInjector
	addr      string
}

// NewDashboard creates a new monitoring dashboard
func NewDashboard(addr string, collector *MetricsCollector, detector *AnomalyDetector, injector *FaultInjector) *Dashboard {
	return &Dashboard{
		collector: collector,
		detector:  detector,
		injector:  injector,
		addr:      addr,
	}
}

// Start starts the HTTP server
func (d *Dashboard) Start() error {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/metrics", d.handleMetrics)
	mux.HandleFunc("/api/metrics/latest", d.handleLatestMetrics)
	mux.HandleFunc("/api/baseline", d.handleBaseline)
	mux.HandleFunc("/api/anomalies", d.handleAnomalies)
	mux.HandleFunc("/api/faults", d.handleFaults)
	mux.HandleFunc("/api/faults/inject", d.handleInjectFault)
	mux.HandleFunc("/api/faults/stop", d.handleStopFault)
	mux.HandleFunc("/api/diagnosis", d.handleDiagnosis)

	// Dashboard UI
	mux.HandleFunc("/", d.handleDashboardUI)

	log.Printf("[DASHBOARD] Starting monitoring dashboard on http://%s", d.addr)
	log.Printf("[DASHBOARD] Open http://%s in your browser to view metrics", d.addr)

	server := &http.Server{
		Addr:    d.addr,
		Handler: d.corsMiddleware(mux),
	}

	return server.ListenAndServe()
}

func (d *Dashboard) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleMetrics returns all collected metrics
func (d *Dashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := d.collector.GetAllMetrics()
	d.writeJSON(w, metrics)
}

// handleLatestMetrics returns the most recent metrics
func (d *Dashboard) handleLatestMetrics(w http.ResponseWriter, r *http.Request) {
	latest := d.collector.GetLatestMetrics()
	if latest == nil {
		http.Error(w, "No metrics available", http.StatusNotFound)
		return
	}
	d.writeJSON(w, latest)
}

// handleBaseline returns or establishes the baseline
func (d *Dashboard) handleBaseline(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// Establish new baseline
		metrics := d.collector.GetAllMetrics()
		if len(metrics) < 10 {
			http.Error(w, "Not enough metrics to establish baseline (need at least 10)", http.StatusBadRequest)
			return
		}
		if err := d.detector.EstablishBaseline(metrics); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		baseline := d.detector.GetBaseline()
		d.writeJSON(w, baseline)
	} else {
		// Get current baseline
		baseline := d.detector.GetBaseline()
		if baseline == nil {
			http.Error(w, "No baseline established", http.StatusNotFound)
			return
		}
		d.writeJSON(w, baseline)
	}
}

// handleAnomalies returns detected anomalies
func (d *Dashboard) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	durationStr := r.URL.Query().Get("duration")
	if durationStr != "" {
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			http.Error(w, "Invalid duration format", http.StatusBadRequest)
			return
		}
		anomalies := d.detector.GetRecentAnomalies(duration)
		d.writeJSON(w, anomalies)
	} else {
		anomalies := d.detector.GetAnomalies()
		d.writeJSON(w, anomalies)
	}
}

// handleFaults returns active faults
func (d *Dashboard) handleFaults(w http.ResponseWriter, r *http.Request) {
	faults := d.injector.GetActiveFaults()
	d.writeJSON(w, map[string]interface{}{
		"active_faults": faults,
	})
}

// handleInjectFault injects a new fault
func (d *Dashboard) handleInjectFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	faultTypeStr := r.URL.Query().Get("type")
	intensityStr := r.URL.Query().Get("intensity")

	if faultTypeStr == "" {
		http.Error(w, "Missing 'type' parameter", http.StatusBadRequest)
		return
	}

	intensity := 5 // default
	if intensityStr != "" {
		var err error
		intensity, err = strconv.Atoi(intensityStr)
		if err != nil {
			http.Error(w, "Invalid intensity value", http.StatusBadRequest)
			return
		}
	}

	faultType := FaultType(faultTypeStr)
	if err := d.injector.InjectFault(faultType, intensity); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.writeJSON(w, map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Injected %s fault with intensity %d", faultType, intensity),
	})
}

// handleStopFault stops a fault
func (d *Dashboard) handleStopFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	faultTypeStr := r.URL.Query().Get("type")
	if faultTypeStr == "" {
		// Stop all faults
		d.injector.StopAllFaults()
		d.writeJSON(w, map[string]string{
			"status":  "success",
			"message": "Stopped all faults",
		})
		return
	}

	faultType := FaultType(faultTypeStr)
	if err := d.injector.StopFault(faultType); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	d.writeJSON(w, map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Stopped %s fault", faultType),
	})
}

// handleDiagnosis provides root cause diagnosis
func (d *Dashboard) handleDiagnosis(w http.ResponseWriter, r *http.Request) {
	durationStr := r.URL.Query().Get("duration")
	duration := 5 * time.Minute // default

	if durationStr != "" {
		var err error
		duration, err = time.ParseDuration(durationStr)
		if err != nil {
			http.Error(w, "Invalid duration format", http.StatusBadRequest)
			return
		}
	}

	anomalies := d.detector.GetRecentAnomalies(duration)
	diagnosis := d.detector.DiagnoseRootCause(anomalies)

	d.writeJSON(w, map[string]interface{}{
		"diagnosis":       diagnosis,
		"anomaly_count":   len(anomalies),
		"analysis_period": duration.String(),
	})
}

func (d *Dashboard) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[DASHBOARD] Error encoding JSON: %v", err)
	}
}

// handleDashboardUI serves the HTML dashboard
func (d *Dashboard) handleDashboardUI(w http.ResponseWriter, r *http.Request) {
	// Serve the static HTML file
	http.ServeFile(w, r, "monitoring/dashboard.html")
}
