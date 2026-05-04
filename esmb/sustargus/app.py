import os
import socket
import sqlite3
import datetime
import logging
import threading
import time
import json
from functools import wraps
from flask import (Flask, render_template, request, session,
                   redirect, url_for, jsonify, g, abort, Response)
from werkzeug.security import generate_password_hash, check_password_hash
from apscheduler.schedulers.background import BackgroundScheduler
import csv

app = Flask(__name__)
# Clave fija en env o persistente en archivo
SECRET_KEY_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), 'data', '.secret_key')
os.makedirs(os.path.dirname(SECRET_KEY_FILE), exist_ok=True)
if os.path.exists(SECRET_KEY_FILE):
    with open(SECRET_KEY_FILE, 'rb') as f:
        app.secret_key = f.read()
else:
    key = os.urandom(32)
    with open(SECRET_KEY_FILE, 'wb') as f:
        f.write(key)
    app.secret_key = key

import sys
if getattr(sys, 'frozen', False):
    BASE_DIR = os.path.dirname(sys.executable)
else:
    BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH    = os.path.join(BASE_DIR, 'data', 'sustargus.db')
LOG_PATH   = os.path.join(BASE_DIR, 'logs', 'audit.log')

os.makedirs(os.path.dirname(LOG_PATH), exist_ok=True)

# ─── Logging ────────────────────────────────────────────────
audit_logger = logging.getLogger('audit')
audit_logger.setLevel(logging.INFO)
fh = logging.FileHandler(LOG_PATH, encoding='utf-8')
fh.setFormatter(logging.Formatter('%(asctime)s | IP: %(clientip)s | USER: %(user)s | %(message)s'))
audit_logger.addHandler(fh)

def get_client_ip():
    try:
        # Si estamos fuera de una petición Flask (ej. en un hilo secundario)
        # request lanzará RuntimeError o devolverá False
        from flask import request
        if not request: return "SYSTEM"
        if request.headers.getlist("X-Forwarded-For"):
            return request.headers.getlist("X-Forwarded-For")[0].split(",")[0].strip()
        return request.remote_addr
    except:
        return "SYSTEM"

def audit_log(user, message):
    audit_logger.info(message, extra={'clientip': get_client_ip(), 'user': user})

# ─── DB ─────────────────────────────────────────────────────
def get_db():
    db = getattr(g, '_database', None)
    if db is None:
        db = g._database = sqlite3.connect(DB_PATH)
        db.row_factory = sqlite3.Row
        db.execute("PRAGMA foreign_keys = ON")
    return db

@app.teardown_appcontext
def close_connection(exception):
    db = getattr(g, '_database', None)
    if db is not None:
        db.close()

def init_db():
    with app.app_context():
        db = get_db()
        # Verificar si la tabla existe y tiene la restricción antigua
        cursor = db.execute("PRAGMA table_info(users)")
        cols = cursor.fetchall()
        
        # Si la tabla existe, intentamos recrearla para actualizar el CHECK constraint si es necesario
        db.executescript('''
            CREATE TABLE IF NOT EXISTS stations (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                name TEXT NOT NULL,
                ip_esmb TEXT NOT NULL,
                ip_station TEXT NOT NULL,
                output_dir TEXT NOT NULL,
                username TEXT DEFAULT '',
                password TEXT DEFAULT '',
                observations TEXT DEFAULT ''
            );
            CREATE TABLE IF NOT EXISTS recordings (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                station_id INTEGER NOT NULL,
                antenna INTEGER NOT NULL,
                freq_start REAL NOT NULL,
                freq_end   REAL NOT NULL,
                start_time DATETIME NOT NULL,
                end_time   DATETIME NOT NULL,
                output_dir TEXT DEFAULT '',
                status TEXT DEFAULT 'pending',
                FOREIGN KEY(station_id) REFERENCES stations(id) ON DELETE CASCADE
            );
        ''')

        # Especial para USERS: Si falla el insert por el CHECK antiguo, recreamos la tabla
        try:
            db.execute("INSERT OR IGNORE INTO users(username,password,role) VALUES(?,?,?)",
                       ('admin', generate_password_hash('admin'), 'admin'))
            db.execute("INSERT OR IGNORE INTO users(username,password,role) VALUES(?,?,?)",
                       ('gestor', generate_password_hash('gestor'), 'manager'))
            db.execute("INSERT OR IGNORE INTO users(username,password,role) VALUES(?,?,?)",
                       ('visor', generate_password_hash('visor'), 'viewer'))
        except sqlite3.IntegrityError:
            # Si da error de integridad, es el CHECK antiguo. Recreamos la tabla migrando datos.
            print("[!] Migrando tabla de usuarios para nuevos roles...")
            db.executescript('''
                CREATE TABLE users_new (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    username TEXT UNIQUE NOT NULL,
                    password TEXT NOT NULL,
                    role TEXT NOT NULL CHECK(role IN ('admin','manager','viewer'))
                );
                INSERT INTO users_new (id, username, password, role) 
                SELECT id, username, password, 'admin' FROM users;
                DROP TABLE users;
                ALTER TABLE users_new RENAME TO users;
            ''')
            # Reintentar inserts de los nuevos roles
            db.execute("INSERT OR IGNORE INTO users(username,password,role) VALUES(?,?,?)",
                       ('gestor', generate_password_hash('gestor'), 'manager'))
            db.execute("INSERT OR IGNORE INTO users(username,password,role) VALUES(?,?,?)",
                       ('visor', generate_password_hash('visor'), 'viewer'))

        db.commit()

