import time
import threading
from flask import Flask, render_template, request, jsonify
from RsInstrument import *

import os
import sys

if getattr(sys, 'frozen', False):
    template_folder = os.path.join(sys._MEIPASS, 'templates')
    static_folder = os.path.join(sys._MEIPASS, 'static')
    app = Flask(__name__, template_folder=template_folder, static_folder=static_folder)
else:
    app = Flask(__name__)

scan_state = {
    "is_running": False,
    "freq_inicio": 100.0,
    "freq_fin": 105.0,
    "paso": 0.1,
    "ip_esmb": "192.168.29.22",
    "latest_trace": {"frequencies": [], "levels": []},
    "error": None
}

def scan_worker():
    global scan_state
    
    instr = None
    try:
        instr = RsInstrument(f'TCPIP::{scan_state["ip_esmb"]}::5555::SOCKET', True, False)
        instr.visa_timeout = 3000
        
        instr.write("*CLS; ABORT")
        instr.write(":FREQ:MODE SWE")
        instr.write(":TRAC:FEED:CONT MTRACE, ALW")
        instr.write(":STAT:TRAC:ENAB #B10010")
        instr.write(f":FREQ:STAR {scan_state['freq_inicio']} MHz;STOP {scan_state['freq_fin']} MHz")
        # El paso interno podría ser ignorado por el ESMB, pero lo mandamos
        instr.write(f":SWE:STEP {scan_state['paso']} MHz")
        instr.write(":FORM ASC")
    except Exception as e:
        scan_state["error"] = f"Error de conexión: {str(e)}"
        scan_state["is_running"] = False
        return

    while scan_state["is_running"]:
        try:
            # Forzamos barrido
            instr.write(":INIT;*WAI")
            time.sleep(0.3)
            
            raw_data = instr.query(":TRAC? MTRACE")
            niveles = [float(x) for x in raw_data.split(',') if x.strip()]
            
            num_puntos = len(niveles)
            if num_puntos > 1:
                paso_real = (scan_state['freq_fin'] - scan_state['freq_inicio']) / (num_puntos - 1)
                
                f_list = []
                v_list = []
                last_valid = -10.0 # Valor base por si el primero falla
                
                for i, n in enumerate(niveles):
                    # Filtrar picos raros (NaN, valores extremos que devuelve el equipo cuando no está listo)
                    if n < -50 or n > 150:
                        n = last_valid # Usar el último valor válido en lugar de saltarlo para no romper la matriz 3D
                    else:
                        last_valid = n
                        
                    f_list.append(round(scan_state['freq_inicio'] + (i * paso_real), 6))
                    v_list.append(n)
                
                # Actualizar datos globales para la web
                scan_state["latest_trace"] = {
                    "frequencies": f_list,
                    "levels": v_list
                }
                scan_state["error"] = None
            
            time.sleep(0.05)
            
        except Exception as e:
            scan_state["error"] = f"Error leyendo traza: {str(e)}"
            time.sleep(1)

    if instr:
        try:
            instr.close()
        except: pass

@app.route('/')
def index():
    return render_template('index.html')

@app.route('/api/start', methods=['POST'])
def start_scan():
    global scan_state
    if scan_state["is_running"]:
        return jsonify({"success": False, "error": "El escáner ya está corriendo"})
    
    data = request.json
    scan_state['freq_inicio'] = float(data.get('freq_inicio', 101.0))
    scan_state['freq_fin'] = float(data.get('freq_fin', 103.0))
    scan_state['ip_esmb'] = data.get('ip', '192.168.29.22')
    scan_state['is_running'] = True
    scan_state['error'] = None
    scan_state['latest_trace'] = {"frequencies": [], "levels": []}
    
    t = threading.Thread(target=scan_worker)
    t.daemon = True
    t.start()
    
    return jsonify({"success": True})

@app.route('/api/stop', methods=['POST'])
def stop_scan():
    global scan_state
    scan_state["is_running"] = False
    return jsonify({"success": True})

@app.route('/api/data', methods=['GET'])
def get_data():
    return jsonify({
        "is_running": scan_state["is_running"],
        "trace": scan_state["latest_trace"],
        "error": scan_state["error"]
    })

if __name__ == '__main__':
    print("[+] Sentinel ESMB Web Iniciado en http://0.0.0.0:5010")
    try:
        from waitress import serve
        serve(app, host='0.0.0.0', port=5010)
    except ImportError:
        app.run(host='0.0.0.0', port=5010, debug=True)
