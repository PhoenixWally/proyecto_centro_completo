// ═══════════════════════════════════════════════════════
// sentinel.js  –  Visor ESMB en Tiempo Real
// ═══════════════════════════════════════════════════════

let sseSource   = null;
let isScanning  = false;
let zHistory    = [];   // Historial de trazas para el 3D
let freqAxis    = [];   // Eje X de frecuencias (se fija al iniciar el scan)
const MAX_Z_ROWS = 30;  // Filas de historial en 3D
const MAX_LOG    = 50;  // Máx entradas en log
let lastLogTime  = 0;   // Para no saturar el log con medidas

function addLog(message, type = 'system') {
    const logBox = document.getElementById('esmbLog');
    if (!logBox) return;
    
    const entry = document.createElement('div');
    const now = new Date();
    const timeStr = now.getHours().toString().padStart(2, '0') + ':' + 
                  now.getMinutes().toString().padStart(2, '0') + ':' + 
                  now.getSeconds().toString().padStart(2, '0');
    
    entry.className = `log-row ${type}`;
    entry.innerHTML = `<span style="opacity:0.5">[${timeStr}]</span> ${message}`;
    
    logBox.appendChild(entry);
    logBox.scrollTop = logBox.scrollHeight;
    
    if (logBox.children.length > MAX_LOG) {
        logBox.removeChild(logBox.firstChild);
    }
    
    const emptyMsg = logBox.querySelector('.log-empty');
    if (emptyMsg) emptyMsg.remove();
}

// ─── Cambio de estación ───────────────────────────────────
function onStationChange() {
    zHistory = [];
    freqAxis = [];
    // Limpiar gráficas
    if (window.Plotly) {
        Plotly.react('chart3D', [{ z: [[0,0],[0,0]], type: 'surface' }], { title: 'Cargando...' });
        Plotly.react('chart2D', [{ x: [], y: [] }], { title: 'Cargando...' });
    }
    // Reiniciar polling para la nueva estación
    startSSE();
}

// ─── Start / Stop ─────────────────────────────────────────
async function startScan() {
    const ip = document.getElementById('viewerStationSelect').value;
    if (!ip) { alert('Selecciona una estación primero.'); return; }

    const freqStart = parseFloat(document.getElementById('freqStart').value);
    const freqEnd   = parseFloat(document.getElementById('freqEnd').value);
    const stepKhz   = parseFloat(document.getElementById('stepKhz').value) || 100;
    if (freqStart >= freqEnd) { alert('Frecuencia de inicio debe ser menor que la de fin.'); return; }

    const res = await fetch('/api/esmb/scan/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ip_esmb: ip, freq_start: freqStart, freq_end: freqEnd, step_khz: stepKhz })
    });

    const json = await res.json();
    if (!json.success) {
        addLog('Error: ' + (json.error || 'desconocido'), 'error');
        alert('Error al iniciar: ' + (json.error || 'desconocido'));
        return;
    }

    isScanning  = true;
    zHistory    = [];
    freqAxis    = [];
    document.getElementById('btnStart').style.display = 'none';
    document.getElementById('btnStop').style.display  = 'block';
    setStatus(true, 'Conectando...');
    addLog(`Monitor Iniciado: ${ip} (${freqStart}-${freqEnd} MHz)`, 'system');
    initCharts(freqStart, freqEnd);
    startSSE();
}

async function stopScan() {
    const ip = document.getElementById('viewerStationSelect').value;
    if (!ip) return;
    isScanning = false;
    await fetch('/api/esmb/scan/stop', { 
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ ip_esmb: ip })
    });
    closeSEE();
    document.getElementById('btnStart').style.display = 'block';
    document.getElementById('btnStop').style.display  = 'none';
    setStatus(false, 'Detenido');
    addLog('Monitor detenido', 'system');
}

// ─── AJAX Polling – datos en tiempo real ────────────────────
let pollInterval = null;

function startSSE() {
    if (pollInterval) clearInterval(pollInterval);
    pollInterval = setInterval(fetchData, 100); // 100ms polling rigido
}

function closeSEE() {
    if (pollInterval) { clearInterval(pollInterval); pollInterval = null; }
}

