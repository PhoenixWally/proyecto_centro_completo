from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 3000
    
    print("--- PRUEBA FINAL DE GONIÓMETRO (DDF) ---")
    
    # 1. Limpiar cola de errores
    instr.write("*CLS")
    
    # 2. Configurar modo DF continuo (Encontrado en DLL G_DDF205)
    print("Configurando modo DF:MODE CONT...")
    try:
        instr.write("MEAS:DF:MODE CONT")
        time.sleep(0.5)
        # Verificamos si aceptó el comando
        err = instr.query("SYST:ERR?")
        print(f"Estado tras comando: {err}")
    except Exception as e:
        print(f"Error enviando modo: {e}")

    # 3. Intentar lectura de datos DF
    print("\nIniciando captura de datos (15 segundos)...")
    for i in range(30):
        try:
            # Intentamos la query estándar de DF
            res = instr.query("MEAS:DF?")
            if res:
                print(f"[{i}] DATO RECIBIDO -> {res}")
            else:
                print(f"[{i}] Sin datos...")
        except:
            # Si MEAS:DF? falla, probamos con FETCH:DF?
            try:
                res = instr.query("FETCH:DF?")
                print(f"[{i}] FETCH RECIBIDO -> {res}")
            except:
                pass
        time.sleep(0.5)

    # 4. Apagar modo DF
    try:
        instr.write("MEAS:DF:MODE NORM")
    except:
        pass
        
    instr.close()
    print("\n--- PRUEBA FINALIZADA ---")

except Exception as e:
    print(f"Error: {e}")
