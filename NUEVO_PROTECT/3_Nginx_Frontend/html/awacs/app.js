// ============================================================================
//  Sentinel AWACS — Monitor de Datos Puro (v2 - FIX)
// ============================================================================

document.addEventListener('DOMContentLoaded', () => {
    const connectBtn = document.getElementById('connectBtn');
    const sourceSelect = document.getElementById('sourceSelect');
    const statusText = document.getElementById('statusText');

    // Ocultar gráfica
    const waterfall = document.getElementById('waterfallContainer');
    if (waterfall) waterfall.style.display = 'none';

    // Monitor mejorado
    const monitor = document.createElement('div');
    Object.assign(monitor.style, {
        position: 'fixed', top: '300px', left: '20px', right: '20px', bottom: '20px',
        backgroundColor: 'rgba(0,0,0,0.95)', color: '#0f0', fontFamily: 'monospace', fontSize: '12px',
        padding: '15px', overflowY: 'scroll', border: '2px solid #00ffcc', zIndex: '1000',
        lineHeight: '1.4em', whiteSpace: 'pre-wrap'
    });
    document.body.appendChild(monitor);

    function logData(msg, color = '#0f0') {
        const line = document.createElement('div');
        line.style.color = color;
        line.textContent = `[${new Date().toLocaleTimeString()}] ${msg}`;
        monitor.appendChild(line);
        if (monitor.childNodes.length > 300) monitor.removeChild(monitor.firstChild);
        monitor.scrollTop = monitor.scrollHeight;
    }

    logData("=== INICIANDO MODO MONITOR (v2) ===", "#00ffcc");

    async function loadStations() {
        logData("Cargando estaciones desde API...");
        try {
            // Usamos la misma lógica que el original para evitar problemas de CORS/Auth
            const resp = await fetch('/api/v1/stations', { credentials: 'include' });
            if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
            const stations = await resp.json();
            
            sourceSelect.innerHTML = '';
            stations.forEach(st => {
                const opt = document.createElement('option');
                opt.value = st.id;
                opt.textContent = st.name;
                opt.dataset.physicalPath = st.physical_path;
                sourceSelect.appendChild(opt);
            });
            sourceSelect.disabled = false;
            connectBtn.disabled = false;
            logData(`OK: ${stations.length} estaciones listas.`);
        } catch (e) { 
            logData(`ERROR cargando estaciones: ${e.message}`, "red");
            // Forzar habilitación para debug si falla
            sourceSelect.disabled = false;
            connectBtn.disabled = false;
        }
    }
    loadStations();

    let ws = null;
    connectBtn.addEventListener('click', () => {
        if (ws) { ws.close(); return; }
        const selected = sourceSelect.options[sourceSelect.selectedIndex];
        if (!selected) { logData("Error: Selecciona una estación primero", "red"); return; }

        // Detectar IP y protocolo automáticamente para pasar a través de Nginx (Puerto 80)
        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsPort = window.location.port ? `:${window.location.port}` : '';
        const wsUrl = `${wsProto}//${window.location.hostname}${wsPort}/ws/`;
        logData(`Conectando a ${wsUrl} ...`, "yellow");
        
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            logData("WebSocket Abierto. Autenticando...");
            const token = document.cookie.match(/jwt=([^;]+)/)?.[1] || 'anonymous';
            ws.send(JSON.stringify({ action: 'auth', token }));
            setTimeout(() => {
                ws.send(JSON.stringify({ action: 'subscribe', source_path: selected.dataset.physicalPath }));
                logData(`Suscripción enviada: ${selected.dataset.physicalPath}`);
            }, 200);
        };

        ws.onmessage = (e) => {
            const msg = JSON.parse(e.data);
            if (msg.type === 'delta_frame' && msg.sweep) {
                const count = msg.sweep.length;
                const fmin = window._cal_fmin || 0;
                const fmax = window._cal_fmax || 0;
                const step = (count > 1) ? (fmax - fmin) / (count - 1) : 0;
                
                logData(`DATA -> ${count} bins recibidos.`, "#39ff14");
                msg.sweep.forEach((lv, i) => {
                    const freq = (fmin + i * step) / 1e6;
                    logData(`  [Bin ${i}] ${freq.toFixed(3)} MHz  -->  ${lv.toFixed(1)} dB`, "#39ff14");
                });
            } else if (msg.type === 'init_frame') {
                window._cal_fmin = msg.fmin;
                window._cal_fmax = msg.fmax;
                logData(`INIT -> Fmin: ${(msg.fmin/1e6).toFixed(2)} MHz, Fmax: ${(msg.fmax/1e6).toFixed(2)} MHz`, "#00f3ff");
            } else {
                logData(`MSG: ${msg.type} ${msg.event || ''}`, "#888");
            }
        };

        ws.onclose = () => {
            logData("Conexión cerrada.", "yellow");
            ws = null;
        };
        ws.onerror = (err) => logData(`Error WS: ${err.message}`, "red");
    });
});
