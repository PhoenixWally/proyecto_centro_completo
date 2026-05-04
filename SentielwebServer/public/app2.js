let ws = null;
let currentSource = "UMA";

let isConnected = false;
let autoCaptureCounter = 0; // Contador de tramas para la red

let latestRadarData = null; // Buffer para el Game Loop
let isRenderLoopRunning = false;

const btnStart = document.getElementById('btn-start');
const btnStop = document.getElementById('btn-stop');
const comboFuentes = document.getElementById('combo-fuentes');
const btnConfigFuentes = document.getElementById('btn-config-fuentes');
const lblStatus = document.getElementById('lbl-status');

// Cargar Fuentes via API Local al Iniciar
async function loadSources() {
    try {
        const resp = await fetch('/api/fuentes');
        const fuentes = await resp.json();
        comboFuentes.innerHTML = '';
        const tbody = document.getElementById('tbody-fuentes');
        tbody.innerHTML = '';

        fuentes.forEach(f => {
            const opt = document.createElement('option');
            opt.value = f.id; opt.textContent = f.id;
            comboFuentes.appendChild(opt);

            // Añadir a la tabla del modal
            const tr = document.createElement('tr');
            tr.innerHTML = `
                <td>${f.id}</td>
                <td>${f.path}</td>
                <td>${f.user || '-'}</td>
                <td>${f.password ? '****' : '-'}</td>
                <td><button class="btn-small" style="background:#f44336;" onclick="deleteSource('${f.id}')">Borrar</button></td>
            `;
            tbody.appendChild(tr);
        });
    } catch (e) {
        console.error("Error al cargar fuentes. ¿Está encendido el Http Server?", e);
    }
}
window.deleteSource = async function (id) {
    try {
        const resp = await fetch('/api/fuentes');
        let fuentes = await resp.json();
        fuentes = fuentes.filter(f => f.id !== id);
        await fetch('/api/fuentes', {
            method: 'POST', body: JSON.stringify(fuentes), headers: { 'Content-Type': 'application/json' }
        });
        loadSources();
    } catch (e) { }
};

document.getElementById('btn-config-fuentes').onclick = () => {
    document.getElementById('modal-fuentes').style.display = 'block';
};
document.getElementById('btn-close-modal').onclick = () => {
    document.getElementById('modal-fuentes').style.display = 'none';
};

window.addFuente = async function () {
    const id = document.getElementById('new-id').value;
    const path = document.getElementById('new-path').value;
    const user = document.getElementById('new-user').value;
    const pass = document.getElementById('new-pass').value;
    if (!id || !path) return;
    try {
        const resp = await fetch('/api/fuentes');
        const fuentes = await resp.json();
        fuentes.push({ id, path, user, password: pass });
        await fetch('/api/fuentes', {
            method: 'POST', body: JSON.stringify(fuentes), headers: { 'Content-Type': 'application/json' }
        });
        loadSources();
        document.getElementById('new-id').value = '';
        document.getElementById('new-path').value = '';
    } catch (e) { }
};

loadSources();

// Conexión WebSockets
const logText = document.getElementById("sys-log");
const txtDetections = document.getElementById("txt-detections");
const entryFmin = document.getElementById("entry-fmin");
const entryFmax = document.getElementById("entry-fmax");
const entryThreshold = document.getElementById("entry-threshold");

function writeLog(msg) {
    const timeStr = new Date().toLocaleTimeString();
    logText.innerHTML += `[${timeStr}] ${msg}<br>`;
    logText.scrollTop = logText.scrollHeight;
}

function writeDetection(msg) {
    txtDetections.innerHTML += `${msg}<br>`;
    txtDetections.scrollTop = txtDetections.scrollHeight;
}

// ==== SINTETIZADOR SONORO MODERNO ====
const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
let lastBeepTime = 0;
function playBeep() {
    const now = Date.now();
    if (now - lastBeepTime < 500) return; // Máximo 2 beeps por segundo
    lastBeepTime = now;

    if (audioCtx.state === 'suspended') audioCtx.resume();
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();
    osc.type = 'sawtooth';
    osc.frequency.setValueAtTime(880, audioCtx.currentTime); // 880 Hz

    gain.gain.setValueAtTime(0.2, audioCtx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.01, audioCtx.currentTime + 0.3);

    osc.connect(gain);
    gain.connect(audioCtx.destination);
    osc.start();
    osc.stop(audioCtx.currentTime + 0.3);
}

