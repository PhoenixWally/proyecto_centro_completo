import json
import os
import sys

# Anclar el script obligatoriamente a la carpeta donde reside server.py
# (Evita que lea importacion.xlsx de C:\Windows\System32 u otro lugar si se lanza desde otra ruta)
base_dir = os.path.dirname(os.path.abspath(__file__))
os.chdir(base_dir)

import time
import socket
import threading
import subprocess
import urllib.request
import urllib.error
import urllib.parse
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
import pandas as pd
from urllib.parse import urlparse, parse_qs
import shutil
import base64

PORT = 4000
ESTADOS_FILE = 'estado_nodos.json'
EXCEL_FILE = 'importacion.xlsx'
HISTORIAL_FILE = 'historial_nodos.json'

global_state = {}
history_data = [] # Guardará snapshots históricos (hasta 7 días)

if os.path.exists(HISTORIAL_FILE):
    try:
        with open(HISTORIAL_FILE, 'r', encoding='utf-8') as f:
            history_data = json.load(f)
    except Exception as e:
        print(f"[!] Error leyendo {HISTORIAL_FILE}: {e}")

if os.path.exists(ESTADOS_FILE):
    try:
        with open(ESTADOS_FILE, 'r', encoding='utf-8') as f:
            global_state = json.load(f)
    except Exception as e:
        print(f"[!] Error leyendo {ESTADOS_FILE}: {e}")

def save_state():
    try:
        with open(ESTADOS_FILE, 'w', encoding='utf-8') as f:
            json.dump(global_state, f, ensure_ascii=False, indent=2)
    except Exception as e:
        print(f"[!] Error guardando {ESTADOS_FILE}: {e}")

# --- UTILIDADES PARA EXCEL ---
def normalize_col(c):
    import unicodedata
    return unicodedata.normalize('NFD', str(c).lower()).encode('ascii', 'ignore').decode('utf-8').replace('_', ' ')

def get_nodes_from_excel():
    if not os.path.exists(EXCEL_FILE):
        return []
        
    try:
        df = pd.read_excel(EXCEL_FILE)
        cols_lower = [normalize_col(c) for c in df.columns]
        df.columns = cols_lower
        
        nodes = []
        for idx, row in df.iterrows():
            ip = None
            telemando = None
            user = 'admin'
            password = '1'
            t_user = 'Administrador'
            t_pass = 'admin'
            
            for index, item in row.items():
                if pd.isna(item):
                    continue
                k = str(index)
                if k in ['ip estacion', 'ip pc', 'ip']:
                    if str(item).strip().lower() not in ['', 'nan', 'sin ip', 'n/a']:
                        ip = str(item).strip()
                elif k in ['usuario', 'user', 'usr']:
                    user = str(item).strip()
                elif k in ['contraseña', 'contrasena', 'pass', 'password', 'pw']:
                    password = str(item).strip()
                elif k in ['ip telemando', 'telemando', 'pdu', 'ip pdu']:
                    if str(item).strip() not in ['', 'nan', '*', 'n/a']:
                        telemando = str(item).strip()
                elif k in ['usuario telemando', 'usuario_telemando']:
                    t_user = str(item).strip()
                elif k in ['contraseña telemando', 'contrasena telemando', 'pass telemando']:
                    t_pass = str(item).strip()
            
            if ip:
                nodes.append({
                    'ip': ip,
                    'telemando': telemando,
                    'user': user,
                    'pass': password,
                    't_user': t_user,
                    't_pass': t_pass,
                    'row_idx': idx
                })
        return nodes
    except Exception as e:
        print(f"[!] Error leyendo excel: {e}")
        return []

# --- SCANNERS CORES ---

