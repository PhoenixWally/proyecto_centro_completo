import sqlite3
import datetime
import os
import time
import threading
import csv
from RsInstrument import *

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH  = os.path.join(BASE_DIR, 'data', 'sustargus.db')

# Configuración de rotación
MAX_FILE_MINUTES = 5
MAX_FILE_BYTES   = 20 * 1024 * 1024  # 20MB

active_threads = {}

def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

def parse_db_date(date_str):
    """Parsea fechas de SQLite que pueden venir con T o espacio."""
    for fmt in ('%Y-%m-%dT%H:%M:%S', '%Y-%m-%d %H:%M:%S', '%Y-%m-%dT%H:%M', '%Y-%m-%d %H:%M'):
        try:
            return datetime.datetime.strptime(date_str, fmt)
        except:
            continue
    return None

class RecordingWorker(threading.Thread):
    def __init__(self, rid, ip, antenna, f_start, f_end, output_dir, station_name, end_time_str):
        super().__init__()
        self.rid = rid
        self.ip = ip
        self.antenna = antenna
        self.f_start = f_start
        self.f_end = f_end
        self.output_dir = output_dir if output_dir else "C:/Grabaciones"
        self.station_name = station_name
        self.end_time = parse_db_date(end_time_str)
        self.running = True

    def select_antenna(self):
        """Conmutar antena ZA129A1 vía TCP/Serial bridge (Puerto 10001)"""
        if not self.ip: return
        import socket
        try:
            s = socket.create_connection((self.ip, 10001), timeout=2)
            cmd = f"\nS{self.antenna}\r".encode()
            s.sendall(cmd)
            time.sleep(0.5)
            s.close()
            print(f"    [Antenna] ID {self.rid}: Seleccionada Antena {self.antenna}")
        except Exception as e:
            print(f"    [!] Error seleccionando antena en ID {self.rid}: {e}")

    def run(self):
        print(f"[+] INICIANDO GRABACIÓN ID {self.rid} | {self.station_name} | Fin: {self.end_time}")
        self.select_antenna()
        
        instr = None
        try:
            # Lógica maestra de grabador_esmb_ok.py
            instr = RsInstrument(f'TCPIP::{self.ip}::5555::SOCKET', True, False)
            instr.visa_timeout = 5000
            instr.write("*CLS; ABORT")
            instr.write("INIT:CONT ON")
            instr.write(":FREQ:MODE SWE")
            instr.write(":TRAC:FEED:CONT MTRACE, ALW")
            instr.write(":STAT:TRAC:ENAB #B10010")
            instr.write(f":FREQ:STAR {self.f_start} MHz;STOP {self.f_end} MHz")
            
            span = self.f_end - self.f_start
            paso = 0.1 if span > 1 else 0.01
            instr.write(f":SWE:STEP {paso} MHz")
            instr.write(":FORM ASC")
            
            file_start_time = time.time()
            current_file = None
            writer = None
            csv_file = None
            
            def open_new_file():
                nonlocal current_file, writer, csv_file, file_start_time
                if csv_file: csv_file.close()
                
                os.makedirs(self.output_dir, exist_ok=True)
                now_dt = datetime.datetime.now()
                now_str = now_dt.strftime('%Y%m%d_%H%M%S')
                filename = f"{self.station_name}_{now_str}.csv"
                current_file = os.path.join(self.output_dir, filename)
                
                csv_file = open(current_file, 'w', newline='')
                writer = csv.writer(csv_file)
                writer.writerow(['Fecha', 'Hora', 'Frecuencia (MHz)', 'Nivel (dBuV)'])
                file_start_time = time.time()
                print(f"    [File] ID {self.rid}: Nuevo archivo -> {filename}")

            open_new_file()
            trace_count = 0

            while self.running:
                now_dt = datetime.datetime.now()
                
                # Comprobar si debemos parar (por tiempo o por DB)
                if now_dt >= self.end_time:
                    print(f"[*] ID {self.rid}: Tiempo finalizado ({now_dt} >= {self.end_time})")
                    break
                
                # Cada 10 segundos verificamos si el usuario la ha parado en la web
                if trace_count % 10 == 0:
                    db = get_db()
                    row = db.execute("SELECT status FROM recordings WHERE id=?", (self.rid,)).fetchone()
                    db.close()
                    if not row or row['status'] == 'stopped':
                        print(f"[*] ID {self.rid}: Detenido por el usuario.")
                        break

                try:
                    instr.write(":INIT;*WAI")
                    time.sleep(0.5) 
                    
                    raw_data = instr.query(":TRAC? MTRACE")
                    levels = [float(x) for x in raw_data.split(',') if x.strip()]
                    
                    if len(levels) > 1:
                        fecha_str = now_dt.strftime("%Y-%m-%d")
                        hora_str = now_dt.strftime("%H:%M:%S.%f")[:-3]
                        paso_real = (self.f_end - self.f_start) / (len(levels) - 1)
                        
                        for i, lvl in enumerate(levels):
                            if lvl < -9e36: continue
                            freq = round(self.f_start + (i * paso_real), 6)
                            writer.writerow([fecha_str, hora_str, freq, lvl])
                        
                        csv_file.flush()
                        trace_count += 1
                        if trace_count % 20 == 0:
                            print(f"    [Rec] ID {self.rid}: {trace_count} trazas...")

                    if (time.time() - file_start_time) > (MAX_FILE_MINUTES * 60) or os.path.getsize(current_file) > MAX_FILE_BYTES:
                        open_new_file()
                except Exception as e:
                    print(f"    [!] ID {self.rid} Error: {e}")
                    time.sleep(1)

                time.sleep(0.1)

            if csv_file: csv_file.close()
            
            db = get_db()
            db.execute("UPDATE recordings SET status='done' WHERE id=?", (self.rid,))
            db.commit()
            db.close()
            print(f"[✓] Grabación {self.rid} COMPLETADA.")

        except Exception as e:
            print(f"[!] Error crítico ID {self.rid}: {str(e)}")
            db = get_db()
            db.execute("UPDATE recordings SET status='error' WHERE id=?", (self.rid,))
            db.commit()
            db.close()
        finally:
            if instr: instr.close()
            if self.rid in active_threads: del active_threads[self.rid]