// Inicializar Gráficas (Plotly)
const layout3D = {
    title: false,
    autosize: true,
    margin: { l: 0, r: 0, b: 0, t: 0 },
    paper_bgcolor: '#363434ff',
    plot_bgcolor: '#0d0e0dff',
    font: { color: '#ffffff' },
    scene: {
        camera: {
            // eye controla la cámara por defecto. 
            // x e y controlan la rotación (para intercambiar izquierda/derecha).
            // z controla la altura o inclinación vertical.
            eye: { x: 1.2, y: -1.2, z: 0.4 },
            projection: { type: 'orthographic' }
        },
        xaxis: { title: 'Frecuencia', backgroundcolor: "black", showbackground: true, color: "#ffffff" },
        yaxis: {
            title: 'Tiempo (1 Fotograma ≈ 500ms)<br>',
            backgroundcolor: "black",
            showbackground: true,
            color: "#ffffff",
            tickmode: 'array',
            tickvals: Array.from({ length: 13 }, (_, j) => -12.0 + (j * 2) * 0.5),
            ticktext: Array.from({ length: 13 }, (_, j) => { let i = j * 2; return `${i}-${(12.0 - i * 0.5).toFixed(1).replace('.0', '')}sg`; })
        },
        zaxis: {
            title: 'dBµV',
            backgroundcolor: "black",
            showbackground: true,
            range: [-10, 80],
            color: "#ffffff",
            tickmode: 'linear',
            tick0: -10,
            dtick: 10
        },
        aspectratio: { x: 1.5, y: 1.5, z: 0.6 } // Cúbico y equilibrado
    }
};

const layout2D = {
    title: false,
    autosize: true,
    margin: { l: 40, r: 20, b: 30, t: 10 },
    paper_bgcolor: '#000000',
    plot_bgcolor: '#0e0e2eff',
    font: { color: '#ffffff' },
    xaxis: { title: 'Frecuencia (MHz)', gridcolor: '#333', color: '#ffffff' },
    yaxis: { title: 'Nivel (dBµV)', gridcolor: '#393a39ff', range: [-10, 80], color: '#ffffff' },
    showlegend: false
};

Plotly.newPlot('plot3D', [{ z: [[0, 0], [0, 0]], type: 'surface' }], layout3D, { displayModeBar: false });
Plotly.newPlot('plot2D', [{ x: [0], y: [0], type: 'scatter' }], layout2D, { displayModeBar: false });

// Limpieza de memoria (UI y Servidor)
window.clearPlots = function () {
    if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ action: "clear_cache", source: currentSource }));
    }
    Plotly.react('plot3D', [{ z: [[0, 0], [0, 0]], type: 'surface', showscale: false }], layout3D, { displayModeBar: false });
    Plotly.react('plot2D', [{ x: [0], y: [0], type: 'scatter' }], layout2D, { displayModeBar: false });
    writeLog("Visores gráficos y búfer C++ borrados por orden de usuario.");
};

window.resetCamera3D = function () {
    layout3D.scene.camera = { eye: { x: 0.5, y: -2.0, z: 0.6 } };
    Plotly.relayout('plot3D', { 'scene.camera': layout3D.scene.camera });
};

window.resetZoom2D = function () {
    Plotly.relayout('plot2D', { 'xaxis.autorange': true, 'yaxis.range': [-10, 80] });
};

// Ajuste dinámico de Plotly al estirar la ventana
window.addEventListener('resize', () => {
    Plotly.Plots.resize(document.getElementById('plot3D'));
    Plotly.Plots.resize(document.getElementById('plot2D'));

    // Ancla la cámara 3D para evitar que el cubo de WebGL salga volando fuera del cuadro
    const p3d = document.getElementById('plot3D');
    if (p3d && p3d.layout && p3d.layout.scene && p3d.layout.scene.camera) {
        layout3D.scene.camera = p3d.layout.scene.camera;
        Plotly.relayout('plot3D', {
            'scene.camera': layout3D.scene.camera,
            'scene.aspectmode': 'auto'
        });
    } else {
        window.resetCamera3D();
    }
});

