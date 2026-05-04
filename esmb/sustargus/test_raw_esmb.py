import socket
import time
import csv
import datetime

# Configuración
ESMB_IP = "192.168.29.102"
ESMB_PORT = 5555
DURATION = 3  # segundos
OUTPUT_FILE = "test_raw_capture.csv"

def run_raw_capture():
    print(f"[*] Conectando a {ESMB_IP}:{ESMB_PORT}...")
    
    try:
        # 1. Abrir socket TCP
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(5)
        sock.connect((ESMB_IP, ESMB_PORT))
        print("[+] Conectado satisfactoriamente.")

        # 2. Conmutar Antena (ZA129A1)
        print(f"[*] Conmutando Antena 1 en {ESMB_IP}:10001...")
        import socket as s_lib
        try:
            s_ant = s_lib.create_connection((ESMB_IP, 10001), timeout=2)
            s_ant.sendall(b"\nS1\r") # Seleccionar Antena 1
            time.sleep(0.5)
            s_ant.close()
            print("[+] Antena 1 seleccionada.")
        except Exception as e:
            print(f"[!] No se pudo conectar al selector de antena: {e}")

        # 3. Configuración SCPI con verificación de errores
        def send_check(cmd):
            sock.sendall(cmd.encode() + b"\n")
            sock.sendall(b"SYST:ERR?\n")
            return sock.recv(1024).decode().strip()

        print("[*] Configurando ESMB...")
        send_check("*CLS")
        send_check(":FREQ:MODE SWE")
        send_check(":FREQ:START 1560MHz")
        send_check(":FREQ:STOP 1580MHz")
        send_check(":FORM ASC")
        send_check(":INIT:CONT ON")
        time.sleep(1)

        F_START = 1560.0
        F_STOP  = 1580.0

        print("[*] Iniciando ráfaga (Modo RAW / 60+ FPS)...")
        start_time = time.time()
        count = 0
        
        with open(OUTPUT_FILE, 'w', newline='') as f:
            writer = csv.writer(f)
            writer.writerow(['Timestamp', 'Frequency_MHz', 'Level_dBm'])

            while (time.time() - start_time) < DURATION:
                try:
                    # Query directa sin *WAI para no bloquear
                    sock.sendall(b":TRAC? MTRACE\n")
                    
                    response = b""
                    while not response.endswith(b"\n"):
                        chunk = sock.recv(65536)
                        if not chunk: break
                        response += chunk
                    
                    ts = datetime.datetime.now().strftime('%H:%M:%S.%f')[:-3]
                    data_str = response.decode().strip()
                    levels = [float(x) for x in data_str.split(',') if x.strip()]
                    
                    count += 1
                    num_points = len(levels)
                    
                    # Diagnóstico en consola
                    if num_points > 1:
                        step = (F_STOP - F_START) / (num_points - 1)
                        for i, lvl in enumerate(levels):
                            if lvl < 1e30: # Solo guardamos si no es error
                                freq = round(F_START + (i * step), 6)
                                writer.writerow([ts, freq, lvl])
                        
                        if count % 10 == 0:
                            print(f"    [+] Traza {count}: {num_points} puntos recibidos.")
                    else:
                        # Si num_points es 1, es el error 9.91E37
                        if count % 10 == 0:
                            print(f"    [!] Traza {count}: ERROR (Solo 1 punto recibido: {levels[0]})")
                    
                except Exception as e:
                    print(f"    [!] Error: {e}")
                    break

            print(f"[✓] Captura finalizada. Total trazas en 3 seg: {count}")

        sock.close()
        print(f"[*] Archivo guardado en: {OUTPUT_FILE}")

    except Exception as e:
        print(f"[!] Error: {e}")

if __name__ == "__main__":
    run_raw_capture()
