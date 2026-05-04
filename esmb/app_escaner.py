import threading
import time
import csv
import os
from datetime import datetime, timedelta
from flask import Flask, request, jsonify, render_template

# Import RsInstrument safely
try:
    from RsInstrument import *
    HAS_RSINSTRUMENT = True
except ImportError:
    HAS_RSINSTRUMENT = False

app = Flask(__name__)

# Global state to keep track of the current scan
scan_state = {
    "is_running": False,
    "start_time": None,
    "end_time": None,
    "freq_inicio": 0,
    "freq_fin": 0,
    "paso": 0,
    "csv_filename": ""
}

def scan_worker(freq_inicio, freq_fin, paso, duration_seconds, csv_filename):
    global scan_state
    
    # Initialize connection
    instr = None
    if HAS_RSINSTRUMENT:
        try:
            instr = RsInstrument('TCPIP::192.168.29.102::5555::SOCKET', True, False)
            instr.visa_timeout = 2000
            instr.write("*CLS; ABORT")
            instr.write("INIT:CONT ON")
        except Exception as e:
            print(f"Error connecting to instrument: {e}")
            scan_state["is_running"] = False
            return
            
    # Open CSV
    with open(csv_filename, mode='w', newline='') as file:
        writer = csv.writer(file)
        writer.writerow(["Fecha", "Hora", "Frecuencia (MHz)", "Nivel (dBuV)"])
        
        if instr:
            try:
                # CONFIGURACIÓN MODO ULTRA-RÁPIDO (ARGUS STYLE)
                instr.write(":FREQ:MODE SWE")
                instr.write(":TRAC:FEED:CONT MTRACE, ALW")
                instr.write(":STAT:TRAC:ENAB #B10010")
                instr.write(f":FREQ:STAR {freq_inicio} MHz;STOP {freq_fin} MHz")
                instr.write(f":SWE:STEP {paso} MHz")
                instr.write(":FORM ASC")
                instr.write(":INIT")
                # Pequeña espera inicial para que el hardware llene el primer buffer
                time.sleep(0.5)
            except Exception as e:
                print(f"Error configurando modo rápido: {e}")

        start_t = time.time()
        end_t = start_t + duration_seconds
        
        while time.time() < end_t and scan_state["is_running"]:
            if instr:
                try:
                    # Forzamos un nuevo barrido antes de pedir los datos
                    # El *WAI asegura que el equipo termine el sweep antes de continuar
                    instr.write(":INIT;*WAI")
                    
                    # Espera segura para que el equipo empaquete los datos en MTRACE
                    time.sleep(0.5) 
                    
                    # Pedimos el bloque completo
                    raw_data = instr.query(":TRAC? MTRACE")
                    niveles = [float(x) for x in raw_data.split(',') if x.strip()]
                    num_puntos = len(niveles)
                    
                    if num_puntos > 1:
                        now = datetime.now()
                        fecha_str = now.strftime("%Y-%m-%d")
                        hora_str = now.strftime("%H:%M:%S.%f")[:-3]
                        
                        # CALCULAMOS EL PASO REAL
                        paso_real = (freq_fin - freq_inicio) / (num_puntos - 1)
                        
                        for i, nivel in enumerate(niveles):
                            if nivel < -9e36: continue
                            
                            f_actual = round(freq_inicio + (i * paso_real), 6)
                            writer.writerow([fecha_str, hora_str, f_actual, nivel])
                        
                        file.flush()
                    
                    # Un pequeño respiro antes del siguiente ciclo continuo
                    time.sleep(0.1)
                    
                except Exception as e:
                    print(f"Error en ráfaga de datos: {e}")
                    time.sleep(1)
            else:
                # Simulación para pruebas sin equipo
                import random
                now = datetime.now()
                fecha_str = now.strftime("%Y-%m-%d")
                hora_str = now.strftime("%H:%M:%S.%f")[:-3]
                
                f_actual = freq_inicio
                while f_actual <= freq_fin:
                    nivel = round(random.uniform(10, 80), 2)
                    writer.writerow([fecha_str, hora_str, f_actual, nivel])
                    f_actual = round(f_actual + paso, 4)
                
                file.flush()
                time.sleep(1)

    if instr:
        try:
            instr.close()
        except:
            pass
    
    scan_state["is_running"] = False

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/api/start', methods=['POST'])
def start_scan():
    global scan_state
    if scan_state["is_running"]:
        return jsonify({"error": "El escaneo ya está en curso"}), 400
        
    data = request.json
    try:
        freq_inicio = float(data.get('freq_inicio'))
        freq_fin = float(data.get('freq_fin'))
        paso = float(data.get('paso'))
        dias = int(data.get('dias', 0))
        horas = int(data.get('horas', 0))
        
        if freq_inicio >= freq_fin or paso <= 0:
            return jsonify({"error": "Rango de frecuencia o salto inválido"}), 400
            
        duration_seconds = dias * 86400 + horas * 3600
        if duration_seconds <= 0:
            return jsonify({"error": "La duración debe ser mayor a 0"}), 400
            
    except (ValueError, TypeError):
        return jsonify({"error": "Parámetros inválidos"}), 400

    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    csv_filename = f"medidas_{timestamp}.csv"
    
    # Guarda en la misma carpeta del script
    csv_path = os.path.join(os.path.dirname(os.path.abspath(__file__)), csv_filename)
    
    scan_state = {
        "is_running": True,
        "start_time": datetime.now(),
        "end_time": datetime.now() + timedelta(seconds=duration_seconds),
        "freq_inicio": freq_inicio,
        "freq_fin": freq_fin,
        "paso": paso,
        "csv_filename": csv_filename
    }
    
    thread = threading.Thread(target=scan_worker, args=(freq_inicio, freq_fin, paso, duration_seconds, csv_path))
    thread.daemon = True
    thread.start()
    
    return jsonify({"message": "Escaneo iniciado", "csv": csv_filename})

@app.route('/api/stop', methods=['POST'])
def stop_scan():
    global scan_state
    scan_state["is_running"] = False
    return jsonify({"message": "Escaneo detenido"})

@app.route('/api/status', methods=['GET'])
def get_status():
    global scan_state
    if not scan_state["is_running"]:
        return jsonify({"is_running": False})
        
    now = datetime.now()
    remaining = (scan_state["end_time"] - now).total_seconds()
    
    if remaining <= 0:
        scan_state["is_running"] = False
        return jsonify({"is_running": False})
        
    return jsonify({
        "is_running": True,
        "start_time": scan_state["start_time"].strftime("%Y-%m-%d %H:%M:%S"),
        "end_time": scan_state["end_time"].strftime("%Y-%m-%d %H:%M:%S"),
        "remaining_seconds": int(remaining),
        "freq_inicio": scan_state["freq_inicio"],
        "freq_fin": scan_state["freq_fin"],
        "paso": scan_state["paso"],
        "csv_filename": scan_state["csv_filename"]
    })

if __name__ == '__main__':
    print("[+] Servidor Web ESMB iniciado en http://0.0.0.0:5005")
    try:
        from waitress import serve
        serve(app, host='0.0.0.0', port=5005)
    except ImportError:
        app.run(host='0.0.0.0', port=5005, debug=True)
