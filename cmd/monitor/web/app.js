const metrics = [
  { key: 'cpu_percent', label: 'CPU Utilization (%)', unit: '%', color: '#38bdf8' },
  { key: 'memory_percent', label: 'Memory Utilization (%)', unit: '%', color: '#a855f7' },
  { key: 'memory_used_mb', label: 'Memory Used (MB)', unit: 'MB', color: '#f97316' },
  { key: 'disk_used_percent', label: 'Disk Utilization (%)', unit: '%', color: '#22c55e' },
  { key: 'disk_used_gb', label: 'Disk Used (GB)', unit: 'GB', color: '#fb7185' },
];

const state = {
  samples: [],
  anomalies: [],
  thresholds: {},
  baseline: null,
  sensitivity: 3,
};

const chartState = new Map();
let toastTimer;

init();

function init() {
  setupCharts();
  setupForms();
  refreshStatus();
  refreshMetrics();
  setInterval(refreshStatus, 10000);
  setInterval(refreshMetrics, 3000);
}

function setupCharts() {
  const container = document.getElementById('chart-container');
  metrics.forEach((metric) => {
    const card = document.createElement('div');
    card.className = 'chart-card';
    card.dataset.metric = metric.key;
    card.innerHTML = `
      <div class="chart-card__header">
        <h3>${metric.label}</h3>
        <span class="chart-card__value" id="value-${metric.key}">--</span>
      </div>
      <canvas></canvas>
    `;
    container.appendChild(card);
    const canvas = card.querySelector('canvas');
    resizeCanvas(canvas);
    chartState.set(metric.key, {
      ...metric,
      canvas,
      valueEl: card.querySelector('.chart-card__value'),
    });
  });

  window.addEventListener('resize', () => {
    chartState.forEach(({ canvas }) => resizeCanvas(canvas));
    drawAllCharts();
  });
}

function resizeCanvas(canvas) {
  const scale = window.devicePixelRatio || 1;
  const width = canvas.clientWidth * scale;
  const height = canvas.clientHeight * scale;
  canvas.width = width || 600;
  canvas.height = height || 240 * scale;
}

