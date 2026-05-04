from RsInstrument import *
import time

try:
    # Conectamos
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    instr.write("*CLS; ABORT")
    
    print("[+] Configurando modo SWE (Sweep) estilo Argus...")
    
    # 1. Ponemos modo Barrido
    instr.write(":FREQ:MODE SWE")
    instr.write(":SWE:DIR UP")
    instr.write(":SWE:COUN 1") # Un solo barrido para la prueba
    
    # 2. Rango
    instr.write(":FREQ:STAR 101 MHz;STOP 103 MHz")
    instr.write(":SWE:STEP 100 kHz")
    
    # 3. Formato (usamos ASC para leerlo fácil ahora, luego pasamos a PACK si funciona)
    instr.write(":FORM ASC")
    
    # 4. Disparamos
    print("[+] Iniciando barrido...")
    start = time.time()
    instr.write(":INIT; *WAI")
    
    # 5. Pedimos la traza completa (MTRACE o TRACE1)
    print("[+] Pidiendo bloque de datos (Trace)...")
    try:
        # Probamos con MTRACE que aparecía en la DLL
        datos = instr.query(":TRAC? MTRACE")
        print(f"[!] ¡ÉXITO! Recibidos {len(datos)} caracteres.")
        print(f"Muestra de datos: {datos[:200]}...")
    except Exception as e:
        print(f"[-] Falló MTRACE: {e}")
        try:
            # Reintento con TRACE1 por si acaso
            datos = instr.query(":TRAC? TRACE1")
            print(f"[!] ¡ÉXITO con TRACE1! Recibidos {len(datos)} caracteres.")
        except Exception as e2:
            print(f"[-] Falló TRACE1: {e2}")

    print(f"Tiempo total de captura: {time.time() - start:.3f} segundos")
    
    instr.close()
except Exception as e:
    print(f"Error general: {e}")
