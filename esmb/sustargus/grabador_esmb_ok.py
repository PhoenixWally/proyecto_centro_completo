from RsInstrument import *
import time
import csv
import datetime
import os

# Configuración
ESMB_IP = "192.168.29.22"
DURATION = 3  # segundos
OUTPUT_FILE = "test_rs_capture.csv"
F_START = 100.0
F_STOP  = 120.0

def run_rs_capture():
    instr = None
    try:
        print(f"[*] Conectando a {ESMB_IP} vía RsInstrument...")
        # Resource string para socket directo
        resource = f'TCPIP::{ESMB_IP}::5555::SOCKET'
        instr = RsInstrument(resource, True, False)
        instr.visa_timeout = 5000
        
        print(f"[+] Conectado: {instr.query('*IDN?')}")

        # Configuración "PERFECTA" de app_escaner.py
        instr.write("*CLS; ABORT")
        instr.write("INIT:CONT ON")
        instr.write(":FREQ:MODE SWE")
        instr.write(":TRAC:FEED:CONT MTRACE, ALW")
        instr.write(":STAT:TRAC:ENAB #B10010")
        instr.write(f":FREQ:STAR {F_START} MHz;STOP {F_STOP} MHz")
        instr.write(":SWE:STEP 0.1 MHz")
        instr.write(":FORM ASC")
        
        # Pequeña espera inicial (app_escaner línea 60)
        time.sleep(0.5)

        print(f"[*] Iniciando ráfaga (Lógica app_escaner.py)...")
        
        with open(OUTPUT_FILE, 'w', newline='') as f:
            writer = csv.writer(f)
            writer.writerow(['Fecha', 'Hora', 'Frecuencia (MHz)', 'Nivel (dBuV)'])
            
            start_time = time.time()
            count = 0
            while (time.time() - start_time) < DURATION:
                try:
                    # Lógica exacta de app_escaner.py (líneas 72-79)
                    instr.write(":INIT;*WAI")
                    time.sleep(0.5)
                    
                    raw_data = instr.query(":TRAC? MTRACE")
                    niveles = [float(x) for x in raw_data.split(',') if x.strip()]
                    
                    if len(niveles) > 1:
                        now = datetime.datetime.now()
                        fecha_str = now.strftime("%Y-%m-%d")
                        hora_str = now.strftime("%H:%M:%S.%f")[:-3]
                        paso_real = (F_STOP - F_START) / (len(niveles) - 1)
                        
                        for i, n in enumerate(niveles):
                            # Filtro app_escaner
                            if n < -9e36: continue
                            
                            freq = round(F_START + (i * paso_real), 6)
                            writer.writerow([fecha_str, hora_str, freq, n])
                        
                        count += 1
                        if count % 2 == 0:
                            print(f"    [+] Traza {count} capturada ({len(niveles)} puntos)")
                    
                    time.sleep(0.1)
                except Exception as e:
                    print(f"    [!] Error: {e}")
                    break

            print(f"[✓] Finalizado. Trazas totales: {count}")

            print(f"[✓] Finalizado. Trazas totales: {count}")
            print(f"[*] Archivo: {OUTPUT_FILE}")

    except Exception as e:
        print(f"[!] Error crítico: {e}")
    finally:
        if instr:
            instr.close()

if __name__ == "__main__":
    run_rs_capture()