function setupForms() {
  const baselineForm = document.getElementById('baseline-form');
  baselineForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const formData = new FormData(baselineForm);
    const payload = {
      min_samples: Number(formData.get('minSamples') || 0),
      sensitivity: Number(formData.get('sensitivity') || 0),
    };
    try {
      await fetchJSON('/api/baseline', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
      showToast('Baseline derived from current window', 'success');
      refreshStatus();
      refreshMetrics();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('clear-baseline').addEventListener('click', async () => {
    try {
      await fetchJSON('/api/baseline', { method: 'DELETE' });
      showToast('Baseline cleared', 'success');
      refreshStatus();
      refreshMetrics();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  const faultForm = document.getElementById('fault-form');
  faultForm.addEventListener('submit', async (event) => {
    event.preventDefault();
    const formData = new FormData(faultForm);
    const payload = {
      type: formData.get('type'),
      delay_seconds: Number(formData.get('delay') || 0),
      duration_seconds: Number(formData.get('duration') || 0),
    };
    try {
      await fetchJSON('/api/fault', {
        method: 'POST',
        body: JSON.stringify(payload),
      });
      showToast('Fault injection started', 'success');
      refreshStatus();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });

  document.getElementById('stop-fault').addEventListener('click', async () => {
    try {
      await fetchJSON('/api/fault', { method: 'DELETE' });
      showToast('Fault injector stopped', 'success');
      refreshStatus();
    } catch (err) {
      showToast(err.message, 'error');
    }
  });
}

async function refreshStatus() {
  try {
    const data = await fetchJSON('/api/status');
    document.getElementById('sampling-interval').textContent = `${data.metadata.interval_millis} ms`;
    document.getElementById('sample-count').textContent = data.metadata.sample_count;
    document.getElementById('disk-path').textContent = data.metadata.disk_path || '--';
    document.getElementById('history-limit').textContent = data.history_limit;
    document.getElementById('baseline-info').textContent = data.baseline
      ? `${data.baseline.sample_count} samples`
      : 'none';
    setFaultStatus(data.fault);
    updateBaselineTable(data.baseline);
  } catch (err) {
    showToast(`Status error: ${err.message}`, 'error');
  }
}

async function refreshMetrics() {
  try {
    const data = await fetchJSON('/api/metrics');
    state.samples = data.samples || [];
    state.anomalies = data.anomalies || [];
    state.thresholds = data.thresholds || {};
    state.baseline = data.baseline || null;
    state.sensitivity = data.sensitivity;
    document.getElementById('sample-count').textContent = data.metadata.sample_count;
    drawAllCharts();
    updateAnomalyTable(state.anomalies);
  } catch (err) {
    showToast(`Metrics error: ${err.message}`, 'error');
  }
}

function drawAllCharts() {
  const anomaliesByMetric = groupByMetric(state.anomalies);
  chartState.forEach(({ key, canvas, valueEl, color, unit }) => {
    const latestSample = state.samples[state.samples.length - 1];
    valueEl.textContent = latestSample && latestSample[key] !== undefined
      ? `${formatNumber(latestSample[key])}${unit ? ` ${unit}` : ''}`
      : '--';
    drawChart({
      canvas,
      metricKey: key,
      samples: state.samples,
      thresholds: state.thresholds[key],
      anomalies: anomaliesByMetric[key] || [],
      color,
    });
  });
}

function drawChart({ canvas, metricKey, samples, thresholds, anomalies, color }) {
  const ctx = canvas.getContext('2d');
  ctx.clearRect(0, 0, canvas.width, canvas.height);

  if (!samples.length) {
    ctx.fillStyle = 'rgba(255,255,255,0.5)';
    ctx.font = '14px Inter, system-ui';
    ctx.fillText('Waiting for samples...', 20, canvas.height / 2);
    return;
  }

  const padding = 28;
  const plotWidth = canvas.width - padding * 2;
  const plotHeight = canvas.height - padding * 2;

  const time0 = new Date(samples[0].timestamp).getTime() / 1000;
  const xs = samples.map((sample) => new Date(sample.timestamp).getTime() / 1000 - time0);
  const values = samples.map((sample) => Number(sample[metricKey] || 0));
  const xMin = xs[0];
  const xMax = xs[xs.length - 1] || xMin + 1;

  let yMin = Math.min(...values);
  let yMax = Math.max(...values);
  if (thresholds) {
    yMin = Math.min(yMin, thresholds.lower);
    yMax = Math.max(yMax, thresholds.upper);
  }
  if (yMin === yMax) {
    yMin -= 1;
    yMax += 1;
  }

  const scaleX = (v) => padding + ((v - xMin) / Math.max(1e-6, xMax - xMin)) * plotWidth;
  const scaleY = (v) => canvas.height - padding - ((v - yMin) / (yMax - yMin || 1)) * plotHeight;

  ctx.strokeStyle = 'rgba(255,255,255,0.08)';
  ctx.strokeRect(padding, padding, plotWidth, plotHeight);

  if (thresholds) {
    ctx.setLineDash([6, 6]);
    ctx.strokeStyle = 'rgba(244,63,94,0.7)';
    ctx.beginPath();
    ctx.moveTo(padding, scaleY(thresholds.upper));
    ctx.lineTo(canvas.width - padding, scaleY(thresholds.upper));
    ctx.stroke();
    ctx.strokeStyle = 'rgba(34,197,94,0.7)';
    ctx.beginPath();
    ctx.moveTo(padding, scaleY(thresholds.lower));
    ctx.lineTo(canvas.width - padding, scaleY(thresholds.lower));
    ctx.stroke();
    ctx.setLineDash([]);
  }

  ctx.beginPath();
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  xs.forEach((x, idx) => {
    const px = scaleX(x);
    const py = scaleY(values[idx]);
    if (idx === 0) {
      ctx.moveTo(px, py);
    } else {
      ctx.lineTo(px, py);
    }
  });
  ctx.stroke();

  if (anomalies?.length) {
    ctx.fillStyle = '#fbbf24';
    anomalies.forEach((anomaly) => {
      const ts = new Date(anomaly.timestamp).getTime() / 1000 - time0;
      const px = scaleX(ts);
      const py = scaleY(anomaly.value);
      ctx.beginPath();
      ctx.arc(px, py, 4, 0, Math.PI * 2);
      ctx.fill();
    });
  }
}

function groupByMetric(anomalies) {
  return anomalies.reduce((acc, anomaly) => {
    if (!acc[anomaly.metric]) {
      acc[anomaly.metric] = [];
    }
    acc[anomaly.metric].push(anomaly);
    return acc;
  }, {});
}

function updateBaselineTable(baseline) {
  const tbody = document.querySelector('#baseline-table tbody');
  tbody.innerHTML = '';
  if (!baseline) {
    const row = document.createElement('tr');
    row.innerHTML = '<td colspan="3" class="muted">Baseline not set</td>';
    tbody.appendChild(row);
    return;
  }

  const orderedKeys = metrics.map((m) => m.key);
  orderedKeys.forEach((metricKey) => {
    const stats = baseline.metrics?.[metricKey];
    if (!stats) {
      return;
    }
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${labelForMetric(metricKey)}</td>
      <td>${formatNumber(stats.mean)}</td>
      <td>${formatNumber(stats.std_dev)}</td>
    `;
    tbody.appendChild(row);
  });
}

function updateAnomalyTable(anomalies) {
  const tbody = document.querySelector('#anomaly-table tbody');
  tbody.innerHTML = '';
  if (!anomalies.length) {
    const row = document.createElement('tr');
    row.innerHTML = '<td colspan="5" class="muted">No anomalies detected</td>';
    tbody.appendChild(row);
    return;
  }

  const recent = anomalies.slice(-30).reverse();
  recent.forEach((anomaly) => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${labelForMetric(anomaly.metric)}</td>
      <td>${formatTime(anomaly.timestamp)}</td>
      <td>${formatNumber(anomaly.value)}</td>
      <td>${formatNumber(anomaly.threshold)}</td>
      <td>${formatNumber(anomaly.std_devs_away)}</td>
    `;
    tbody.appendChild(row);
  });
}

function setFaultStatus(fault) {
  const infoEl = document.getElementById('fault-info');
  const detailEl = document.getElementById('fault-status-text');
  if (!fault || (!fault.active && !fault.type)) {
    infoEl.textContent = 'idle';
    detailEl.textContent = 'No fault running';
    return;
  }
  if (fault.active) {
    infoEl.textContent = `${fault.type} running`;
    const started = fault.started_at ? formatTime(fault.started_at) : 'now';
    const duration = fault.requested_duration_seconds
      ? `${fault.requested_duration_seconds}s`
      : 'open-ended';
    detailEl.textContent = `Running ${fault.type} fault (delay ${fault.delay_seconds || 0}s, duration ${duration}) since ${started}`;
  } else {
    infoEl.textContent = 'idle';
    const completed = fault.completed_at ? formatTime(fault.completed_at) : 'recently';
    detailEl.textContent = `Last ${fault.type || 'fault'} finished ${completed}`;
  }
}

function labelForMetric(metricKey) {
  const match = metrics.find((m) => m.key === metricKey);
  return match ? match.label : metricKey;
}

function formatNumber(value) {
  if (value === null || value === undefined || Number.isNaN(Number(value))) {
    return '--';
  }
  const num = Number(value);
  if (Math.abs(num) >= 1000) {
    return num.toFixed(0);
  }
  if (Math.abs(num) >= 100) {
    return num.toFixed(1);
  }
  return num.toFixed(2);
}

function formatTime(value) {
  if (!value) return '--';
  const date = new Date(value);
  return `${date.toLocaleTimeString()} ${date.toLocaleDateString()}`;
}

async function fetchJSON(url, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (options.body && !headers['Content-Type']) {
    headers['Content-Type'] = 'application/json';
  }
  const opts = { ...options, headers };
  const response = await fetch(url, opts);
  let data = {};
  try {
    data = await response.json();
  } catch (err) {
    data = {};
  }
  if (!response.ok) {
    throw new Error(data.error || response.statusText);
  }
  return data;
}

function showToast(message, type = 'info') {
  const toast = document.getElementById('toast');
  toast.textContent = message;
  toast.classList.remove('hidden', 'success', 'error', 'visible');
  if (type === 'success') {
    toast.classList.add('success');
  } else if (type === 'error') {
    toast.classList.add('error');
  }
  requestAnimationFrame(() => toast.classList.add('visible'));
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    toast.classList.remove('visible');
  }, 3200);
}
