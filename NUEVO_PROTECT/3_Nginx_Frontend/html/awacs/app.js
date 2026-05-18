// ============================================================================
//  Sentinel AWACS — Monitor Dual Premium (v13 - RENDERIZADO LERP 60 FPS)
// ============================================================================

document.addEventListener('DOMContentLoaded', () => {
    const connectBtn = document.getElementById('connectBtn');
    const sourceSelect = document.getElementById('sourceSelect');
    const statusText = document.getElementById('statusText');
    const statusDot = document.getElementById('statusDot');
    const macId = document.getElementById('macId');
    const centerFreq = document.getElementById('centerFreq');
    const thresholdSlider = document.getElementById('thresholdSlider');
    const thresholdValue = document.getElementById('thresholdValue');
    const hudAlert = document.getElementById('hudAlert');
    const hwidError = document.getElementById('hwidError');

    // Panel Lateral
    const toggleBtn = document.getElementById('toggleBtn');
    const sidePanel = document.getElementById('sidePanel');
    if (toggleBtn && sidePanel) {
        toggleBtn.addEventListener('click', () => {
            sidePanel.classList.toggle('collapsed');
            toggleBtn.textContent = sidePanel.classList.contains('collapsed') ? '❯' : '❮';
            setTimeout(() => { if (chart) chart.resize(); }, 300);
        });
    }

    // Asegurar que el contenedor de la gráfica 3D sea visible
    const waterfall = document.getElementById('waterfallContainer');
    if (waterfall) waterfall.style.display = 'block';

    // Monitor táctico de tamaño fijo integrado en la barra lateral (debajo de Alert Threshold)
    const monitor = document.createElement('div');
    Object.assign(monitor.style, {
        marginTop: '15px',
        width: '100%',
        height: '240px', // Altura perfecta y fija para encajar en el panel
        backgroundColor: 'rgba(0, 0, 0, 0.85)',
        color: '#39ff14', // Verde Neón táctico
        fontFamily: 'Consolas, monospace',
        fontSize: '11px',
        padding: '10px',
        overflowY: 'auto', // Barra de desplazamiento automática
        border: '1px solid var(--border-dim)',
        borderRadius: '3px',
        boxShadow: 'inset 0 0 10px rgba(0,0,0,0.9)',
        lineHeight: '1.4em',
        whiteSpace: 'pre-wrap'
    });

    // Cabecera fija de la mini-consola con botón de limpiar integrado
    const consoleHeader = document.createElement('div');
    Object.assign(consoleHeader.style, {
        borderBottom: '1px solid #0a2f1a',
        paddingBottom: '6px',
        marginBottom: '8px',
        color: '#00f3ff',
        fontWeight: 'bold',
        fontSize: '10px',
        letterSpacing: '1px',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
    });
    
    const headerTitle = document.createElement('span');
    headerTitle.textContent = '📡 TELEMETRÍA EN VIVO';
    consoleHeader.appendChild(headerTitle);
    
    const clearBtn = document.createElement('button');
    clearBtn.textContent = 'LIMPIAR';
    Object.assign(clearBtn.style, {
        background: 'transparent',
        border: '1px solid #00f3ff',
        color: '#00f3ff',
        padding: '2px 8px',
        fontSize: '9px',
        cursor: 'pointer',
        fontFamily: 'monospace',
        borderRadius: '2px',
        textTransform: 'uppercase',
        transition: 'all 0.2s ease',
        fontWeight: 'bold'
    });
    
    clearBtn.addEventListener('mouseenter', () => {
        clearBtn.style.backgroundColor = 'rgba(0, 243, 255, 0.2)';
    });
    clearBtn.addEventListener('mouseleave', () => {
        clearBtn.style.backgroundColor = 'transparent';
    });
    
    clearBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        logContent.innerHTML = '';
        logSweepBlock("=== CONSOLA PURGADA ===", "#00f3ff");
    });
    
    consoleHeader.appendChild(clearBtn);
    monitor.appendChild(consoleHeader);

    // Contenedor interno de los logs
    const logContent = document.createElement('div');
    monitor.appendChild(logContent);

    // Insertar debajo del bloque de Alert Threshold en la sección stats de la barra lateral
    const statsSection = document.querySelector('.panel-section.stats');
    if (statsSection) {
        statsSection.appendChild(monitor);
    } else {
        document.body.appendChild(monitor);
    }

    // Búfer para la mini-consola integrada (limita a 40 bloques de sweep completo en pantalla para rendimiento extremo)
    function logSweepBlock(textBlock, color = '#39ff14') {
        const preElement = document.createElement('pre');
        preElement.style.color = color;
        preElement.style.margin = '0 0 12px 0';
        preElement.style.padding = '0';
        preElement.style.fontFamily = 'inherit';
        preElement.style.fontSize = 'inherit';
        preElement.textContent = textBlock;
        
        logContent.appendChild(preElement);
        
        if (logContent.childNodes.length > 40) {
            logContent.removeChild(logContent.firstChild);
        }
        
        // Autoscroll robusto diferido para asegurar cálculo tras pintado del DOM
        setTimeout(() => {
            monitor.scrollTop = monitor.scrollHeight;
        }, 0);
    }

    logSweepBlock("=== MODO INTEGRADO DUAL INICIALIZADO ===", "#00f3ff");

    // Cargar estaciones
    async function loadStations() {
        logSweepBlock("Cargando estaciones...");
        try {
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
            logSweepBlock("OK: Estaciones cargadas con éxito.");
        } catch (e) { 
            logSweepBlock(`ERROR cargando estaciones: ${e.message}`, "red");
            sourceSelect.disabled = false;
            connectBtn.disabled = false;
        }
    }
    loadStations();

    // Inicializar Gráfica ECharts GL 3D Waterfall
    let chart = null;
    let historyBuffer = [];
    const MAX_SWEEPS = 45; // Profundidad de cascada
    let currentThreshold = parseInt(thresholdSlider.value);

    thresholdSlider.addEventListener('input', (e) => {
        currentThreshold = parseInt(e.target.value);
        thresholdValue.textContent = `${currentThreshold} dBµV`;
    });

    function initChart() {
        if (!waterfall) return;
        chart = echarts.init(waterfall);
        
        const option = {
            backgroundColor: 'transparent',
            tooltip: {
                show: true,
                formatter: (params) => {
                    if (params.value && params.value.length >= 3) {
                        return `Frec: ${params.value[0].toFixed(3)} MHz<br>Barrido: ${params.value[1]}<br>Nivel: ${params.value[2].toFixed(1)} dBµV`;
                    }
                    return '';
                }
            },
            visualMap: {
                show: true,
                min: -110,
                max: -30,
                dimension: 2,
                right: 15,
                top: 'center',
                text: ['dBµV', ''],
                textStyle: { color: '#e0e0e0', fontFamily: 'monospace' },
                inRange: {
                    color: ['rgba(3, 5, 4, 0.85)', 'rgba(10, 31, 20, 0.95)', '#10522c', '#39ff14', '#00f3ff', '#ff003c']
                }
            },
            xAxis3D: {
                type: 'value',
                name: 'FRECUENCIA (MHz)',
                nameTextStyle: { color: 'var(--cyan)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'var(--border-dim)' } },
                axisLabel: { color: '#e0e0e0', fontFamily: 'monospace', formatter: (val) => val.toFixed(1) },
                splitLine: { lineStyle: { color: 'rgba(10, 31, 20, 0.15)' } }
            },
            yAxis3D: {
                type: 'value',
                name: 'TIEMPO (BARRIDOS)',
                nameTextStyle: { color: 'var(--cyan)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'var(--border-dim)' } },
                axisLabel: { show: false },
                splitLine: { lineStyle: { color: 'rgba(10, 31, 20, 0.15)' } }
            },
            zAxis3D: {
                type: 'value',
                name: 'NIVEL',
                nameTextStyle: { color: 'var(--alert-red)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'var(--border-dim)' } },
                axisLabel: { color: '#e0e0e0', fontFamily: 'monospace', formatter: '{value} dB' },
                splitLine: { lineStyle: { color: 'rgba(10, 31, 20, 0.15)' } }
            },
            grid3D: {
                boxWidth: 100,
                boxDepth: 60,
                boxHeight: 35,
                viewControl: {
                    projection: 'perspective',
                    autoRotate: false,
                    distance: 105,
                    alpha: 35,
                    beta: -45
                },
                light: {
                    main: { intensity: 1.5, shadow: false },
                    ambient: { intensity: 0.8 }
                }
            },
            series: [{
                type: 'surface', // CASCADA DE ONDA SÓLIDA FLUIDA Y SUAVE (GPU hardware accelerated)
                data: [],
                wireframe: {
                    show: false // ¡CLAVE DE RENDIMIENTO! Desactiva las líneas de rejilla pesadas
                },
                shading: 'color', // Sombreado de color directo súper rápido
                silent: true
            }]
        };
        chart.setOption(option);
    }

    initChart();
    window.addEventListener('resize', () => { if (chart) chart.resize(); });

    // --- SINTETIZADOR DE SONIDO DE ALERTA RÍTMICO TÁCTICO ---
    let lastBeepTime = 0;
    function emitirPitidoAlerta() {
        const now = Date.now();
        if (now - lastBeepTime < 800) return; // Limitar ritmo a un pulso cada 800ms
        lastBeepTime = now;
        
        try {
            if (!window.audioCtx) {
                window.audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            }
            if (window.audioCtx.state === 'suspended') {
                window.audioCtx.resume();
            }
            
            const osc = window.audioCtx.createOscillator();
            const gain = window.audioCtx.createGain();
            
            osc.connect(gain);
            gain.connect(window.audioCtx.destination);
            
            osc.type = 'sine';
            osc.frequency.setValueAtTime(880, window.audioCtx.currentTime); // Tono agudo táctico (880 Hz)
            
            // Efecto atenuador (fade-out suave) premium militar
            gain.gain.setValueAtTime(0.15, window.audioCtx.currentTime);
            gain.gain.exponentialRampToValueAtTime(0.001, window.audioCtx.currentTime + 0.15);
            
            osc.start();
            osc.stop(window.audioCtx.currentTime + 0.15);
        } catch (e) {
            console.warn("El sintetizador de audio ha fallado:", e);
        }
    }

    let ws = null;
    let sweepCounter = 0;

    // --- VARIABLES DE DESACOPLE Y LERP ---
    let latestFrame = null;
    let newFrameAvailable = false;
    let isRenderLoopActive = false;
    let targetSweep = null;

    // ==== MOTOR DE RENDERIZADO "GAME LOOP" ASÍNCRONO ULTRA-FLUIDO (60 FPS) ====
    function renderLoop() {
        if (!ws) {
            isRenderLoopActive = false;
            return;
        }

        const fmin = window._cal_fmin || 0;
        const fmax = window._cal_fmax || 0;

        // 1. Cuando llega un nuevo paquete, deslizamos las filas físicas del waterfall
        if (newFrameAvailable && latestFrame) {
            const msg = latestFrame;
            newFrameAvailable = false;
            latestFrame = null;

            const count = msg.sweep.length;
            const step = (count > 1) ? (fmax - fmin) / (count - 1) : 0;
            
            // --- Formateo de Consola Táctica en Bloque ---
            const timeStr = new Date().toLocaleTimeString();
            let logLines = [];
            logLines.push(`[${timeStr}] DATA -> ${count} bins recibidos.`);
            msg.sweep.forEach((lv, i) => {
                const freq = (fmin + i * step) / 1e6;
                logLines.push(`[${timeStr}]   [Bin ${i}] ${freq.toFixed(3)} MHz  -->  ${lv.toFixed(1)} dB`);
            });
            logSweepBlock(logLines.join("\n"), "#39ff14");
            
            // Desplazar las coordenadas Y de las filas del histórico hacia atrás en la cascada
            historyBuffer.forEach(sweepPoints => {
                sweepPoints.forEach(pt => {
                    pt[1] += 1; 
                });
            });
            
            // Insertar una nueva fila al inicio (Y = 0) heredando los niveles de la fila anterior
            // para evitar saltos geométricos bruscos y permitir una interpolación suave en 60 FPS
            const initialSweep = (historyBuffer.length > 0) ? historyBuffer[0].map(pt => pt[2]) : msg.sweep;
            const newPoints = initialSweep.map((lv, i) => {
                const freq = (fmin + i * step) / 1e6;
                return [freq, 0, lv]; // [Frecuencia, Tiempo, Nivel]
            });
            
            historyBuffer.unshift(newPoints);
            if (historyBuffer.length > MAX_SWEEPS) {
                historyBuffer.pop();
            }
            
            targetSweep = msg.sweep;
            sweepCounter++;
        }

        // 2. Ejecutar la interpolación LERP (60 FPS) sobre la fila activa en cada ciclo del monitor
        if (targetSweep && historyBuffer.length > 0) {
            const frontRow = historyBuffer[0];
            let hasAlert = false;
            let maxLevel = -150;
            let minLevel = 0;
            
            frontRow.forEach((pt, i) => {
                // Ecuación LERP: Nivel += (Nivel Objetivo - Nivel Actual) * Factor de Suavizado (0.22)
                pt[2] += (targetSweep[i] - pt[2]) * 0.22;
                
                if (pt[2] > currentThreshold) hasAlert = true;
                if (pt[2] > maxLevel) maxLevel = pt[2];
                if (pt[2] < minLevel) minLevel = pt[2];
            });
            
            // Aplanar todo el volumen 3D para pasárselo a la GPU
            let flatData = [];
            historyBuffer.forEach(sweepPoints => {
                flatData.push(...sweepPoints);
            });
            
            // 3. Dibujado y Calibración sobre la GPU
            if (chart) {
                // Calibrar la escala en el primer barrido del flujo (Estable y bloqueado)
                if (sweepCounter === 1) {
                    const fminM = fmin / 1e6;
                    const fmaxM = fmax / 1e6;
                    const zMin = Math.floor(minLevel) - 10;
                    const zMax = Math.ceil(maxLevel) + 5;
                    chart.setOption({
                        xAxis3D: { min: fminM, max: fmaxM },
                        zAxis3D: { min: zMin, max: zMax },
                        visualMap: { min: zMin, max: zMax }
                    });
                }
                
                // Actualizar vértices del Waterfall en la GPU
                chart.setOption({
                    series: [{ data: flatData }]
                }, { lazyUpdate: true });
            }
            
            // --- Alertas y Sonido ---
            if (hasAlert) {
                hudAlert.classList.add('active');
                emitirPitidoAlerta();
            } else {
                hudAlert.classList.remove('active');
            }
            
            // --- Umbral de Alerta Autoadaptativo ---
            const avgLevel = Math.round(targetSweep.reduce((sum, v) => sum + v, 0) / targetSweep.length);
            const sliderMin = avgLevel - 50;
            const sliderMax = avgLevel + 50;
            
            if (parseInt(thresholdSlider.min) !== sliderMin || parseInt(thresholdSlider.max) !== sliderMax) {
                thresholdSlider.min = sliderMin;
                thresholdSlider.max = sliderMax;
                if (sweepCounter === 1) {
                    thresholdSlider.value = Math.min(sliderMax, Math.max(sliderMin, avgLevel + 15));
                } else {
                    thresholdSlider.value = Math.min(sliderMax, Math.max(sliderMin, currentThreshold));
                }
                currentThreshold = parseInt(thresholdSlider.value);
                thresholdValue.textContent = `${currentThreshold} dBµV`;
            }
        }

        // Bucle de renderizado continuo sincronizado por hardware (60 FPS)
        requestAnimationFrame(renderLoop);
    }

    connectBtn.addEventListener('click', () => {
        // Desbloqueo del sintetizador en clic directo del usuario
        try {
            if (!window.audioCtx) {
                window.audioCtx = new (window.AudioContext || window.webkitAudioContext)();
            } else if (window.audioCtx.state === 'suspended') {
                window.audioCtx.resume();
            }
        } catch (e) {
            console.warn("No se pudo desbloquear el AudioContext:", e);
        }

        if (ws) { 
            ws.close(); 
            return; 
        }
        const selected = sourceSelect.options[sourceSelect.selectedIndex];
        if (!selected) { 
            logSweepBlock("Error: Selecciona una estación", "red"); 
            return; 
        }

        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsPort = window.location.port ? `:${window.location.port}` : '';
        const wsUrl = `${wsProto}//${window.location.hostname}${wsPort}/ws/`;
        logSweepBlock(`Conectando: ${wsUrl}...`, "yellow");
        
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            logSweepBlock("Autenticando WebSocket...", "yellow");
            statusText.textContent = "AUTH...";
            statusDot.className = "dot blink";
            
            const token = document.cookie.match(/jwt=([^;]+)/)?.[1] || 'anonymous';
            ws.send(JSON.stringify({ action: 'auth', token }));
            
            setTimeout(() => {
                ws.send(JSON.stringify({ action: 'subscribe', source_path: selected.dataset.physicalPath }));
                logSweepBlock(`Subscrito a share: ${selected.text}`, "var(--cyan)");
                statusText.textContent = "SCANNING";
                statusText.parentElement.classList.add('online');
                statusDot.className = "dot";
                connectBtn.textContent = "DETENER BARRIDO";
                connectBtn.style.backgroundColor = "var(--alert-red)";
                connectBtn.style.color = "#000";
            }, 200);
        };

        ws.onmessage = (e) => {
            // Decoplamiento ultra-rápido: Almacenamos el mensaje y cedemos el control de inmediato
            latestFrame = JSON.parse(e.data);
            newFrameAvailable = true;

            if (latestFrame.type === 'delta_frame' && latestFrame.sweep) {
                // Arrancar el Game Loop de renderizado continuo si no estuviera corriendo
                if (!isRenderLoopActive) {
                    isRenderLoopActive = true;
                    requestAnimationFrame(renderLoop);
                }
            } else if (latestFrame.type === 'init_frame') {
                window._cal_fmin = latestFrame.fmin;
                window._cal_fmax = latestFrame.fmax;
                
                const fminM = (latestFrame.fmin / 1e6).toFixed(2);
                const fmaxM = (latestFrame.fmax / 1e6).toFixed(2);
                const fCenter = ((latestFrame.fmin + latestFrame.fmax) / 2 / 1e6).toFixed(2);
                
                centerFreq.textContent = `${fCenter} MHz`;
                macId.textContent = "ANT-ARGUS-PRO";
                
                logSweepBlock(`INIT -> Rango: ${fminM} - ${fmaxM} MHz`, "var(--cyan)");
                
                // Limpiar buffers
                historyBuffer = [];
                sweepCounter = 0;
                targetSweep = null;
            } else {
                if (latestFrame.event === 'error_security') {
                    hwidError.classList.add('active');
                    logSweepBlock("ALERTA DE SEGURIDAD: HWID mismatch", "red");
                } else {
                    logSweepBlock(`SISTEMA: ${latestFrame.type} ${latestFrame.event || ''}`, "#888");
                }
            }
        };

        ws.onclose = () => {
            logSweepBlock("Conexión cerrada.", "yellow");
            statusText.textContent = "STANDBY";
            statusText.parentElement.classList.remove('online');
            statusDot.className = "dot blink";
            connectBtn.textContent = "INICIAR BARRIDO";
            connectBtn.style.backgroundColor = "#051510";
            connectBtn.style.color = "var(--neon-green)";
            ws = null;
            hudAlert.classList.remove('active');
        };
        
        ws.onerror = (err) => logSweepBlock(`Error WS: ${err.message}`, "red");
    });
});