function updateFilters() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    let fmin = parseFloat(entryFmin.value);
    let fmax = parseFloat(entryFmax.value);

    ws.send(JSON.stringify({
        action: "update_filters",
        source: currentSource,
        fmin: isNaN(fmin) ? null : fmin,
        fmax: isNaN(fmax) ? null : fmax
    }));
}

// ==== Light / Dark Mode ====
let isLightMode = false;
window.toggleTheme = function () {
    isLightMode = !isLightMode;
    const body = document.body;
    const btnTheme = document.getElementById("btn-theme-toggle");

    // Tweak body class
    if (isLightMode) {
        body.classList.add("light-mode");
        btnTheme.innerHTML = "🌙";
    } else {
        body.classList.remove("light-mode");
        btnTheme.innerHTML = "☀️";
    }

    // Refresh Plotly Colors
    const pltBg = isLightMode ? "#ffffff" : "#0d0d0d";
    const pltPaper = isLightMode ? "#f9f9f9" : "#0d0d0d";
    const pltFont = isLightMode ? "#111111" : "#ffffff";
    const gridC = isLightMode ? "#cccccc" : "#333333";
    const axBg = isLightMode ? "#f0f0f0" : "black";

    Plotly.relayout('plot3D', {
        'paper_bgcolor': pltBg,
        'plot_bgcolor': pltBg,
        'font.color': pltFont,
        'scene.xaxis.backgroundcolor': axBg,
        'scene.yaxis.backgroundcolor': axBg,
        'scene.zaxis.backgroundcolor': axBg,
        'scene.xaxis.color': pltFont,
        'scene.yaxis.color': pltFont,
        'scene.zaxis.color': pltFont
    });

    Plotly.relayout('plot2D', {
        'paper_bgcolor': pltPaper,
        'plot_bgcolor': pltPaper,
        'font.color': pltFont,
        'xaxis.gridcolor': gridC,
        'yaxis.gridcolor': gridC,
        'xaxis.color': pltFont,
        'yaxis.color': pltFont
    });
};

entryFmin.addEventListener('change', updateFilters);
entryFmax.addEventListener('change', updateFilters);

// == HELPERS PARA GRABAR LOS GRÁFICOS 3D EN IMAGEN (Superando el límite de WebGL con html2canvas) ==
async function doFullCapture() {
    const p3d = document.getElementById("plot3D");
    const p2d = document.getElementById("plot2D");

    const w3 = p3d.offsetWidth || 800; const h3 = p3d.offsetHeight || 600;
    const w2 = p2d.offsetWidth || 800; const h2 = p2d.offsetHeight || 400;

    const img3dURL = await Plotly.toImage(p3d, { format: 'jpeg', width: w3, height: h3 });
    const img2dURL = await Plotly.toImage(p2d, { format: 'jpeg', width: w2, height: h2 });

    const img3d = document.createElement("img");
    img3d.src = img3dURL;
    img3d.style.position = "absolute"; img3d.style.left = "0"; img3d.style.top = "0";
    img3d.style.zIndex = "99";

    const img2d = document.createElement("img");
    img2d.src = img2dURL;
    img2d.style.position = "absolute"; img2d.style.left = "0"; img2d.style.top = "0";
    img2d.style.zIndex = "99";

    p3d.style.position = "relative"; p3d.appendChild(img3d);
    p2d.style.position = "relative"; p2d.appendChild(img2d);

    // Escondemos los lienzos de plotly nativos
    const plotsC = document.querySelectorAll('.plotly');
    plotsC.forEach(c => c.style.opacity = '0');

    const canvas = await html2canvas(document.body, { backgroundColor: isLightMode ? '#eef2f5' : '#111' });

    // Restauramos
    p3d.removeChild(img3d);
    p2d.removeChild(img2d);
    plotsC.forEach(c => c.style.opacity = '1');

    return canvas;
}

