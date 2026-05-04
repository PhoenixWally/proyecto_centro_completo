document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('scan-form');
    const btnStart = document.getElementById('btn-start');
    const btnStop = document.getElementById('btn-stop');
    const statusPanel = document.getElementById('status-panel');
    const valCsv = document.getElementById('val-csv');
    const valTime = document.getElementById('val-time');
    const progressFill = document.getElementById('progress-fill');
    const statusIndicator = document.getElementById('status-indicator');
    
    let statusInterval = null;
    let totalDuration = 0;

    // Check initial status
    checkStatus();

    form.addEventListener('submit', async (e) => {
        e.preventDefault();
        
        const freq_inicio = parseFloat(document.getElementById('freq_inicio').value);
        const freq_fin = parseFloat(document.getElementById('freq_fin').value);
        const paso = parseFloat(document.getElementById('paso').value);
        const dias = parseInt(document.getElementById('dias').value) || 0;
        const horas = parseInt(document.getElementById('horas').value) || 0;
        
        totalDuration = (dias * 86400) + (horas * 3600);
        
        try {
            const response = await fetch('/api/start', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ freq_inicio, freq_fin, paso, dias, horas })
            });
            
            const data = await response.json();
            
            if (response.ok) {
                setRunningState(true);
                valCsv.textContent = data.csv;
                startStatusPolling();
            } else {
                alert(data.error || 'Error al iniciar escaneo');
            }
        } catch (error) {
            console.error('Error:', error);
            alert('Error de conexión con el servidor');
        }
    });

    btnStop.addEventListener('click', async () => {
        try {
            const response = await fetch('/api/stop', { method: 'POST' });
            if (response.ok) {
                setRunningState(false);
            }
        } catch (error) {
            console.error('Error:', error);
        }
    });

    function setRunningState(isRunning) {
        btnStart.disabled = isRunning;
        btnStop.disabled = !isRunning;
        
        const inputs = form.querySelectorAll('input');
        inputs.forEach(input => input.disabled = isRunning);
        
        if (isRunning) {
            statusPanel.classList.remove('hidden');
            statusIndicator.classList.add('active');
        } else {
            stopStatusPolling();
            statusIndicator.classList.remove('active');
            valTime.textContent = '00:00:00';
            progressFill.style.width = '100%';
            setTimeout(() => {
                if(!btnStart.disabled) { // If still stopped
                    statusPanel.classList.add('hidden');
                }
            }, 3000); // Hide panel after 3 seconds of stopping
        }
    }

    async function checkStatus() {
        try {
            const response = await fetch('/api/status');
            const data = await response.json();
            
            if (data.is_running) {
                setRunningState(true);
                valCsv.textContent = data.csv_filename;
                
                // Calculate total duration from endpoints if not set locally
                const start = new Date(data.start_time);
                const end = new Date(data.end_time);
                totalDuration = (end - start) / 1000;
                
                updateStatusUI(data.remaining_seconds);
                startStatusPolling();
            } else {
                setRunningState(false);
            }
        } catch (error) {
            console.error('Error checking status:', error);
        }
    }

    function startStatusPolling() {
        if (statusInterval) clearInterval(statusInterval);
        statusInterval = setInterval(async () => {
            try {
                const response = await fetch('/api/status');
                const data = await response.json();
                
                if (data.is_running) {
                    updateStatusUI(data.remaining_seconds);
                } else {
                    setRunningState(false);
                }
            } catch (error) {
                console.error('Polling error:', error);
            }
        }, 1000);
    }

    function stopStatusPolling() {
        if (statusInterval) {
            clearInterval(statusInterval);
            statusInterval = null;
        }
    }

    function updateStatusUI(remainingSeconds) {
        // Format time
        const h = Math.floor(remainingSeconds / 3600);
        const m = Math.floor((remainingSeconds % 3600) / 60);
        const s = Math.floor(remainingSeconds % 60);
        
        valTime.textContent = `${h.toString().padStart(2, '0')}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
        
        // Update progress
        if (totalDuration > 0) {
            const elapsed = totalDuration - remainingSeconds;
            const percentage = Math.min(100, Math.max(0, (elapsed / totalDuration) * 100));
            progressFill.style.width = `${percentage}%`;
        }
    }
});