async function fetchData() {
    const ip = document.getElementById('viewerStationSelect').value;
    if (!ip) return;

    try {
        const res = await fetch(`/api/esmb/data?ip=${ip}`);
        const payload = await res.json();
        
        const btnStart = document.getElementById('btnStart');
        const btnStop = document.getElementById('btnStop');

        if (payload.is_running) {
            isScanning = true;
            // Si el dueño no soy yo, ocultar controles
            if (payload.owner !== payload.current_user) {
                btnStart.style.display = 'none';
                btnStop.style.display  = 'none';
                setStatus(true, `Escaneando (Controlado por ${payload.owner})`);
            } else {
                btnStart.style.display = 'none';
                btnStop.style.display  = 'block';
                setStatus(true, 'Escaneando (Controlado por TI)');
            }

            if (payload.trace && payload.trace.frequencies.length > 0) {
                processNewData(payload.trace.frequencies, payload.trace.levels);
            }
        } else {
            isScanning = false;
            btnStart.style.display = 'block';
            btnStop.style.display  = 'none';
            setStatus(false, 'Estación Libre');
        }
    } catch (err) {
        console.error("Error polling:", err);
    }
}

// ─── Procesar datos reales ────────────────────────────────
function processNewData(xRow, yRow) {
    const threshold = parseFloat(document.getElementById('peakThreshold').value);

    // Añadir al log visual
    const logEl = document.getElementById('esmbLog');
    
    // Solo logueamos picos que pasen el threshold para no saturar la web
    const ts = new Date().toLocaleTimeString();
    let picos = [];
    for (let i = 0; i < xRow.length; i++) {
        if (yRow[i] >= threshold) {
            picos.push({ f: xRow[i], db: yRow[i] });
        }
    }
    
    // Ordenar picos de mayor a menor y logear el más alto
    if (picos.length > 0) {
        picos.sort((a,b) => b.db - a.db);
        const top = picos[0];
        addLog(`${top.f.toFixed(4)} MHz | ${top.db.toFixed(1)} dBm (Pico Máx)`, 'log-peak');
    }

    // Log de medida general cada ~1 segundo
    const nowTs = Date.now();
    if (nowTs - lastLogTime > 1000) {
        const avg = yRow.reduce((a, b) => a + b, 0) / yRow.length;
        addLog(`Recibida traza: ${yRow.length} pts (Avg: ${avg.toFixed(2)} dBuV)`, 'measure');
        lastLogTime = nowTs;
    }

    if (!freqAxis.length) freqAxis = xRow;

    const now = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
    const stationName = document.getElementById('viewerStationSelect').selectedOptions[0]?.text || 'Scan';
    const filename = `${stationName}_${now}`;

    // 2D: actualizar traza
    Plotly.react('chart2D', [
        {
            x: xRow, y: yRow,
            type: 'scatter', mode: 'lines', name: 'Espectro',
            line: { color: '#10b981', width: 1.5 },
            fill: 'tozeroy', fillcolor: 'rgba(16,185,129,0.12)'
        },
        {
            x: picos.map(p => p.f), y: picos.map(p => p.db),
            type: 'scatter', mode: 'markers', name: `Picos > ${threshold} dBm`,
            marker: { color: '#f59e0b', size: 8, symbol: 'triangle-up' }
        }
    ], {
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor:  'rgba(0,0,0,0)',
        font: { color: '#e2e8f0', family: 'Inter' },
        margin: { l: 60, r: 20, b: 80, t: 35 },
        xaxis: { title: 'Frecuencia (MHz)', color: '#94a3b8', gridcolor: 'rgba(255,255,255,0.05)' },
        yaxis: { title: 'dBm', color: '#94a3b8', gridcolor: 'rgba(255,255,255,0.05)' },
        legend: { font: { color: '#e2e8f0' } }
    }, { 
        responsive: true, 
        displayModeBar: true,
        toImageButtonOptions: { format: 'png', filename: filename + '_2D' }
    });


    // 3D: añadir fila al historial
    zHistory.push(yRow);
    if (zHistory.length > MAX_Z_ROWS) zHistory.shift();

    // Actualizar 3D siempre (ya no hay pestañas)
    // Recuperar la posición actual de la cámara para que no salte
    const plot3DDiv = document.getElementById("chart3D");
    let currentCamera = { eye: { x: 1.2, y: -1.2, z: 0.4 }, projection: { type: 'orthographic' } };
    if (plot3DDiv && plot3DDiv.layout && plot3DDiv.layout.scene && plot3DDiv.layout.scene.camera) {
        currentCamera = plot3DDiv.layout.scene.camera;
    }

    const trace3D = {
        z: renderZ_3d = (zHistory.length === 1 ? [zHistory[0], zHistory[0]] : zHistory),
        x: xRow,
        type: 'surface',
        colorscale: 'Jet',
        cmin: -10,
        cmax: 80,
        showscale: false
    };

    const layout3D = {
        title: false,
        autosize: true,
        margin: { l: 0, r: 0, b: 0, t: 0 },
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor: 'rgba(0,0,0,0)',
        font: { color: '#94a3b8', family: 'Inter' },
        scene: {
            camera: currentCamera,
            xaxis: { title: 'MHz', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155' },
            yaxis: { title: 'Historia', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155', showticklabels: false },
            zaxis: { title: 'dBm', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, range: [-30, 80], color: "#94a3b8", gridcolor: '#334155' },
            aspectratio: { x: 1.5, y: 1.5, z: 0.6 }
        }
    };

    Plotly.react('chart3D', [trace3D], layout3D, { 
        displayModeBar: true, 
        responsive: true,
        toImageButtonOptions: { format: 'png', filename: filename + '_3D' }
    });
}

// ─── Helpers UI ───────────────────────────────────────────
function setStatus(on, msg) {
    document.getElementById('statusLed').className = 'led ' + (on ? 'led-on' : 'led-off');
    document.getElementById('statusText').textContent = msg;
}

function clearLog() {
    document.getElementById('esmbLog').innerHTML = '<p class="text-muted log-empty">Log limpiado.</p>';
}

// ─── Tabs ─────────────────────────────────────────────────
function switchTab(tab) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.getElementById('chart3D').style.display = tab === '3d' ? 'block' : 'none';
    document.getElementById('chart2D').style.display = tab === '2d' ? 'block' : 'none';
    document.getElementById(`tab${tab}`).classList.add('active');
    if (window.Plotly) {
        Plotly.Plots.resize(tab === '3d' ? 'chart3D' : 'chart2D');
    }
}

// ─── Init Plotly Charts ───────────────────────────────────
function initCharts(freqStart, freqEnd) {
    const baseLayout = {
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor:  'rgba(0,0,0,0)',
        font: { color: '#e2e8f0', family: 'Inter' },
        margin: { l: 50, r: 20, b: 50, t: 35 }
    };

    // ── 3D Surface ──────────────────────────
    const layout3D = {
        title: false,
        autosize: true,
        margin: { l: 0, r: 0, b: 0, t: 0 },
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor: 'rgba(0,0,0,0)',
        font: { color: '#94a3b8', family: 'Inter' },
        scene: {
            camera: {
                eye: { x: 1.2, y: -1.2, z: 0.4 },
                projection: { type: 'orthographic' }
            },
            xaxis: { title: 'MHz', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155' },
            yaxis: { title: 'Historia', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155', showticklabels: false },
            zaxis: { title: 'dBm', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, range: [-30, 80], color: "#94a3b8", gridcolor: '#334155' },
            aspectratio: { x: 1.5, y: 1.5, z: 0.6 }
        }
    };

    Plotly.newPlot('chart3D', [{
        type: 'surface',
        z: [[-100, -100], [-100, -100]],
        x: [freqStart, freqEnd],
        colorscale: 'Jet',
        cmin: -10,
        cmax: 80,
        showscale: false
    }], layout3D, { responsive: true, displayModeBar: true });

    // ── 2D Spectrum ──────────────────────────
    const threshold = parseFloat(document.getElementById('peakThreshold').value);
    Plotly.newPlot('chart2D', [
        {
            x: [freqStart, freqEnd], y: [-100, -100],
            type: 'scatter', mode: 'lines', name: 'Espectro',
            line: { color: '#10b981', width: 1.5 },
            fill: 'tozeroy', fillcolor: 'rgba(16,185,129,0.12)'
        },
        {
            x: [], y: [],
            type: 'scatter', mode: 'markers', name: `Picos > ${threshold} dBm`,
            marker: { color: '#f59e0b', size: 8, symbol: 'triangle-up' }
        }
    ], {
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor:  'rgba(0,0,0,0)',
        font: { color: '#e2e8f0', family: 'Inter' },
        margin: { l: 50, r: 20, b: 50, t: 35 },
        xaxis: { title: 'Frecuencia (MHz)', color: '#94a3b8', gridcolor: 'rgba(255,255,255,0.05)' },
        yaxis: { title: 'dBm', color: '#94a3b8', gridcolor: 'rgba(255,255,255,0.05)' },
        legend: { font: { color: '#e2e8f0' } }
    }, { responsive: true, displayModeBar: true });
}
