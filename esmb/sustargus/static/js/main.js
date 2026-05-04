// --- Utilidades Modales ---
function toggleStationModal() {
    const modal = document.getElementById('stationModal');
    if (modal) {
        modal.classList.toggle('show');
    }
}

// --- Dashboard (Admin) ---
if (document.getElementById('stationForm')) {
    document.getElementById('stationForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        const data = {
            name: document.getElementById('stName').value,
            ip_esmb: document.getElementById('stIpEsmb').value,
            ip_station: document.getElementById('stIpStation').value,
            output_dir: document.getElementById('stOutputDir').value,
            username: document.getElementById('stUser').value,
            password: document.getElementById('stPass').value,
            observations: document.getElementById('stObs').value
        };

        try {
            const res = await fetch('/api/stations', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
            if (res.ok) {
                window.location.reload();
            } else {
                alert("Error al guardar estación");
            }
        } catch (error) {
            console.error(error);
            alert("Error de conexión");
        }
    });
}

async function deleteStation(id) {
    if (confirm("¿Seguro que deseas eliminar esta estación?")) {
        try {
            const res = await fetch(`/api/stations/${id}`, { method: 'DELETE' });
            if (res.ok) {
                window.location.reload();
            }
        } catch(e) {
            console.error(e);
        }
    }
}

if (document.getElementById('recordForm')) {
    document.getElementById('recordForm').addEventListener('submit', async (e) => {
        e.preventDefault();
        const data = {
            station_id: document.getElementById('stationSelect').value,
            antenna: document.getElementById('antennaSelect').value,
            frequency: document.getElementById('freqInput').value,
            start_time: document.getElementById('startTime').value,
            end_time: document.getElementById('endTime').value
        };

        try {
            const res = await fetch('/api/recordings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(data)
            });
            if (res.ok) {
                alert("Grabación programada exitosamente.");
                document.getElementById('recordForm').reset();
            } else {
                alert("Error al programar grabación");
            }
        } catch (error) {
            console.error(error);
        }
    });
}

// --- Sentinel Viewer (Invitado/Admin) ---
let isScanning = false;
let scanInterval;

function toggleScan(start) {
    const stationIp = document.getElementById('viewerStationSelect')?.value;
    if (!stationIp) {
        alert("Selecciona una estación primero.");
        return;
    }

    isScanning = start;
    document.getElementById('btnStartScan').style.display = start ? 'none' : 'block';
    document.getElementById('btnStopScan').style.display = start ? 'block' : 'none';
    
    const statusLed = document.querySelector('.led');
    const statusText = document.querySelector('#scanStatus span');
    
    if (start) {
        statusLed.classList.remove('led-off');
        statusLed.classList.add('led-on');
        statusText.innerText = 'Escaneando...';
        initCharts(); // Initialize empty charts
        scanInterval = setInterval(simulateData, 1000); // Simulate incoming data
        
        // Avisar al backend
        fetch('/api/esmb/scan', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ ip_esmb: stationIp, action: 'start' })
        });
    } else {
        statusLed.classList.add('led-off');
        statusLed.classList.remove('led-on');
        statusText.innerText = 'Desconectado';
        clearInterval(scanInterval);
        
        fetch('/api/esmb/scan', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({ ip_esmb: stationIp, action: 'stop' })
        });
    }
}

function switchTab(tabId) {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.chart-container').forEach(c => c.classList.remove('active'));
    document.querySelectorAll('.chart-container').forEach(c => c.style.display = 'none');
    
    event.target.classList.add('active');
    const container = document.getElementById(tabId === '3d' ? 'chart3D' : 'chart2D');
    container.style.display = 'block';
    container.classList.add('active');
    
    // Resize plotly chart to fit new container
    if (window.Plotly) {
        Plotly.Plots.resize(container);
    }
}

// -- Simulación de Gráficos --
let zData = [];
function initCharts() {
    if (!document.getElementById('chart3D')) return;
    
    const layout3D = {
        title: 'Cascada Espectral 3D',
        autosize: true,
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor: 'rgba(0,0,0,0)',
        font: { color: '#e2e8f0' },
        scene: {
            xaxis: { title: 'Frecuencia (MHz)' },
            yaxis: { title: 'Tiempo' },
            zaxis: { title: 'Potencia (dBm)' },
            bgcolor: 'rgba(0,0,0,0)'
        },
        margin: { l: 0, r: 0, b: 0, t: 30 }
    };
    
    Plotly.newPlot('chart3D', [{
        z: [[0]],
        type: 'surface',
        colorscale: 'Viridis'
    }], layout3D);

    const layout2D = {
        title: 'Espectro en Tiempo Real',
        paper_bgcolor: 'rgba(0,0,0,0)',
        plot_bgcolor: 'rgba(0,0,0,0)',
        font: { color: '#e2e8f0' },
        xaxis: { title: 'Frecuencia (MHz)' },
        yaxis: { title: 'Potencia (dBm)' },
        margin: { l: 40, r: 20, b: 40, t: 30 }
    };
    
    Plotly.newPlot('chart2D', [{
        x: [100, 101, 102, 103, 104, 105],
        y: [-100, -100, -100, -100, -100, -100],
        type: 'scatter',
        line: { color: '#10b981' },
        fill: 'tozeroy'
    }], layout2D);
}

function simulateData() {
    const freqStart = parseFloat(document.getElementById('freqStart').value);
    const freqEnd = parseFloat(document.getElementById('freqEnd').value);
    
    const x = [];
    const y = [];
    for(let i=freqStart; i<=freqEnd; i+=0.1) {
        x.push(i);
        // Generar ruido aleatorio con un par de picos
        let val = -110 + Math.random() * 20;
        if (Math.abs(i - ((freqEnd+freqStart)/2)) < 0.2) val += 50; // Pico central simulado
        y.push(val);
    }
    
    if (zData.length > 20) zData.shift(); // Mantener historial
    zData.push(y);
    
    if(document.getElementById('chart2D').style.display !== 'none') {
        Plotly.update('chart2D', {x: [x], y: [y]});
    }
    
    if(document.getElementById('chart3D').style.display !== 'none') {
        Plotly.update('chart3D', {z: [zData]});
    }
}
