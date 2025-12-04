package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wenyinh/18749-project/monitor"
)

type uiServer struct {
	live *monitor.LiveMonitor
}

type thresholdBounds struct {
	Upper float64 `json:"upper"`
	Lower float64 `json:"lower"`
}

type baselineRequest struct {
	MinSamples  int     `json:"min_samples"`
	Sensitivity float64 `json:"sensitivity"`
}

type faultRequest struct {
	Type            string  `json:"type"`
	DelaySeconds    float64 `json:"delay_seconds"`
	DurationSeconds float64 `json:"duration_seconds"`
}

type statusResponse struct {
	Metadata     monitor.SnapshotMetadata `json:"metadata"`
	HistoryLimit int                      `json:"history_limit"`
	Sensitivity  float64                  `json:"sensitivity"`
	Baseline     *monitor.BaselineSpec    `json:"baseline,omitempty"`
	Fault        monitor.FaultStatus      `json:"fault"`
}

type metricsResponse struct {
	Metadata    monitor.SnapshotMetadata   `json:"metadata"`
	Samples     []monitor.MetricSample     `json:"samples"`
	Baseline    *monitor.BaselineSpec      `json:"baseline,omitempty"`
	Thresholds  map[string]thresholdBounds `json:"thresholds,omitempty"`
	Anomalies   []monitor.Anomaly          `json:"anomalies,omitempty"`
	Sensitivity float64                    `json:"sensitivity"`
}

func newUIServer(live *monitor.LiveMonitor) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	s := &uiServer{live: live}

	router.GET("/", s.serveIndex)
	router.StaticFS("/static", embeddedHTTPFS())
	router.GET("/api/status", s.getStatus)
	router.GET("/api/metrics", s.getMetrics)
	router.POST("/api/baseline", s.createBaseline)
	router.DELETE("/api/baseline", s.deleteBaseline)
	router.POST("/api/fault", s.startFault)
	router.DELETE("/api/fault", s.stopFault)

	return router
}

func (s *uiServer) serveIndex(c *gin.Context) {
	data, err := embeddedWeb.ReadFile("web/index.html")
	if err != nil {
		c.String(http.StatusInternalServerError, "missing ui assets")
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", data)
}

func (s *uiServer) getStatus(c *gin.Context) {
	snap := s.live.HistorySnapshot()
	resp := statusResponse{
		Metadata:     snap.Metadata,
		HistoryLimit: s.live.HistoryLimit(),
		Sensitivity:  s.live.Sensitivity(),
		Fault:        s.live.FaultStatus(),
	}
	if baseline, ok := s.live.Baseline(); ok {
		resp.Baseline = &baseline
	}
	c.JSON(http.StatusOK, resp)
}

func (s *uiServer) getMetrics(c *gin.Context) {
	snap := s.live.HistorySnapshot()
	sensitivity := s.live.Sensitivity()
	resp := metricsResponse{
		Metadata:    snap.Metadata,
		Samples:     snap.Samples,
		Sensitivity: sensitivity,
	}
	if baseline, ok := s.live.Baseline(); ok {
		resp.Baseline = &baseline
		resp.Anomalies = monitor.DetectAnomalies(snap, baseline, sensitivity)
		resp.Thresholds = make(map[string]thresholdBounds, len(baseline.Metrics))
		for metric, stats := range baseline.Metrics {
			upper, lower := monitor.ThresholdBounds(stats, sensitivity)
			resp.Thresholds[metric] = thresholdBounds{Upper: upper, Lower: lower}
		}
	}
	c.JSON(http.StatusOK, resp)
}

func (s *uiServer) createBaseline(c *gin.Context) {
	var req baselineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Sensitivity > 0 {
		s.live.SetSensitivity(req.Sensitivity)
	}
	baseline, err := s.live.ComputeBaseline(req.MinSamples)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, baseline)
}

func (s *uiServer) deleteBaseline(c *gin.Context) {
	s.live.ClearBaseline()
	c.JSON(http.StatusOK, gin.H{"status": "cleared"})
}

func (s *uiServer) startFault(c *gin.Context) {
	var req faultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "fault type is required"})
		return
	}
	delay := time.Duration(req.DelaySeconds * float64(time.Second))
	duration := time.Duration(req.DurationSeconds * float64(time.Second))
	if err := s.live.StartFault(req.Type, delay, duration); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, s.live.FaultStatus())
}

func (s *uiServer) stopFault(c *gin.Context) {
	if stopped := s.live.StopFault(); !stopped {
		log.Printf("[monitor] stop fault request received but nothing was running")
	}
	c.JSON(http.StatusOK, s.live.FaultStatus())
}
