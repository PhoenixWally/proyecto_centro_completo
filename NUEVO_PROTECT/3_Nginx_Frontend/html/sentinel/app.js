// ============================================================================
//  Sentinel Radar — Monitor Dual Premium (v14 - INTERACTIVO Y BOTONES SEPARADOS)
// ============================================================================

document.addEventListener('DOMContentLoaded', () => {
    const startBtn = document.getElementById('startBtn');
    const stopBtn = document.getElementById('stopBtn');
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

    // Botón de Volver al Portal
    const backToPortalBtn = document.getElementById('backToPortalBtn');
    if (backToPortalBtn) {
        backToPortalBtn.addEventListener('click', () => {
            window.location.href = '/sso/';
        });
    }

    // Asegurar que el contenedor de la gráfica 3D sea visible
    const waterfall = document.getElementById('waterfallContainer');
    if (waterfall) waterfall.style.display = 'block';

    // Contenedor principal del bloque de log (para albergar cabecera fija y caja con scroll)
    const logWrapper = document.createElement('div');
    Object.assign(logWrapper.style, {
        marginTop: '15px',
        width: '100%',
        display: 'flex',
        flexDirection: 'column'
    });

    // Cabecera fija de la mini-consola con botón de limpiar integrado (fuera de la caja con scroll!)
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
    logWrapper.appendChild(consoleHeader);

    // Monitor táctico de tamaño fijo integrado en la barra lateral (debajo de Alerta Pico)
    const monitor = document.createElement('div');
    Object.assign(monitor.style, {
        width: '100%',
        height: '210px', // Altura perfecta y fija
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

    // Contenedor interno de los logs
    const logContent = document.createElement('div');
    monitor.appendChild(logContent);
    logWrapper.appendChild(monitor);

    // Insertar debajo del bloque de Alerta Pico en la barra lateral
    const statsSection = document.querySelector('.panel-section.stats');
    if (statsSection) {
        statsSection.appendChild(logWrapper);
    } else {
        document.body.appendChild(logWrapper);
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
            startBtn.disabled = false;
            logSweepBlock("OK: Estaciones cargadas con éxito.");
        } catch (e) {
            logSweepBlock(`ERROR cargando estaciones: ${e.message}`, "red");
            sourceSelect.disabled = false;
            startBtn.disabled = false;
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
                nameGap: 30,
                interval: 0.2, // Cuadrícula de alta definición militar cada 200 kHz (0.2 MHz)
                nameTextStyle: { color: 'var(--cyan)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.85)' } },
                axisLabel: {
                    color: '#e0e0e0',
                    fontFamily: 'monospace',
                    formatter: (val) => {
                        // Mostrar etiquetas de texto cada 0.5 o 1.0 MHz para prevenir solapamientos visuales
                        const dec = val % 1;
                        if (Math.abs(dec) < 0.02 || Math.abs(dec - 0.5) < 0.02 || Math.abs(dec - 1) < 0.02) {
                            return val.toFixed(1);
                        }
                        return '';
                    }
                },
                splitLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.35)' } } // Opacidad calibrada a 0.35 para rejillas densas de alta definición
            },
            yAxis3D: {
                type: 'value',
                name: 'TIEMPO BARRIDO: 0=NUEVO ──▶ X ANTIGUO)',
                nameGap: 30,
                nameTextStyle: { color: 'var(--cyan)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.85)' } },
                axisLabel: {
                    show: true,
                    color: '#e0e0e0',
                    fontFamily: 'monospace',
                    formatter: (val) => {
                        if (val === 0) return 'NUEVO (0)';
                        if (val === 45) return 'ANTIGUO (45)';
                        return val;
                    }
                },
                splitLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.7)' } }
            },
            zAxis3D: {
                type: 'value',
                name: 'NIVEL',
                interval: 5, // Cuadrícula de alta definición vertical cada 5 dB
                nameTextStyle: { color: 'var(--alert-red)', fontFamily: 'monospace', fontSize: 10 },
                axisLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.85)' } },
                axisLabel: { color: '#e0e0e0', fontFamily: 'monospace', formatter: '{value} dB' },
                splitLine: { lineStyle: { color: 'rgba(255, 255, 255, 0.45)' } } // Opacidad optimizada para rejilla densa
            },
            grid3D: {
                boxWidth: 100,
                boxDepth: 60,
                boxHeight: 35,
                viewControl: {
                    projection: 'perspective',
                    autoRotate: false,
                    distance: 160, // Zoom ligeramente más alejado
                    alpha: 35,
                    beta: 45 // Perspectiva invertida: Frecuencia a la izquierda, Tiempo a la derecha
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

    // --- MANEJADORES DE CONEXIÓN Y EVENTOS WS ---

    function startScanning() {
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
        const token = document.cookie.match(/jwt=([^;]+)/)?.[1] || '';
        const wsUrl = `${wsProto}//${window.location.hostname}${wsPort}/ws/?token=${token}`;
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

                // Control táctico de botones
                startBtn.disabled = true;
                stopBtn.disabled = false;
            }, 200);
        };

        ws.onmessage = (e) => {
            const msg = JSON.parse(e.data);

            if (msg.type === 'delta_frame' && msg.sweep) {
                const count = msg.sweep.length;
                const fmin = window._cal_fmin || 0;
                const fmax = window._cal_fmax || 0;
                const step = (count > 1) ? (fmax - fmin) / (count - 1) : 0;

                // --- Formateo de Consola Táctica en Bloque ---
                const timeStr = new Date().toLocaleTimeString();
                let logLines = [];
                logLines.push(`[${timeStr}] DATA -> ${count} bins recibidos.`);

                let hasAlert = false;
                let maxLevel = -150;
                let minLevel = 0;

                const newPoints = msg.sweep.map((lv, i) => {
                    const freq = (fmin + i * step) / 1e6;
                    logLines.push(`[${timeStr}]   [Bin ${i}] ${freq.toFixed(3)} MHz  -->  ${lv.toFixed(1)} dB`);
                    if (lv > currentThreshold) hasAlert = true;
                    if (lv > maxLevel) maxLevel = lv;
                    if (lv < minLevel) minLevel = lv;
                    return [freq, 0, lv]; // [Frecuencia, Tiempo, Nivel]
                });

                logSweepBlock(logLines.join("\n"), "#39ff14");

                // --- Desplazamiento Geométrico de Cascada 3D ---
                historyBuffer.forEach(sweepPoints => {
                    sweepPoints.forEach(pt => {
                        pt[1] += 1;
                    });
                });

                historyBuffer.unshift(newPoints);
                if (historyBuffer.length > MAX_SWEEPS) {
                    historyBuffer.pop();
                }

                let flatData = [];
                historyBuffer.forEach(sweepPoints => {
                    flatData.push(...sweepPoints);
                });

                // --- Actualización de Geometría en GPU ---
                // ¡RECOMENDADO!: Al llamar a setOption solo una vez al recibir tramas (cada 200ms),
                // el canvas WebGL se actualiza limpiamente sin bloquear las rutinas internas
                // de interacción por ratón de ECharts GL, desbloqueando Zoom y Rotación 3D fluidas.
                if (chart) {
                    // Calibrar la escala en el primer barrido del flujo (Estable y bloqueado)
                    if (sweepCounter === 0) {
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
                const avgLevel = Math.round(msg.sweep.reduce((sum, v) => sum + v, 0) / count);
                const sliderMin = avgLevel - 50;
                const sliderMax = avgLevel + 50;

                if (parseInt(thresholdSlider.min) !== sliderMin || parseInt(thresholdSlider.max) !== sliderMax) {
                    thresholdSlider.min = sliderMin;
                    thresholdSlider.max = sliderMax;
                    if (sweepCounter === 0) {
                        thresholdSlider.value = Math.min(sliderMax, Math.max(sliderMin, avgLevel + 15));
                    } else {
                        thresholdSlider.value = Math.min(sliderMax, Math.max(sliderMin, currentThreshold));
                    }
                    currentThreshold = parseInt(thresholdSlider.value);
                    thresholdValue.textContent = `${currentThreshold} dBµV`;
                }

                sweepCounter++;
            } else if (msg.type === 'init_frame') {
                window._cal_fmin = msg.fmin;
                window._cal_fmax = msg.fmax;

                const fminM = msg.fmin / 1e6;
                const fmaxM = msg.fmax / 1e6;

                // RANGO DE FRECUENCIA completo
                centerFreq.textContent = `${fminM.toFixed(2)} - ${fmaxM.toFixed(2)} MHz`;
                macId.textContent = "ANT-ARGUS-PRO";

                logSweepBlock(`INIT -> Rango: ${fminM.toFixed(2)} - ${fmaxM.toFixed(2)} MHz`, "var(--cyan)");

                // Calcular intervalo de cuadrícula y etiquetas dinámicamente según el tamaño del rango
                const rangeMHz = fmaxM - fminM;
                let dynamicInterval = 0.2;
                let stepLabel = 0.5;
                let decimals = 1;

                if (rangeMHz <= 0.5) {
                    dynamicInterval = 0.01; // Cuadrícula técnica cada 10 kHz (0.01 MHz)
                    stepLabel = 0.05;       // Escribir etiqueta cada 50 kHz
                    decimals = 3;
                } else if (rangeMHz <= 2.0) {
                    dynamicInterval = 0.05; // Cuadrícula técnica cada 50 kHz (0.05 MHz)
                    stepLabel = 0.2;        // Escribir etiqueta cada 200 kHz
                    decimals = 2;
                } else if (rangeMHz <= 5.0) {
                    dynamicInterval = 0.1;  // Cuadrícula técnica cada 100 kHz (0.1 MHz)
                    stepLabel = 0.5;        // Escribir etiqueta cada 500 kHz
                    decimals = 2;
                } else if (rangeMHz <= 15.0) {
                    dynamicInterval = 0.2;  // Cuadrícula técnica cada 200 kHz (0.2 MHz)
                    stepLabel = 1.0;        // Escribir etiqueta cada 1 MHz
                    decimals = 1;
                } else {
                    dynamicInterval = 0.5;  // Cuadrícula técnica cada 500 kHz (0.5 MHz)
                    stepLabel = 2.0;        // Escribir etiqueta cada 2 MHz
                    decimals = 1;
                }

                chart.setOption({
                    xAxis3D: {
                        min: fminM,
                        max: fmaxM,
                        interval: dynamicInterval,
                        axisLabel: {
                            color: '#e0e0e0',
                            fontFamily: 'monospace',
                            formatter: (val) => {
                                // Garantizar que la frecuencia inicial y final del eje se muestren siempre con total precisión
                                if (Math.abs(val - fminM) < 0.001) return val.toFixed(decimals);
                                if (Math.abs(val - fmaxM) < 0.001) return val.toFixed(decimals);

                                const modVal = val % stepLabel;
                                if (Math.abs(modVal) < (dynamicInterval / 2) || Math.abs(modVal - stepLabel) < (dynamicInterval / 2)) {
                                    return val.toFixed(decimals);
                                }
                                return '';
                            }
                        }
                    }
                });

                // Limpiar buffers
                historyBuffer = [];
                sweepCounter = 0;
            } else {
                if (msg.event === 'error_security') {
                    hwidError.classList.add('active');
                    logSweepBlock("ALERTA DE SEGURIDAD: HWID mismatch", "red");
                } else {
                    logSweepBlock(`SISTEMA: ${msg.type} ${msg.event || ''}`, "#888");
                }
            }
        };

        ws.onclose = () => {
            logSweepBlock("Conexión cerrada.", "yellow");
            statusText.textContent = "STANDBY";
            statusText.parentElement.classList.remove('online');
            statusDot.className = "dot blink";

            // Toggles de botones
            startBtn.disabled = false;
            stopBtn.disabled = true;
            ws = null;
            hudAlert.classList.remove('active');
        };

        ws.onerror = (err) => logSweepBlock(`Error WS: ${err.message}`, "red");
    }

    function stopScanning() {
        if (ws) {
            ws.close();
        }
    }

    startBtn.addEventListener('click', startScanning);
    stopBtn.addEventListener('click', stopScanning);
});
