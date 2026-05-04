let pollingInterval = null;
let frames = 0;
let lastFpsTime = Date.now();
let lastLogTime = 0; // Para no saturar el log

function addLog(message, type = 'system') {
    const logBox = document.getElementById('log-box');
    if (!logBox) return;
    
    const entry = document.createElement('div');
    const now = new Date();
    const timeStr = now.getHours().toString().padStart(2, '0') + ':' + 
                  now.getMinutes().toString().padStart(2, '0') + ':' + 
                  now.getSeconds().toString().padStart(2, '0');
    
    entry.className = `log-entry ${type}`;
    entry.innerHTML = `<span style="opacity:0.5">[${timeStr}]</span> ${message}`;
    
    logBox.appendChild(entry);
    logBox.scrollTop = logBox.scrollHeight;
    
    if (logBox.children.length > 50) {
        logBox.removeChild(logBox.firstChild);
    }
}

// Buffer 3D (25 cuadros de historia)
const MAX_HISTORY = 25;
let history3D = [];

// Layouts de Plotly
const layout3D = {
    title: false,
    autosize: false, // Desactivar autosize para control total
    width: null, // Se ajustará al contenedor
    height: 500, // Altura fija idéntica al CSS
    margin: { l: 0, r: 0, b: 0, t: 0 },
    paper_bgcolor: 'rgba(0,0,0,0)',
    plot_bgcolor: 'rgba(0,0,0,0)',
    font: { color: '#94a3b8' },
    scene: {
        camera: {
            eye: { x: 1.2, y: -1.2, z: 0.4 },
            projection: { type: 'orthographic' }
        },
        xaxis: { title: 'MHz', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155' },
        yaxis: { title: 'Historia', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, color: "#94a3b8", gridcolor: '#334155', showticklabels: false },
        zaxis: { title: 'dBuV', backgroundcolor: "rgba(0,0,0,0.5)", showbackground: true, range: [-30, 80], color: "#94a3b8", gridcolor: '#334155' },
        aspectratio: { x: 1.5, y: 1.5, z: 0.6 }
    }
};

const layout2D = {
    title: false,
    autosize: false,
    width: null,
    height: 500,
    margin: { l: 40, r: 20, b: 30, t: 10 },
    paper_bgcolor: 'rgba(0,0,0,0)',
    plot_bgcolor: 'rgba(0,0,0,0)',
    font: { color: '#94a3b8' },
    xaxis: { title: 'Frecuencia (MHz)', gridcolor: '#334155', color: '#94a3b8' },
    yaxis: { title: 'Nivel (dBuV)', gridcolor: '#334155', range: [-30, 80], color: '#94a3b8' },
    showlegend: false
};

document.addEventListener('DOMContentLoaded', () => {
    // Inicializar Plotly con configuración estática
    const config = { displayModeBar: false, responsive: true };
    Plotly.newPlot('plot3D', [{ z: [[0, 0], [0, 0]], type: 'surface', showscale: false }], layout3D, config);
    Plotly.newPlot('plot2D', [{ x: [0], y: [0], type: 'scatter' }], layout2D, config);

    // Auto-resize
    window.addEventListener('resize', () => {
        Plotly.Plots.resize(document.getElementById('plot3D'));
        Plotly.Plots.resize(document.getElementById('plot2D'));
    });

    const form = document.getElementById('control-form');
    const btnStart = document.getElementById('btn-start');
    const btnStop = document.getElementById('btn-stop');
    const statusDot = document.getElementById('status-dot');
    const errorBox = document.getElementById('error-box');

    checkStatus();

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const ip = document.getElementById('ip_esmb').value;
        const freq_inicio = parseFloat(document.getElementById('freq_inicio').value);
        const freq_fin = parseFloat(document.getElementById('freq_fin').value);
        
        // Limpiar historia al arrancar
        history3D = [];
        
        try {
            const response = await fetch('/api/start', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ ip, freq_inicio, freq_fin })
            });
            
            const data = await response.json();
            if (data.success) {
                addLog(`Monitor Iniciado: ${ip} (${freq_inicio}-${freq_fin} MHz)`, 'system');
                setUIState(true);
                startPolling();
            } else {
                showError(data.error);
            }
        } catch (error) {
            showError('Error de conexión con el servidor web');
        }
    });

    btnStop.addEventListener('click', async () => {
        try {
            await fetch('/api/stop', { method: 'POST' });
            addLog('Monitor detenido por el usuario', 'system');
            setUIState(false);
            stopPolling();
        } catch (error) {
            console.error('Error stopping:', error);
        }
    });

    function setUIState(isRunning) {
        document.getElementById('ip_esmb').disabled = isRunning;
        document.getElementById('freq_inicio').disabled = isRunning;
        document.getElementById('freq_fin').disabled = isRunning;
        
        if (isRunning) {
            btnStart.classList.add('hidden');
            btnStop.classList.remove('hidden');
            statusDot.classList.add('active');
            errorBox.classList.add('hidden');
        } else {
            btnStart.classList.remove('hidden');
            btnStop.classList.add('hidden');
            statusDot.classList.remove('active');
            document.getElementById('fps-counter').innerText = 'FPS: 0';
        }
    }

    function showError(msg) {
        errorBox.textContent = msg;
        errorBox.classList.remove('hidden');
        setUIState(false);
        stopPolling();
    }

    async function checkStatus() {
        try {
            const res = await fetch('/api/data');
            const data = await res.json();
            if (data.is_running) {
                setUIState(true);
                startPolling();
            }
        } catch (e) { }
    }

    function startPolling() {
        if (pollingInterval) return;
        
        pollingInterval = setInterval(async () => {
            try {
                const res = await fetch('/api/data');
                const data = await res.json();
                
                if (data.error) {
                    showError(data.error);
                    return;
                }
                
                if (!data.is_running) {
                    setUIState(false);
                    stopPolling();
                    return;
                }
                
                if (data.trace && data.trace.frequencies.length > 0) {
                    updatePlots(data.trace.frequencies, data.trace.levels);
                    calculateFPS();

                    // Log de medida cada ~1 segundo para no saturar
                    const now = Date.now();
                    if (now - lastLogTime > 1000) {
                        const avg = data.trace.levels.reduce((a, b) => a + b, 0) / data.trace.levels.length;
                        addLog(`Recibida traza: ${data.trace.levels.length} pts (Avg: ${avg.toFixed(2)} dBuV)`, 'measure');
                        lastLogTime = now;
                    }
                }
                
            } catch (e) {
                console.error("Polling error", e);
            }
        }, 150); // Mismo refresco rápido
    }

    function stopPolling() {
        if (pollingInterval) {
            clearInterval(pollingInterval);
            pollingInterval = null;
        }
    }

    function calculateFPS() {
        frames++;
        const now = Date.now();
        if (now - lastFpsTime >= 1000) {
            document.getElementById('fps-counter').innerText = `FPS: ${frames}`;
            frames = 0;
            lastFpsTime = now;
        }
    }

    function updatePlots(x_data, y_data) {
        // Actualizar Gráfica 2D
        const trace2D = {
            x: x_data,
            y: y_data,
            type: 'scatter',
            mode: 'lines',
            line: { color: '#0ea5e9', width: 2 },
            fill: 'tozeroy',
            fillcolor: 'rgba(14, 165, 233, 0.1)'
        };
        Plotly.react('plot2D', [trace2D], layout2D, { displayModeBar: false });

        // Gestionar historia 3D
        history3D.push([...y_data]);
        if (history3D.length > MAX_HISTORY) {
            history3D.shift(); // Eliminar el más antiguo
        }

        // Si aún no tenemos suficientes datos para 3D (Plotly necesita al menos 2 filas), rellenamos
        let renderZ = history3D;
        if (renderZ.length === 1) {
            renderZ = [history3D[0], history3D[0]];
        }

        // Recuperar la posición actual de la cámara para que no salte
        const plot3DDiv = document.getElementById("plot3D");
        if (plot3DDiv && plot3DDiv.layout && plot3DDiv.layout.scene && plot3DDiv.layout.scene.camera) {
            layout3D.scene.camera = plot3DDiv.layout.scene.camera;
        }

        const trace3D = {
            z: renderZ,
            x: x_data,
            type: 'surface',
            colorscale: 'Jet',
            cmin: -10,
            cmax: 80,
            showscale: false
        };

        Plotly.react('plot3D', [trace3D], layout3D, { displayModeBar: false });
    }
});
