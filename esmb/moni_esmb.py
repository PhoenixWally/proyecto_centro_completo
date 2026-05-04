from RsInstrument import *
import time

# CONFIGURACIÓN
IP_ESMB = '192.168.29.72' # <--- Cambia esto si es necesario
FREQ_START = "1574 MHz"
FREQ_STOP = "1576 MHz"

try:
    print(f"Conectando a {IP_ESMB}...")
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    
    # Limpieza inicial profunda
    print("Reseteando estados del equipo...")
    instr.write("*CLS; ABORT")
    time.sleep(0.5)
    
    # Configuración modo rápido
    print("Configurando barrido rápido...")
    instr.write(":FREQ:MODE SWE")
    instr.write(":TRAC:FEED:CONT MTRACE, ALW")
    instr.write(":STAT:TRAC:ENAB #B10010")
    instr.write(f":FREQ:STAR {FREQ_START};STOP {FREQ_STOP}")
    instr.write(":FORM ASC")
    
    print("\n--- INICIANDO MONITORIZACIÓN EN VIVO ---")
    print("Presiona Ctrl+C para detener\n")
    
    while True:
        try:
            # Disparar barrido
            instr.write(":INIT;*WAI")
            time.sleep(0.5) # Tiempo para que el equipo procese
            
            # Leer bloque
            raw_data = instr.query(":TRAC? MTRACE")
            
            # Mostrar resumen de lo que escupe el equipo
            puntos = raw_data.split(',')
            print(f"[{time.strftime('%H:%M:%S')}] Recibidos {len(puntos)} puntos. Muestra: {raw_data[:60]}...")
            
        except Exception as e:
            print(f"Error en el ciclo: {e}")
            time.sleep(1)
            
except Exception as e:
    print(f"Error de conexión: {e}")
finally:
    if 'instr' in locals():
        instr.close()
        print("\nConexión cerrada.")