// Guardado automático rápido sin congelar la web (sin html2canvas)
window.takeScreenshotAndUploadFast = async function () {
    try {
        const p3d = document.getElementById("plot3D");
        // Capturar solo la gráfica 3D que es lo más importante y es instantáneo
        const base64Data = await Plotly.toImage(p3d, { format: 'jpeg', width: 800, height: 600 });
        fetch('/api/captura', {
            method: 'POST',
            body: JSON.stringify({ src: currentSource, image: base64Data }),
            headers: { 'Content-Type': 'application/json' }
        });
    } catch (e) { console.error("Error en autocaptura rápida:", e); }
};

// Guardado automático en el servidor (original)
window.takeScreenshotAndUpload = async function () {
    try {
        const canvas = await doFullCapture();
        const base64Data = canvas.toDataURL('image/jpeg', 0.8);
        fetch('/api/captura', {
            method: 'POST',
            body: JSON.stringify({ src: currentSource, image: base64Data }),
            headers: { 'Content-Type': 'application/json' }
        });
        writeLog("Auto-Captura rotativa subida al servidor.");
    } catch (e) { console.error("Error en autocaptura:", e); }
};

// Descarga manual local (Para que el usuario elija dónde guardar)
window.captureManual = async function () {
    try {
        const canvas = await doFullCapture();
        const base64Data = canvas.toDataURL('image/jpeg', 0.95);
        const link = document.createElement('a');
        link.download = `Captura_SentinelHD_${currentSource}_${Date.now()}.jpg`;
        link.href = base64Data;
        link.click();
        writeLog("Captura manual completada y descargada.");
    } catch (e) { console.error("Error en captura manual:", e); }
};

btnStart.addEventListener("click", () => {
    // Protección vital: Si config_fuentes.json no existe en el nuevo PC, el combo estará vacío
    if (!comboFuentes.value) {
        writeLog("[Error] La lista de fuentes está vacía. Pulse 'Configurar Fuentes' para añadir una red Argus en este nuevo ordenador.");
        return;
    }

    currentSource = comboFuentes.value;
    const wsUrl = `ws://${window.location.hostname}:8081`;

    writeLog(`Conectando a motor motor espacial C++ en ${wsUrl}...`);
    ws = new WebSocket(wsUrl);

    ws.onerror = () => {
        writeLog("[Error Critico] El navegador no puede conectar con el puerto 8081.");
        writeLog("Asegúrese de que el C++.exe está abierto y el Firewall de Windows no lo está bloqueando.");
    };

    ws.onclose = () => {
        isConnected = false;
        lblStatus.innerText = "Estado: DESCONECTADO";
        lblStatus.style.color = "#FF9800";
        btnStart.disabled = false;
        btnStop.disabled = true;
    };

    ws.onopen = () => {
        isConnected = true;
        lblStatus.innerText = `Estado: CONECTADO (${currentSource})`;
        lblStatus.style.color = "#4CAF50";
        btnStart.disabled = true;
        btnStop.disabled = false;
        writeLog(`Conexión establecida. Recibiendo datos de [${currentSource}]`);

        ws.send(JSON.stringify({
            action: "subscribe",
            source: currentSource
        }));
        updateFilters();

        // Iniciar Game Loop de renderizado
        if (!isRenderLoopRunning) {
            isRenderLoopRunning = true;
            requestAnimationFrame(renderLoop);
        }
    };

    ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        if (data.type === "log_msg") {
            writeLog(data.msg);
            if (data.msg.includes("[Error]")) {
                lblStatus.innerText = "Estado: Bloqueo/Error Lectura";
                lblStatus.style.color = "#FF9800";
            }
        }
        else if (data.type === "radar_frame") {
            // Guardar para el renderizado asíncrono
            latestRadarData = data;

            // Algoritmo de Detección de Múltiples Picos Locales (Procesa el 100% de las tramas)
            let thres = parseFloat(entryThreshold.value) || 15;
            for (let i = 1; i < data.y2d.length - 1; i++) {
                if (data.y2d[i] >= thres) {
                    // Es un pico real si sobresale de su montículo (mayor que los vecinos)
                    if (data.y2d[i] > data.y2d[i - 1] && data.y2d[i] > data.y2d[i + 1]) {
                        writeDetection(`T:${data.time} -> F:${data.x2d[i].toFixed(4)} L:${data.y2d[i].toFixed(1)}`);
                        picos_detectados++;
                    }
                }
            }

            // Salvavidas por si la señal es muy plana o está en los bordes
            let max_y = Math.max(...data.y2d);
            if (picos_detectados === 0 && max_y >= thres) {
                let idx = data.y2d.indexOf(max_y);
                writeDetection(`T:${data.time} -> F:${data.x2d[idx].toFixed(4)} L:${max_y.toFixed(1)}`);
                picos_detectados++;
            }

            if (picos_detectados > 0) {
                const chk = document.getElementById("chk-alarm");
                if (chk && chk.checked) {
                    playBeep();
                }
            }
        }
    };

    ws.onclose = () => {
        isConnected = false;
        lblStatus.innerText = "Estado: Desconectado";
        lblStatus.style.color = "#f44336";
        btnStart.disabled = false;
        btnStop.disabled = true;
        writeLog("Desconectado del servidor de Trazas.");
    };
});