init_db()

# ─── Scheduler de Grabaciones (Solo Gestión de Estados) ─────
# El trabajo pesado lo hará el script recorder.py por separado
def check_recordings_status():
    """Solo actualiza estados si es necesario, pero no lanza grabaciones"""
    with app.app_context():
        db = get_db()
        now = datetime.datetime.now().isoformat()
        # Marcar como 'error' las que pasaron su hora y siguen 'pending'
        db.execute("UPDATE recordings SET status = 'error' WHERE status = 'pending' AND end_time < ?", (now,))
        db.commit()

scheduler = BackgroundScheduler()
scheduler.add_job(check_recordings_status, 'interval', minutes=5)
scheduler.start()

# ─── Decoradores ─────────────────────────────────────────────
def login_required(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        if 'user_id' not in session:
            return redirect(url_for('login'))
        return f(*args, **kwargs)
    return decorated

def admin_required(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        if 'user_id' not in session or session.get('role') != 'admin':
            audit_log(session.get('username', 'anon'), "Acceso denegado a ruta admin.")
            return jsonify({"success": False, "error": "Permisos insuficientes"}), 403
        return f(*args, **kwargs)
    return decorated

def manager_required(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        if 'user_id' not in session or session.get('role') not in ['admin', 'manager']:
            audit_log(session.get('username', 'anon'), "Acceso denegado a ruta de gestión.")
            return jsonify({"success": False, "error": "Permisos insuficientes"}), 403
        return f(*args, **kwargs)
    return decorated

from RsInstrument import *

# ─── ESMB Scanners Management ──────────────────────────────
# Ahora permitimos múltiples escáneres simultáneos (uno por IP de ESMB)
active_scanners = {} # Diccionario indexado por IP
scanners_lock = threading.Lock()

def get_scanner(ip):
    with scanners_lock:
        if ip not in active_scanners:
            active_scanners[ip] = {
                'running': False,
                'owner': None,
                'ip': ip,
                'freq_start': 0,
                'freq_end': 0,
                'step_khz': 100,
                'latest_trace': {'frequencies': [], 'levels': []},
                'error': None,
                'lock': threading.Lock()
            }
        return active_scanners[ip]

def esmb_scan_thread(ip, freq_start, freq_end, step_khz=100.0, owner=None):
    """Hilo que gestiona el escaneo de una IP específica"""
    state = get_scanner(ip)
    
    instr = None
    try:
        # Abrir conexión
        instr = RsInstrument(f'TCPIP::{ip}::5555::SOCKET', True, False)
        instr.visa_timeout = 3000
        
        instr.write("*CLS; ABORT")
        instr.write(":FREQ:MODE SWE")
        instr.write(":TRAC:FEED:CONT MTRACE, ALW")
        instr.write(":STAT:TRAC:ENAB #B10010")
        instr.write(f":FREQ:STAR {freq_start} MHz;STOP {freq_end} MHz")
        instr.write(f":SWE:STEP {step_khz/1000.0} MHz")
        instr.write(":FORM ASC")
    except Exception as e:
        with state['lock']:
            state['error'] = f"Error conexión: {str(e)}"
            state['running'] = False
            state['owner'] = None
        return

    print(f"[*] Escáner iniciado por {owner} en IP {ip}")
    
    last_valid_n = -10.0
    while state['running']:
        try:
            instr.write(":INIT;*WAI")
            time.sleep(0.2)
            raw_data = instr.query(":TRAC? MTRACE")
            
            niveles = [float(x) for x in raw_data.split(',') if x.strip()]
            num_puntos = len(niveles)
            
            if num_puntos > 1:
                paso_real = (freq_end - freq_start) / (num_puntos - 1)
                f_list = []
                v_list = []
                
                for i, n in enumerate(niveles):
                    if n < -50 or n > 150:
                        n = last_valid_n
                    else:
                        last_valid_n = n
                    f_list.append(round(freq_start + (i * paso_real), 6))
                    v_list.append(n)
                
                with state['lock']:
                    state['latest_trace'] = {"frequencies": f_list, "levels": v_list}
                    state['error'] = None
            
            time.sleep(0.05)
            
        except Exception as e:
            with state['lock']:
                state['error'] = f"Error en traza: {str(e)}"
            time.sleep(1)

    if instr:
        try: instr.close()
        except: pass
    
    with state['lock']:
        state['running'] = False
        state['owner'] = None
    print(f"[*] Escáner detenido en IP {ip}")

# ─── Rutas principales ───────────────────────────────────────
@app.route('/login', methods=['GET', 'POST'])
def login():
    if request.method == 'POST':
        username = request.form.get('username', '').strip()
        password = request.form.get('password', '')
        db = get_db()
        user = db.execute("SELECT * FROM users WHERE username=?", (username,)).fetchone()
        if user and check_password_hash(user['password'], password):
            session.clear()
            session['user_id']  = user['id']
            session['username'] = user['username']
            session['role']     = user['role']
            audit_log(user['username'], "Login exitoso.")
            return redirect(url_for('dashboard') if user['role'] == 'admin' else url_for('sentinel'))
        audit_log('anon', f"Login fallido para: {username}")
        return render_template('login.html', error="Credenciales inválidas")
    return render_template('login.html')

@app.route('/logout')
def logout():
    audit_log(session.get('username', 'anon'), "Logout.")
    session.clear()
    return redirect(url_for('login'))

@app.route('/')
@login_required
def index():
    return redirect(url_for('dashboard') if session.get('role') == 'admin' else url_for('sentinel'))

@app.route('/sentinel/publico')
def sentinel_publico():
    """Acceso visor sin login - modo solo lectura completo."""
    db = get_db()
    stations = db.execute("SELECT id, name, ip_esmb FROM stations").fetchall()
    audit_log('visitante', "Acceso al visor público.")
    
    # Otorgar sesión válida para que puedan usar el escáner sin restricciones
    session['user_id'] = 0
    session['username'] = 'Visitante'
    session['role'] = 'guest'
    
    return render_template('sentinel.html', stations=stations, role='guest', username='Visitante')

@app.route('/dashboard')
@login_required
def dashboard():
    db = get_db()
    stations    = db.execute("SELECT * FROM stations").fetchall()
    recordings  = db.execute('''
        SELECT r.*, s.name as station_name
        FROM recordings r JOIN stations s ON r.station_id = s.id
        ORDER BY r.start_time DESC
    ''').fetchall()
    return render_template('dashboard.html', 
                           stations=stations, 
                           recordings=recordings,
                           role=session.get('role'),
                           username=session.get('username'))

@app.route('/sentinel')
@login_required
def sentinel():
    db = get_db()
    stations = db.execute("SELECT id, name, ip_esmb FROM stations").fetchall()
    audit_log(session.get('username','?'), "Acceso al visor Sentinel.")
    return render_template('sentinel.html', stations=stations,
                           role=session.get('role'), username=session.get('username'))

# ─── API Usuarios (Admin Only) ──────────────────────────────
@app.route('/api/users', methods=['GET'])
@admin_required
def list_users():
    db = get_db()
    rows = db.execute("SELECT id, username, role FROM users").fetchall()
    return jsonify([dict(r) for r in rows])

@app.route('/api/users', methods=['POST'])
@admin_required
def add_user():
    d = request.json
    db = get_db()
    try:
        db.execute("INSERT INTO users(username, password, role) VALUES(?,?,?)",
                   (d['username'], generate_password_hash(d['password']), d['role']))
        db.commit()
        audit_log(session['username'], f"Usuario creado: {d['username']}")
        return jsonify({"success": True})
    except sqlite3.IntegrityError:
        return jsonify({"success": False, "error": "El usuario ya existe"}), 400

@app.route('/api/users/<int:uid>', methods=['PUT'])
@admin_required
def update_user(uid):
    d = request.json
    db = get_db()
    if d.get('password'):
        db.execute("UPDATE users SET role=?, password=? WHERE id=?",
                   (d['role'], generate_password_hash(d['password']), uid))
    else:
        db.execute("UPDATE users SET role=? WHERE id=?", (d['role'], uid))
    db.commit()
    return jsonify({"success": True})

@app.route('/api/users/<int:uid>', methods=['DELETE'])
@admin_required
def delete_user(uid):
    if uid == session['user_id']:
        return jsonify({"success": False, "error": "No puedes borrarte a ti mismo"}), 400
    db = get_db()
    db.execute("DELETE FROM users WHERE id=?", (uid,))
    db.commit()
    return jsonify({"success": True})

# ─── API Estaciones ──────────────────────────────────────────
@app.route('/api/stations', methods=['GET'])
@login_required
def list_stations():
    db = get_db()
    rows = db.execute("SELECT * FROM stations").fetchall()
    return jsonify([dict(r) for r in rows])

@app.route('/api/stations', methods=['POST'])
@admin_required
def add_station():
    d = request.json
    db = get_db()
    cur = db.execute('''INSERT INTO stations(name,ip_esmb,ip_station,output_dir,username,password,observations)
                        VALUES(?,?,?,?,?,?,?)''',
                     (d['name'], d['ip_esmb'], d['ip_station'], d['output_dir'],
                      d.get('username',''), d.get('password',''), d.get('observations','')))
    db.commit()
    audit_log(session['username'], f"Estación añadida: {d['name']}")
    return jsonify({"success": True, "id": cur.lastrowid})

@app.route('/api/stations/<int:sid>', methods=['GET'])
@admin_required
def get_station(sid):
    db = get_db()
    row = db.execute("SELECT * FROM stations WHERE id=?", (sid,)).fetchone()
    if not row:
        abort(404)
    return jsonify(dict(row))

@app.route('/api/stations/<int:sid>', methods=['PUT'])
@admin_required
def update_station(sid):
    d = request.json
    db = get_db()
    db.execute('''UPDATE stations SET name=?,ip_esmb=?,ip_station=?,output_dir=?,
                  username=?,password=?,observations=? WHERE id=?''',
               (d['name'], d['ip_esmb'], d['ip_station'], d['output_dir'],
                d.get('username',''), d.get('password',''), d.get('observations',''), sid))
    db.commit()
    audit_log(session['username'], f"Estación editada ID:{sid}")
    return jsonify({"success": True})

@app.route('/api/stations/<int:sid>', methods=['DELETE'])
@admin_required
def delete_station(sid):
    db = get_db()
    db.execute("DELETE FROM stations WHERE id=?", (sid,))
    db.commit()
    audit_log(session['username'], f"Estación borrada ID:{sid}")
    return jsonify({"success": True})

# ─── API Grabaciones ─────────────────────────────────────────
@app.route('/api/recordings', methods=['GET'])
@admin_required
def list_recordings():
    db = get_db()
    rows = db.execute('''SELECT r.*, s.name as station_name FROM recordings r
                         JOIN stations s ON r.station_id=s.id ORDER BY r.start_time DESC''').fetchall()
    return jsonify([dict(r) for r in rows])

@app.route('/api/recordings', methods=['POST'])
@manager_required
def add_recording():
    d = request.json
    db = get_db()
    
    # 1. Validar solapamientos (Misma estación, mismo rango de tiempo)
    overlap = db.execute('''
        SELECT id FROM recordings 
        WHERE station_id = ? 
        AND status != 'done'
        AND (
            (start_time <= ? AND end_time >= ?) OR 
            (start_time <= ? AND end_time >= ?) OR
            (? <= start_time AND ? >= end_time)
        )
    ''', (d['station_id'], d['start_time'], d['start_time'], d['end_time'], d['end_time'], d['start_time'], d['end_time'])).fetchone()
    
    if overlap:
        return jsonify({"success": False, "error": "Ya existe una grabación programada en ese horario para esta estación."}), 400

    cur = db.execute('''INSERT INTO recordings(station_id,antenna,freq_start,freq_end,start_time,end_time,output_dir)
                        VALUES(?,?,?,?,?,?,?)''',
                     (d['station_id'], d['antenna'], d['freq_start'], d['freq_end'],
                      d['start_time'], d['end_time'], d.get('output_dir','')))
    db.commit()
    audit_log(session['username'], f"Grabación programada estación {d['station_id']}")
    return jsonify({"success": True, "id": cur.lastrowid})

@app.route('/api/recordings/<int:rid>', methods=['GET'])
@admin_required
def get_recording(rid):
    db = get_db()
    row = db.execute("SELECT * FROM recordings WHERE id=?", (rid,)).fetchone()
    if not row: abort(404)
    return jsonify(dict(row))

@app.route('/api/recordings/<int:rid>', methods=['PUT'])
@manager_required
def update_recording(rid):
    d = request.json
    db = get_db()
    db.execute('''UPDATE recordings SET station_id=?, antenna=?, freq_start=?, freq_end=?, 
                  start_time=?, end_time=?, output_dir=? WHERE id=?''',
               (d['station_id'], d['antenna'], d['freq_start'], d['freq_end'],
                d['start_time'], d['end_time'], d.get('output_dir',''), rid))
    db.commit()
    audit_log(session['username'], f"Grabación editada ID:{rid}")
    return jsonify({"success": True})

@app.route('/api/recordings/<int:rid>', methods=['DELETE'])
@manager_required
def delete_recording(rid):
    db = get_db()
    db.execute("DELETE FROM recordings WHERE id=?", (rid,))
    db.commit()
    audit_log(session['username'], f"Grabación borrada ID:{rid}")
    return jsonify({"success": True})

# ─── API Scanner ESMB (real) ─────────────────────────────────
@app.route('/api/esmb/scan/start', methods=['POST'])
@login_required
def scan_start():
    d = request.json
    ip         = d.get('ip_esmb')
    freq_start = float(d.get('freq_start', 100.0))
    freq_end   = float(d.get('freq_end',   105.0))
    step_khz   = float(d.get('step_khz',   100.0))
    username   = session.get('username', 'Visitante')

    if not ip:
        return jsonify({"success": False, "error": "IP requerida"}), 400

    state = get_scanner(ip)
    with state['lock']:
        if state['running']:
            return jsonify({"success": False, "error": f"La estación ya está siendo usada por {state['owner']}"}), 409

        state['running']    = True
        state['owner']      = username
        state['freq_start'] = freq_start
        state['freq_end']   = freq_end
        state['step_khz']   = step_khz
        state['latest_trace'] = {'frequencies': [], 'levels': []}
        state['error']      = None

    t = threading.Thread(target=esmb_scan_thread, args=(ip, freq_start, freq_end, step_khz, username))
    t.daemon = True
    t.start()

    audit_log(username, f"Escáner iniciado en {ip} ({freq_start}-{freq_end} MHz)")
    return jsonify({"success": True})
    return jsonify({"success": True})

@app.route('/api/esmb/scan/stop', methods=['POST'])
@login_required
def scan_stop():
    ip = request.json.get('ip_esmb')
    if not ip: return jsonify({"success": False, "error": "IP requerida"}), 400
    
    state = get_scanner(ip)
    username = session.get('username')
    
    with state['lock']:
        # Permitir detener si es el dueño O si es admin
        if not state['running']:
            return jsonify({"success": True})
            
        if state['owner'] != username and session.get('role') != 'admin':
            return jsonify({"success": False, "error": f"No puedes detener este escáner, pertenece a {state['owner']}"}), 403
            
        state['running'] = False
        
    audit_log(username, f"Escáner detenido en {ip}")
    return jsonify({"success": True})

@app.route('/api/esmb/data')
@login_required
def esmb_data():
    """Endpoint de polling AJAX por IP"""
    ip = request.args.get('ip')
    if not ip: return jsonify({"error": "IP requerida"}), 400
    
    state = get_scanner(ip)
    with state['lock']:
        return jsonify({
            "is_running": state["running"],
            "owner": state["owner"],
            "freq_start": state["freq_start"],
            "freq_end": state["freq_end"],
            "step_khz": state["step_khz"],
            "trace": state["latest_trace"],
            "error": state["error"],
            "current_user": session.get('username')
        })

if __name__ == '__main__':
    try:
        from waitress import serve
        print("[+] SustArgus iniciado en http://0.0.0.0:8000")
        serve(app, host='0.0.0.0', port=8000)
    except ImportError:
        app.run(host='0.0.0.0', port=8000, debug=False)
