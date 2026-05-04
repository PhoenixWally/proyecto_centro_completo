from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 3000
    
    print("--- TEST DE RADIOGONIOMETRÍA (DF) ---")
    
    # Intentamos activar el modo DF
    # Nota: Algunos equipos usan SENS:DF:STAT ON o simplemente DF:MODE ON
    print("Activando modo DF...")
    try:
        instr.write("DF:MODE ON")
    except:
        pass
        
    time.sleep(1)
    
    # Probamos varios comandos de consulta de Azimut
    df_commands = [
        "MEAS:DF?",
        "MEAS:DF:AZIM?",
        "SENS:DF:AZIM?",
        "MEAS:BEAR?",
        "FETCH:DF?"
    ]
    
    for cmd in df_commands:
        print(f"\nProbando comando: {cmd}")
        try:
            res = instr.query(cmd)
            print(f"   Respuesta: {res}")
            # Si responde algo coherente, intentamos 5 lecturas
            if res and len(res) > 0:
                print(f"Capturando ráfaga con {cmd}:")
                for i in range(5):
                    print(f"   [{i}] -> {instr.query(cmd)}")
                    time.sleep(0.5)
        except Exception as e:
            # print(f"   Error: {e}")
            pass

    # Limpieza
    try: instr.write("DF:MODE OFF")
    except: pass
    
    instr.close()
    print("\n--- TEST FINALIZADO ---")

except Exception as e:
    print(f"Error de conexión: {e}")
