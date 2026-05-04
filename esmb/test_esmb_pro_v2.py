from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    instr.write("*CLS; ABORT")
    
    print("[+] Configurando modo PRO (basado en G_ESMB.dll)...")
    
    # 1. Configurar modo y alimentacin de traza
    instr.write(":FREQ:MODE SWE")
    instr.write(":TRAC:FEED:CONT MTRACE, ALW") # ALW = Always (Siempre alimentar buffer)
    instr.write(":STAT:TRAC:ENAB #B10010")    # Habilitar reporte de traza
    
    # 2. Rango de frecuencias
    instr.write(":FREQ:STAR 101 MHz;STOP 103 MHz")
    instr.write(":SWE:STEP 100 kHz")
    
    # 3. Formato de datos
    instr.write(":FORM ASC")
    
    # 4. Iniciar y esperar un poco al barrido hardware
    print("[+] Lanzando barrido hardware...")
    instr.write(":INIT")
    time.sleep(1) # Damos 1 segundo para que el hardware llene el buffer
    
    # 5. Intentar descargar el bloque
    print("[+] Descargando bloque de datos MTRACE...")
    datos = instr.query(":TRAC? MTRACE")
    
    if "9.9" in datos and len(datos) < 20:
        print("[-] El buffer sigue devolviendo NaN. Probando con ITRACE...")
        datos = instr.query(":TRAC? ITRACE")
    
    print(f"[!] Resultado: {len(datos)} caracteres recibidos.")
    print(f"Muestra: {datos[:200]}...")
    
    instr.close()
except Exception as e:
    print(f"Error: {e}")
    # Ver error del equipo
    try:
        instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
        print("SYST:ERR:", instr.query("SYST:ERR?"))
        instr.close()
    except: pass
