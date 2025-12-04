package metrics

func getDashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Black-Box Metrics Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: #333;
            padding: 20px;
            min-height: 100vh;
        }

        .container {
            max-width: 1400px;
            margin: 0 auto;
        }

        header {
            text-align: center;
            color: white;
            margin-bottom: 30px;
        }

        h1 {
            font-size: 2.5em;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }

        .subtitle {
            font-size: 1.1em;
            opacity: 0.9;
        }

        .controls {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            margin-bottom: 20px;
        }

        .controls h2 {
            margin-bottom: 15px;
            color: #667eea;
        }

        .button-group {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }

        button {
            padding: 12px 24px;
            border: none;
            border-radius: 6px;
            font-size: 14px;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        .btn-danger {
            background: #e74c3c;
            color: white;
        }

        .btn-danger:hover {
            background: #c0392b;
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(231, 76, 60, 0.3);
        }

        .btn-warning {
            background: #f39c12;
            color: white;
        }

        .btn-warning:hover {
            background: #d68910;
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(243, 156, 18, 0.3);
        }

        .btn-info {
            background: #3498db;
            color: white;
        }

        .btn-info:hover {
            background: #2980b9;
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(52, 152, 219, 0.3);
        }

        .btn-success {
            background: #27ae60;
            color: white;
        }

        .btn-success:hover {
            background: #229954;
            transform: translateY(-2px);
            box-shadow: 0 4px 8px rgba(39, 174, 96, 0.3);
        }

        .status {
            background: white;
            padding: 15px;
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            margin-bottom: 20px;
            display: flex;
            justify-content: space-between;
            align-items: center;
        }

        .status-item {
            text-align: center;
        }

        .status-label {
            font-size: 0.9em;
            color: #666;
            margin-bottom: 5px;
        }

        .status-value {
            font-size: 1.5em;
            font-weight: bold;
            color: #667eea;
        }

        .fault-status {
            padding: 6px 16px;
            border-radius: 20px;
            font-weight: 500;
        }

        .fault-active {
            background: #e74c3c;
            color: white;
        }

        .fault-none {
            background: #27ae60;
            color: white;
        }

        .charts {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
            gap: 20px;
            margin-bottom: 20px;
        }

        .chart-container {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }

        .chart-container h3 {
            margin-bottom: 15px;
            color: #667eea;
            text-align: center;
        }

        canvas {
            max-height: 300px;
        }

        .anomaly-detector {
            background: white;
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
        }

        .anomaly-detector h2 {
            margin-bottom: 15px;
            color: #667eea;
        }

        .anomaly-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
        }

        .anomaly-card {
            padding: 15px;
            border-radius: 8px;
            border: 2px solid #ecf0f1;
        }

        .anomaly-card.normal {
            border-color: #27ae60;
            background: #d5f4e6;
        }

        .anomaly-card.warning {
            border-color: #f39c12;
            background: #fdebd0;
        }

        .anomaly-card.critical {
            border-color: #e74c3c;
            background: #fadbd8;
        }

        .anomaly-title {
            font-weight: 600;
            margin-bottom: 10px;
        }

        .anomaly-value {
            font-size: 1.3em;
            font-weight: bold;
            margin-bottom: 5px;
        }

        .anomaly-baseline {
            font-size: 0.9em;
            color: #666;
        }

        @media (max-width: 768px) {
            .charts {
                grid-template-columns: 1fr;
            }

            .button-group {
                flex-direction: column;
            }

            button {
                width: 100%;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>Black-Box Failure Diagnosis Dashboard</h1>
        </header>

        <div class="controls">
            <h2>Fault Injection Controls</h2>
            <div class="button-group">
                <button class="btn-danger" onclick="injectFault('memory')">Inject Memory Leak</button>
                <button class="btn-warning" onclick="injectFault('cpu')">Inject CPU Spike</button>
                <button class="btn-info" onclick="injectFault('goroutine')">Inject Goroutine Leak</button>
                <button class="btn-success" onclick="stopFault()">Stop Fault Injection</button>
            </div>
        </div>

        <div class="status">
            <div class="status-item">
                <div class="status-label">Active Fault</div>
                <div class="status-value">
                    <span id="fault-status" class="fault-status fault-none">NoFault</span>
                </div>
            </div>
            <div class="status-item">
                <div class="status-label">Metrics Collected</div>
                <div class="status-value" id="metrics-count">0</div>
            </div>
            <div class="status-item">
                <div class="status-label">Last Update</div>
                <div class="status-value" id="last-update" style="font-size: 1em;">Never</div>
            </div>
        </div>

        <div class="charts">
            <div class="chart-container">
                <h3>CPU Usage (%)</h3>
                <canvas id="cpuChart"></canvas>
            </div>
            <div class="chart-container">
                <h3>Memory Usage (MB)</h3>
                <canvas id="memoryChart"></canvas>
            </div>
            <div class="chart-container">
                <h3>Goroutine Count</h3>
                <canvas id="goroutineChart"></canvas>
            </div>
        </div>

        <div class="anomaly-detector">
            <h2>Anomaly Detection</h2>
            <div class="anomaly-grid">
                <div class="anomaly-card normal" id="cpu-anomaly">
                    <div class="anomaly-title">CPU Usage</div>
                    <div class="anomaly-value">--</div>
                    <div class="anomaly-baseline">Baseline: --</div>
                </div>
                <div class="anomaly-card normal" id="memory-anomaly">
                    <div class="anomaly-title">Memory Usage</div>
                    <div class="anomaly-value">--</div>
                    <div class="anomaly-baseline">Baseline: --</div>
                </div>
                <div class="anomaly-card normal" id="goroutine-anomaly">
                    <div class="anomaly-title">Goroutines</div>
                    <div class="anomaly-value">--</div>
                    <div class="anomaly-baseline">Baseline: --</div>
                </div>
            </div>
        </div>
    </div>

    <script>
        const MAX_POINTS = 100;
        let baseline = null;

        // Initialize charts
        const cpuChart = createChart('cpuChart', 'CPU Usage (%)', 'rgba(231, 76, 60, 0.8)', 'rgba(231, 76, 60, 0.2)');
        const memoryChart = createChart('memoryChart', 'Memory (MB)', 'rgba(52, 152, 219, 0.8)', 'rgba(52, 152, 219, 0.2)');
        const goroutineChart = createChart('goroutineChart', 'Goroutines', 'rgba(155, 89, 182, 0.8)', 'rgba(155, 89, 182, 0.2)');

        function createChart(canvasId, label, borderColor, backgroundColor) {
            const ctx = document.getElementById(canvasId).getContext('2d');
            return new Chart(ctx, {
                type: 'line',
                data: {
                    labels: [],
                    datasets: [{
                        label: label,
                        data: [],
                        borderColor: borderColor,
                        backgroundColor: backgroundColor,
                        borderWidth: 2,
                        tension: 0.4,
                        fill: true,
                        pointRadius: 2,
                        pointHoverRadius: 5
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: true,
                    plugins: {
                        legend: {
                            display: false
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true
                        }
                    },
                    animation: {
                        duration: 300
                    }
                }
            });
        }

        function updateChart(chart, labels, data) {
            chart.data.labels = labels;
            chart.data.datasets[0].data = data;
            chart.update('none');
        }

        async function fetchMetrics() {
            try {
                const response = await fetch('/metrics/latest?n=' + MAX_POINTS);
                const metrics = await response.json();

                if (metrics.length === 0) return;

                const labels = metrics.map(m => new Date(m.timestamp).toLocaleTimeString());
                const cpuData = metrics.map(m => m.cpu_usage_percent.toFixed(2));
                const memData = metrics.map(m => m.memory_alloc_mb.toFixed(2));
                const goroutineData = metrics.map(m => m.goroutine_count);

                updateChart(cpuChart, labels, cpuData);
                updateChart(memoryChart, labels, memData);
                updateChart(goroutineChart, labels, goroutineData);

                document.getElementById('metrics-count').textContent = metrics.length;
                document.getElementById('last-update').textContent = new Date().toLocaleTimeString();

                // Get latest metric for anomaly detection
                const latest = metrics[metrics.length - 1];
                await fetchStatistics(latest);
            } catch (error) {
                console.error('Error fetching metrics:', error);
            }
        }

        async function fetchStatistics(latest) {
            try {
                const response = await fetch('/metrics/stats');
                const stats = await response.json();

                // Always update baseline with latest statistics (dynamic sliding window)
                if (stats.cpu_mean > 0) {
                    baseline = stats;
                }

                if (baseline) {
                    detectAnomaly('cpu', latest.cpu_usage_percent, baseline.cpu_mean, baseline.cpu_stddev);
                    detectAnomaly('memory', latest.memory_alloc_mb, baseline.mem_mean, baseline.mem_stddev);
                    detectAnomaly('goroutine', latest.goroutine_count, baseline.goroutine_mean, baseline.goroutine_stddev);
                }
            } catch (error) {
                console.error('Error fetching statistics:', error);
            }
        }

        function detectAnomaly(metric, current, mean, stddev) {
            const card = document.getElementById(metric + '-anomaly');
            const deviation = Math.abs(current - mean) / (stddev || 1);

            card.querySelector('.anomaly-value').textContent = current.toFixed(2);
            card.querySelector('.anomaly-baseline').textContent =
                'Baseline: ' + mean.toFixed(2) + ' ± ' + stddev.toFixed(2);

            card.classList.remove('normal', 'warning', 'critical');

            if (deviation > 3) {
                card.classList.add('critical');
            } else if (deviation > 2) {
                card.classList.add('warning');
            } else {
                card.classList.add('normal');
            }
        }

        async function fetchFaultStatus() {
            try {
                const response = await fetch('/faults/status');
                const data = await response.json();

                const statusEl = document.getElementById('fault-status');
                statusEl.textContent = data.status;

                if (data.status === 'NoFault') {
                    statusEl.className = 'fault-status fault-none';
                } else {
                    statusEl.className = 'fault-status fault-active';
                }
            } catch (error) {
                console.error('Error fetching fault status:', error);
            }
        }

        async function injectFault(type) {
            try {
                const response = await fetch('/faults/inject?type=' + type, { method: 'POST' });
                const data = await response.json();
                alert('Fault injected: ' + data.fault);
                // No need to reset baseline - it will update automatically via sliding window
                fetchFaultStatus();
            } catch (error) {
                console.error('Error injecting fault:', error);
                alert('Failed to inject fault');
            }
        }

        async function stopFault() {
            try {
                const response = await fetch('/faults/stop', { method: 'POST' });
                const data = await response.json();
                alert(data.message);
                // No need to reset baseline - it will update automatically via sliding window
                fetchFaultStatus();
            } catch (error) {
                console.error('Error stopping fault:', error);
                alert('Failed to stop fault');
            }
        }

        // Update metrics every 1 second
        setInterval(() => {
            fetchMetrics();
            fetchFaultStatus();
        }, 1000);

        // Initial fetch
        fetchMetrics();
        fetchFaultStatus();
    </script>
</body>
</html>`
}