def main():
    print("══ SustArgus Recorder Service v2.0 (Long-Term) ══")
    
    while True:
        try:
            db = get_db()
            now_dt = datetime.datetime.now()
            now_str = now_dt.strftime('%Y-%m-%d %H:%M:%S')
            
            # 1. Buscar grabaciones programadas
            query = '''
                SELECT r.*, s.name as s_name, s.ip_esmb as s_ip 
                FROM recordings r JOIN stations s ON r.station_id = s.id 
                WHERE (r.status = 'pending' OR r.status = 'running')
            '''
            rows = db.execute(query).fetchall()
            
            for r in rows:
                start = parse_db_date(r['start_time'])
                end   = parse_db_date(r['end_time'])
                
                # Caso A: Ya toca empezar
                if r['id'] not in active_threads and start <= now_dt < end:
                    print(f"[!] Detectada tarea ID {r['id']} ({r['s_name']}) -> LANZANDO")
                    
                    if r['status'] == 'pending':
                        db.execute("UPDATE recordings SET status='running' WHERE id=?", (r['id'],))
                        db.commit()
                    
                    t = RecordingWorker(r['id'], r['s_ip'], r['antenna'], r['freq_start'], r['freq_end'], r['output_dir'], r['s_name'], r['end_time'])
                    active_threads[r['id']] = t
                    t.start()
                
                # Caso B: Ya pasó su hora y sigue en DB (y no está en hilos activos)
                elif r['id'] not in active_threads and now_dt >= end:
                    db.execute("UPDATE recordings SET status='done' WHERE id=?", (r['id'],))
                    db.commit()
            
            db.close()
        except Exception as e:
            print(f"[!!!] Error loop: {e}")
            
        time.sleep(5)

if __name__ == "__main__":
    main()

if __name__ == "__main__":
    main()
