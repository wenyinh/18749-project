package metrics

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/wenyinh/18749-project/faults"
)

// Server provides HTTP endpoints for metrics and fault injection
type Server struct {
	collector *Collector
	injector  *faults.Injector
	port      int
}

// NewServer creates a new metrics HTTP server
func NewServer(collector *Collector, injector *faults.Injector, port int) *Server {
	return &Server{
		collector: collector,
		injector:  injector,
		port:      port,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Metrics endpoints
	mux.HandleFunc("/metrics", s.handleMetrics)
	mux.HandleFunc("/metrics/latest", s.handleLatestMetrics)
	mux.HandleFunc("/metrics/stats", s.handleStatistics)

	// Fault injection endpoints
	mux.HandleFunc("/faults/inject", s.handleInjectFault)
	mux.HandleFunc("/faults/stop", s.handleStopFault)
	mux.HandleFunc("/faults/status", s.handleFaultStatus)

	// Dashboard
	mux.HandleFunc("/", s.handleDashboard)

	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("[METRICS] Starting metrics server on http://localhost%s", addr)
	log.Printf("[METRICS] Dashboard available at http://localhost%s", addr)
	log.Printf("[METRICS] API endpoints:")
	log.Printf("[METRICS]   GET  /metrics - All collected metrics")
	log.Printf("[METRICS]   GET  /metrics/latest?n=100 - Latest N metrics")
	log.Printf("[METRICS]   GET  /metrics/stats - Statistical summary")
	log.Printf("[METRICS]   POST /faults/inject?type=memory - Inject fault")
	log.Printf("[METRICS]   POST /faults/stop - Stop fault injection")
	log.Printf("[METRICS]   GET  /faults/status - Current fault status")

	return http.ListenAndServe(addr, s.corsMiddleware(mux))
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
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
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.collector.GetMetrics()
	s.jsonResponse(w, metrics)
}

// handleLatestMetrics returns the latest N metrics
func (s *Server) handleLatestMetrics(w http.ResponseWriter, r *http.Request) {
	n := 100 // default
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if parsed, err := strconv.Atoi(nStr); err == nil {
			n = parsed
		}
	}

	metrics := s.collector.GetLatestMetrics(n)
	s.jsonResponse(w, metrics)
}

// handleStatistics returns statistical summary
func (s *Server) handleStatistics(w http.ResponseWriter, r *http.Request) {
	stats := s.collector.GetStatistics()
	s.jsonResponse(w, stats)
}

// handleInjectFault injects a fault
func (s *Server) handleInjectFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	faultTypeStr := r.URL.Query().Get("type")
	var faultType faults.FaultType

	switch faultTypeStr {
	case "memory":
		faultType = faults.MemoryLeak
	case "cpu":
		faultType = faults.CPUIntensive
	case "goroutine":
		faultType = faults.GoroutineLeak
	default:
		http.Error(w, "Invalid fault type. Use: memory, cpu, or goroutine", http.StatusBadRequest)
		return
	}

	s.injector.InjectFault(faultType)

	response := map[string]string{
		"status":  "injected",
		"fault":   faultType.String(),
		"message": fmt.Sprintf("Fault %s has been injected", faultType.String()),
	}
	s.jsonResponse(w, response)
}

// handleStopFault stops fault injection
func (s *Server) handleStopFault(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.injector.StopFault()

	response := map[string]string{
		"status":  "stopped",
		"message": "Fault injection has been stopped",
	}
	s.jsonResponse(w, response)
}

// handleFaultStatus returns current fault status
func (s *Server) handleFaultStatus(w http.ResponseWriter, r *http.Request) {
	activeFault := s.injector.GetActiveFault()

	response := map[string]string{
		"status": activeFault.String(),
	}
	s.jsonResponse(w, response)
}

// handleDashboard serves the visualization dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	html := getDashboardHTML()
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// jsonResponse writes a JSON response
func (s *Server) jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
