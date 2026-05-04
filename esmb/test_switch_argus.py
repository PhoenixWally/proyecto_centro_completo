from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    print(f"Conectando a {IP_ESMB} para prueba de conmutación...")
    # Usamos ID_QUERY=False para no interferir con el lock de Argus si es posible
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 3000

    print("\n--- PASO 1: Selector Interno (V/H) ---")
    for ant in ["ANTV", "ANTH"]:
        print(f"Cambiando a {ant}...")
        try:
            instr.write(f"ROUT:HF {ant}")
            time.sleep(2) # Pausa para que Argus se entere
        except Exception as e:
            print(f"Error en ROUT:HF: {e}")

    print("\n--- PASO 2: Bits Auxiliares (Selector Externo) ---")
    print("Probando activación secuencial de bits 1 al 4...")
    
    for bit in range(1, 5):
        print(f"Activando AUX BIT {bit}...")
        try:
            # Probamos las dos sintaxis más comunes para AUX BITS
            instr.write(f"OUTP:AUX:BIT{bit} 1")
            time.sleep(2)
            print(f"Desactivando AUX BIT {bit}...")
            instr.write(f"OUTP:AUX:BIT{bit} 0")
            time.sleep(1)
        except Exception as e:
            print(f"Error en BIT {bit}: {e}")

    print("\nPrueba finalizada. Comprueba si Argus ha reaccionado.")
    instr.close()

except Exception as e:
    print(f"Error general: {e}")