def check_ping(ip):
    try:
        result = subprocess.run(['ping', '-n', '1', '-w', '1000', ip], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return result.returncode == 0
    except:
        return False

def check_port(ip, port, timeout=1.0):
    try:
        with socket.create_connection((ip, port), timeout=timeout):
            return True
    except:
        return False

def check_argus(ip, user, password):
    # Intentar limpiar conexiones previas
    subprocess.run(['net', 'use', f'\\\\{ip}\\IPC$', '/delete', '/y'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    
    # 1. Login nativo Windows
    creds = []
    import re
    users_list = [u.strip() for u in re.split(r'[/,]', user) if u.strip()]
    pass_list = [p.strip() for p in re.split(r'[/,]', password) if p.strip()]
    if not users_list: users_list = ['admin']
    if not pass_list: pass_list = ['1']
    
    logged_in = False
    for u in users_list:
        for p in pass_list:
            cmd = ['net', 'use', f'\\\\{ip}\\IPC$', f'/user:{u}', p]
            res = subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if res.returncode == 0:
                logged_in = True
                break
        if logged_in: break
        
    if not logged_in:
        return {"status": False, "msg": "No Auth"}

    # 1.5. Descubrir el nombre de la carpeta (share)
    share_name = None
    try:
        res_view = subprocess.run(['net', 'view', f'\\\\{ip}'], capture_output=True, text=True, timeout=5)
        for line in res_view.stdout.splitlines():
            if 'argus' in line.lower():
                share_name = line.strip().split()[0]
                break
    except:
        pass

    if not share_name:
        return {"status": False, "msg": "Carpeta No Hallada"}

    # 2. Obtener archivo mas reciente en '\\IP\Share_Name'
    base_path = f'\\\\{ip}\\{share_name}'
    try:
        dir_cmd = f'cmd /c "dir "{base_path}" /o-d /a-d /b"'
        result = subprocess.run(dir_cmd, capture_output=True, text=True, timeout=8)
        lines = [l.strip() for l in result.stdout.split() if l.strip()]
        if not lines:
            return {"status": False, "msg": "Carpeta Vacía"}
            
        newest = os.path.join(base_path, lines[0])
        try:
            size1 = os.path.getsize(newest)
            time.sleep(6) # Aumentar a 6s para asegurar crecimiento en bloques
            size2 = os.path.getsize(newest)
            if size2 > size1:
                return {"status": True, "msg": "Grabando"}
            else:
                return {"status": False, "msg": "Parado"}
        except:
            return {"status": False, "msg": "Sin Acceso Archivo"}
    except:
        return {"status": False, "msg": "Timeout SMB"}

# Caches the PDU types
pdu_cache = {}

def get_pdu_status(ip, user, password):
    global pdu_cache
    
    def probe_legacy():
        try:
            url = f"http://{ip}/config/home_f.html"
            req = urllib.request.Request(url)
            b64str = base64.b64encode(f"{user}:{password}".encode('utf-8')).decode('ascii')
            req.add_header("Authorization", f"Basic {b64str}")
            with urllib.request.urlopen(req, timeout=3) as response:
                data = response.read().decode('utf-8', errors='ignore')
                
            ports = []
            import re
            regex = r'socket\s*\(\s*(\d+)\s*,\s*"([^"]+)"\s*,\s*(\d+)\s*,\s*\d+\s*\)'
            for match in re.finditer(regex, data):
                ports.append({
                    "id": int(match.group(1)),
                    "name": match.group(2).strip(),
                    "status": int(match.group(3))
                })
            if ports:
                return {"status": True, "ports": ports, "type": "legacy"}
        except Exception as e:
            pass
        return {"status": False, "type": "legacy"}
        
    def probe_pse544():
        try:
            cmd = ['curl.exe', '-s', '--anyauth', '-u', f'{user}:{password}', '-m', '5', f'http://{ip}/user/control.ssi']
            res = subprocess.run(cmd, capture_output=True, text=True)
            if res.returncode == 0 and res.stdout:
                ports = []
                import re
                regex = r'\["([^"]+)",true,"(On|Off)",0,\['
                idx = 0
                for match in re.finditer(regex, res.stdout):
                    ports.append({
                        "id": idx,
                        "name": match.group(1),
                        "status": 1 if match.group(2) == "On" else 0
                    })
                    idx += 1
                if ports:
                    return {"status": True, "ports": ports, "type": "pse544"}
        except:
            pass
        return {"status": False, "type": "pse544"}

    def probe_raritan():
        try:
            import json, urllib.request, ssl
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            protocol = "https" if check_port(ip, 443, 0.5) else "http"
            base_url = f"{protocol}://{ip}"
            
            login_data = json.dumps({"jsonrpc":"2.0", "method":"login", "params":{"login":user, "password":password}, "id":1}).encode('utf-8')
            req = urllib.request.Request(f"{base_url}/auth/login", data=login_data, headers={'Content-Type': 'application/json'})
            
            session_token = None
            try:
                with urllib.request.urlopen(req, context=ctx, timeout=4) as resp:
                    r_json = json.loads(resp.read().decode())
                    if r_json.get('result', {}).get('_ret_') == 0:
                        session_token = resp.getheader('Set-Cookie')
            except:
                pass
                
            if session_token:
                headers = {'Content-Type': 'application/json', 'Cookie': session_token}
                bulk_data = json.dumps({
                    "jsonrpc": "2.0", "method": "performBulk",
                    "params": {
                        "requests": [{"rid": f"/model/pdu/0/outlet/{i}", "jsonrpc": "2.0", "method": "getState", "id": i} for i in range(1, 9)]
                    }, "id": 2
                }).encode('utf-8')
                
                req2 = urllib.request.Request(f"{base_url}/bulk", data=bulk_data, headers=headers)
                with urllib.request.urlopen(req2, context=ctx, timeout=4) as resp2:
                    out_json = json.loads(resp2.read().decode())
                    
                ports = []
                for i, r in enumerate(out_json.get('result', {}).get('responses', [])):
                    if 'result' in r:
                        pstate = r['result'].get('_ret_', {}).get('powerState', 0)
                        ports.append({
                            "id": i + 1,
                            "name": f"Enchufe {i + 1}",
                            "status": 1 if pstate == 1 else 0
                        })
                if ports:
                    return {"status": True, "ports": ports, "type": "raritan"}
        except:
            pass
        return {"status": False, "type": "raritan"}

    cached = pdu_cache.get(ip)
    if cached == 'legacy':
        return probe_legacy()
    elif cached == 'pse544':
        return probe_pse544()
    elif cached == 'raritan':
        return probe_raritan()
        
    res_rar = probe_raritan()
    if res_rar["status"]:
        pdu_cache[ip] = 'raritan'
        return res_rar
        
    res_pse = probe_pse544()
    if res_pse["status"]:
        pdu_cache[ip] = 'pse544'
        return res_pse
        
    res_leg = probe_legacy()
    if res_leg["status"]:
        pdu_cache[ip] = 'legacy'
        return res_leg
        
    return {"status": False, "msg": "Fallo PDU"}

def set_pdu_action(ip, port, action, user, password):
    ctype = pdu_cache.get(ip, 'legacy')
    
    if ctype == 'raritan':
        try:
            import json, urllib.request, ssl
            ctx = ssl.create_default_context()
            ctx.check_hostname = False
            ctx.verify_mode = ssl.CERT_NONE
            protocol = "https" if check_port(ip, 443, 0.5) else "http"
            base_url = f"{protocol}://{ip}"
            
            login_data = json.dumps({"jsonrpc":"2.0", "method":"login", "params":{"login":user, "password":password}, "id":1}).encode('utf-8')
            req = urllib.request.Request(f"{base_url}/auth/login", data=login_data, headers={'Content-Type': 'application/json'})
            session_token = None
            with urllib.request.urlopen(req, context=ctx, timeout=4) as resp:
                r_json = json.loads(resp.read().decode())
                session_token = resp.getheader('Set-Cookie')
                
            if session_token:
                headers = {'Content-Type': 'application/json', 'Cookie': session_token}
                target_state = 1 if action == '1' else 0
                if action == 'r': target_state = 2 # 2 = cycle en API Raritan
                
                act_data = json.dumps({
                    "jsonrpc": "2.0", "method": "setPowerState",
                    "params": {"pstate": target_state}, "id": 2
                }).encode('utf-8')
                
                req2 = urllib.request.Request(f"{base_url}/model/pdu/0/outlet/{port}", data=act_data, headers=headers)
                with urllib.request.urlopen(req2, context=ctx, timeout=4) as resp2:
                    return True
            return False
        except:
            return False
            
    elif ctype == 'pse544':
        try:
            pse_action = '2'
            if action == '1': pse_action = '0'
            if action == '0': pse_action = '1'
            cmd = ['curl.exe', '-s', '--anyauth', '-u', f'{user}:{password}', '-d', f'CMD=0_{port}_{pse_action}', '-m', '5', f'http://{ip}/user/control.cgi']
            subprocess.run(cmd)
            return True
        except:
            return False
            
    else:
        try:
            url = f"http://{ip}/config/home_f.html"
            data = f"P{port}={action}".encode('ascii')
            req = urllib.request.Request(url, data=data, method='POST')
            b64str = base64.b64encode(f"{user}:{password}".encode('utf-8')).decode('ascii')
            req.add_header("Authorization", f"Basic {b64str}")
            with urllib.request.urlopen(req, timeout=3) as _:
                pass
            return True
        except:
            return False

# --- HILOS EN SEGUNDO PLANO ---
def safe_id(ip):
    return ip.replace('.', '-')

def daemon_ping_ports():
    print("[+] Scanner Ping/Ports Iniciado (cada 60s)")
    while True:
        nodes = get_nodes_from_excel()
        for node in nodes:
            ip = node['ip']
            sid = safe_id(ip)
            if sid not in global_state: global_state[sid] = {}
            
            ping_ok = check_ping(ip)
            global_state[sid]['ping'] = ping_ok
            
            if ping_ok:
                global_state[sid]['rdp'] = check_port(ip, 3389)
                global_state[sid]['vnc'] = check_port(ip, 5900)
            else:
                global_state[sid]['rdp'] = False
                global_state[sid]['vnc'] = False
            
            save_state()
            
        # NUEVO: Alimentar snapshot de historia
        ok = warn = danger = rec = 0
        for sid, st in global_state.items():
            if st.get('argus', {}).get('status', False):
                rec += 1
                
            if st.get('ping'):
                if st.get('rdp') or st.get('vnc'):
                    if st.get('rdp') and st.get('vnc'):
                        ok += 1
                    else:
                        warn += 1
                else:
                    danger += 1
            else:
                danger += 1
                
        snapshot = {
            "time": time.strftime("%Y-%m-%d %H:%M"),
            "ok": ok,
            "warn": warn,
            "danger": danger,
            "rec": rec
        }
        history_data.append(snapshot)
        # Retener máximo 7 días (7 * 24 * 60 = 10080)
        if len(history_data) > 10080:
            history_data.pop(0)
            
        try:
            with open(HISTORIAL_FILE, 'w', encoding='utf-8') as f:
                json.dump(history_data, f, ensure_ascii=False)
        except Exception as e:
            pass
            
        time.sleep(60)

def daemon_argus():
    print("[+] Scanner Argus SMB Iniciado (cada 180s)")
    while True:
        nodes = get_nodes_from_excel()
        for node in nodes:
            ip = node['ip']
            sid = safe_id(ip)
            if sid not in global_state: global_state[sid] = {}
            
            if global_state[sid].get('ping'):
                status = check_argus(ip, node['user'], node['pass'])
                global_state[sid]['argus'] = status
                save_state()
            else:
                global_state[sid]['argus'] = {"status": False, "msg": "Apagado"}
        time.sleep(180) # Grabaciones llevan tiempo, no asfixiar

def daemon_pdu():
    print("[+] Scanner PDU Iniciado (cada 30 min)")
    while True:
        nodes = get_nodes_from_excel()
        for node in nodes:
            ip_t = node['telemando']
            if not ip_t: continue
            
            sid = safe_id(node['ip'])
            if sid not in global_state: global_state[sid] = {}
            
            import re
            u_list = [u.strip() for u in re.split(r'[/,]', node['t_user']) if u.strip()]
            p_list = [p.strip() for p in re.split(r'[/,]', node['t_pass']) if p.strip()]
            if not u_list: u_list = ['Administrador']
            if not p_list: p_list = ['admin']
            
            success_res = None
            for u in u_list:
                for p in p_list:
                    res = get_pdu_status(ip_t, u, p)
                    if res.get('status'):
                        success_res = res
                        success_res['used_user'] = u
                        success_res['used_pass'] = p
                        break
                if success_res: break
            
            if success_res:
                global_state[sid]['pdu'] = success_res
                # Self-healing de Excel Opcional
                # A implementar en el futuro para purgar el excel
            else:
                global_state[sid]['pdu'] = {"status": False, "msg": "Offline"}
                
            save_state()
        time.sleep(1800)

# --- SERVIDOR WEB API ---
class NOCRequestHandler(SimpleHTTPRequestHandler):
    timeout = 5  # Timeout corto para evitar bloqueos por sockets envenenados
    
    def do_GET(self):
        parsed_url = urlparse(self.path)
        qs = parse_qs(parsed_url.query)
        
        if parsed_url.path == '/api/state':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(json.dumps(global_state).encode('utf-8'))
            return
            
        elif parsed_url.path == '/api/history':
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.send_header('Access-Control-Allow-Origin', '*')
            self.end_headers()
            self.wfile.write(json.dumps(history_data).encode('utf-8'))
            return
            
        elif parsed_url.path == '/api/check':
            # Fuerza comprobación ahora mismo
            ip = qs.get('ip', [''])[0]
            if ip:
                self.send_response(200)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                
                # Devolvemos lo actual rápido y lanzamos hilo secundario para refrescar
                sid = safe_id(ip)
                state = global_state.get(sid, {})
                self.wfile.write(json.dumps(state).encode('utf-8'))
                
                def force_check():
                    if check_ping(ip):
                        global_state[sid]['ping'] = True
                        global_state[sid]['rdp'] = check_port(ip, 3389)
                        global_state[sid]['vnc'] = check_port(ip, 5900)
                        
                        # Buscar credenciales en Excel en tiempo real para Argus
                        nodes = get_nodes_from_excel()
                        for n in nodes:
                            if n['ip'] == ip:
                                global_state[sid]['argus'] = check_argus(n['ip'], n['user'], n['pass'])
                                break
                    else:
                        global_state[sid]['ping'] = False
                    save_state()
                threading.Thread(target=force_check).start()
                return
            
        elif parsed_url.path == '/api/power/status':
            ip_t = qs.get('ip', [''])[0]
            auth = qs.get('auth', ['Administrador:admin'])[0]
            
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            
            for cred in auth.split(','):
                u, p = 'Administrador', 'admin'
                if ':' in cred:
                    parts = cred.split(':')
                    u = parts[0]
                    p = ':'.join(parts[1:])
                
                res = get_pdu_status(ip_t, u, p)
                if res.get('status'):
                    self.wfile.write(json.dumps(res).encode('utf-8'))
                    return
                    
            self.wfile.write(json.dumps({"status": False, "msg": "Offline"}).encode('utf-8'))
            return
            
        elif parsed_url.path == '/api/power/action':
            ip_t = qs.get('ip', [''])[0]
            port = qs.get('port', [''])[0]
            action = qs.get('action', [''])[0]
            auth = qs.get('auth', ['Administrador:admin'])[0]
            
            self.send_response(200)
            self.send_header('Content-type', 'application/json')
            self.end_headers()
            
            for cred in auth.split(','):
                u, p = 'Administrador', 'admin'
                if ':' in cred:
                    parts = cred.split(':')
                    u = parts[0]
                    p = ':'.join(parts[1:])
                
                if set_pdu_action(ip_t, port, action, u, p):
                    self.wfile.write(json.dumps({"status": True}).encode('utf-8'))
                    return
            self.wfile.write(json.dumps({"status": False}).encode('utf-8'))
            return
            
        elif parsed_url.path.startswith('/external/'):
            # Permite leer importacion.xlsx aunque este fuera
            ext_path = urllib.parse.unquote(parsed_url.path.replace('/external/', ''))
            if os.path.exists(ext_path):
                self.send_response(200)
                self.send_header('Content-type', 'application/octet-stream')
                self.end_headers()
                with open(ext_path, 'rb') as f:
                    self.wfile.write(f.read())
                return
            
        # Servir HTML/Estaticos de 'public' si existen, si no de la raiz
        public_path = os.path.join(os.getcwd(), 'public', parsed_url.path.lstrip('/'))
        if os.path.exists(public_path) and os.path.isfile(public_path):
            self.path = '/public/' + parsed_url.path.lstrip('/')
            
        elif parsed_url.path == '/' or parsed_url.path == '':
            self.path = '/public/index.html'

        return super().do_GET()

    def do_POST(self):
        parsed_url = urlparse(self.path)
        if parsed_url.path == '/api/save-excel':
            try:
                content_length = int(self.headers.get('Content-Length', 0))
                if content_length > 0:
                    file_data = self.rfile.read(content_length)
                    with open(EXCEL_FILE, 'wb') as f:
                        f.write(file_data)
                    self.send_response(200)
                    self.send_header('Content-type', 'application/json')
                    self.end_headers()
                    self.wfile.write(json.dumps({"success": True}).encode('utf-8'))
                    return
            except Exception as e:
                self.send_response(500)
                self.send_header('Content-type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps({"success": False, "error": str(e)}).encode('utf-8'))
                return
                
        self.send_response(404)
        self.end_headers()

class ThreadingServer(ThreadingHTTPServer):
    daemon_threads = True

def start_server():
    server_address = ('', PORT)
    httpd = ThreadingServer(server_address, NOCRequestHandler)
    # Establecer un timeout pequeño para la prevención de conexiones atascadas
    httpd.timeout = 2
    print(f"📡 Monitor CTER (Python) activo en http://localhost:{PORT}")
    import socket
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.connect(("8.8.8.8", 80))
    print(f"📡 Red Local: http://{s.getsockname()[0]}:{PORT}")
    httpd.serve_forever()

if __name__ == "__main__":
    # Iniciar hilos en modo daemon
    threading.Thread(target=daemon_ping_ports, daemon=True).start()
    threading.Thread(target=daemon_argus, daemon=True).start()
    threading.Thread(target=daemon_pdu, daemon=True).start()
    
    start_server()