// ==== MOTOR DE RENDERIZADO "GAME LOOP" ====
// Desacopla la recepción de datos de la generación de gráficas
function renderLoop() {
    if (!isConnected) {
        isRenderLoopRunning = false;
        return;
    }

    if (latestRadarData) {
        const data = latestRadarData;
        latestRadarData = null; // Limpiar para no repintar lo mismo

        document.getElementById("lbl-time-sync").innerHTML = data.time;

        // Render 3D Surface
        const trace3D = {
            z: data.z3d,
            x: data.x3d,
            y: Array.from({ length: 25 }, (_, i) => -12.0 + i * 0.5), // Eje Y en segundos
            colorscale: 'Jet',
            type: 'surface',
            cmin: data.db_min,
            cmax: data.db_max,
            showscale: false
        };

        const plot3DDiv = document.getElementById("plot3D");
        if (plot3DDiv && plot3DDiv.layout && plot3DDiv.layout.scene && plot3DDiv.layout.scene.camera) {
            layout3D.scene.camera = plot3DDiv.layout.scene.camera;
        }

        Plotly.react('plot3D', [trace3D], layout3D, { displayModeBar: false });

        // Render 2D Line & Threshold Line
        let thres = parseFloat(entryThreshold.value) || 15;
        const trace2D = {
            x: data.x2d,
            y: data.y2d,
            type: 'scatter',
            mode: 'lines+markers',
            marker: {
                color: data.y2d,
                colorscale: 'Jet',
                cmin: data.db_min,
                cmax: data.db_max,
                size: 5
            },
            line: { color: 'rgba(6, 243, 18, 0.91)', width: 2 }
        };
        const thres2D = {
            x: [Math.min(...data.x2d), Math.max(...data.x2d)],
            y: [thres, thres],
            type: 'scatter',
            mode: 'lines',
            line: { color: '#FFD700', width: 1, dash: 'dash' }
        };
        Plotly.react('plot2D', [trace2D, thres2D], layout2D, { displayModeBar: false });

        // ================= AUTO CAPTURA ROTATIVA (Rápida) =================
        autoCaptureCounter++;
        if (autoCaptureCounter >= 10) {
            takeScreenshotAndUploadFast();
            autoCaptureCounter = 0;
        }
        // ==================================================================
    }

    // Pedir el siguiente frame a la tarjeta gráfica (60 FPS)
    requestAnimationFrame(renderLoop);
}
// ===========================================

btnStop.addEventListener("click", () => {
    if (ws) {
        ws.close();
        ws = null;
    }
});

// Botones de Zoom Manual
document.getElementById('btn-zoom-in').addEventListener('click', () => {
    layout3D.scene.camera.eye.x *= 0.8;
    layout3D.scene.camera.eye.y *= 0.8;
    layout3D.scene.camera.eye.z *= 0.8;
    Plotly.relayout('plot3D', { 'scene.camera': layout3D.scene.camera });
});

document.getElementById('btn-zoom-out').addEventListener('click', () => {
    layout3D.scene.camera.eye.x *= 1.25;
    layout3D.scene.camera.eye.y *= 1.25;
    layout3D.scene.camera.eye.z *= 1.25;
    Plotly.relayout('plot3D', { 'scene.camera': layout3D.scene.camera });
});
