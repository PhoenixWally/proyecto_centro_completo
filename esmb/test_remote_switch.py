from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def realizar_medida(instr, tag, f_start, f_stop):
    print(f"\n[Midiendo {tag}] {f_start}-{f_stop} MHz")
    try:
        instr.write(f"FREQ:STAR {f_start}MHz")
        instr.write(f"FREQ:STOP {f_stop}MHz")
        instr.write("INIT")
        time.sleep(3)
        res = instr.query("TRAC? MTRACE")
        data = [float(x) for x in res.split(',')]
        max_val = max(data)
        print(f"   > Max: {max_val:.2f} dBuV")
        return max_val
    except Exception as e:
        print(f"   > Error: {e}")
        return -999

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    
    # Reset de errores previo
    instr.write("*CLS")
    instr.write("ROUT:HF ANTV")
    instr.write("FREQ:MODE SWE")

    print("--- FASE 1: Antena 1 (ADD195) ---")
    # Intentamos activar BIT 1 (Antena 1)
    instr.write("OUTP:AUX BIT1,1")
    instr.write("OUTP:AUX BIT2,0")
    instr.write("OUTP:AUX BIT3,0")
    time.sleep(1)
    
    # Nivel de referencia en banda baja (donde ADD195 funciona)
    ref_a1 = realizar_medida(instr, "ADD195 - Ref 100MHz", 100, 110)
    # Nivel en banda alta (fuera de ADD195)
    test_a1 = realizar_medida(instr, "ADD195 - Test 1.5GHz", 1500, 1510)

    print("\n--- FASE 2: Antena 3 (HE314A1) ---")
    # Intentamos activar BIT 3 (Antena 3)
    instr.write("OUTP:AUX BIT1,0")
    instr.write("OUTP:AUX BIT2,0")
    instr.write("OUTP:AUX BIT3,1")
    time.sleep(1)
    
    # Nivel en banda alta (dentro de HE314A1)
    test_a3 = realizar_medida(instr, "HE314A1 - Test 1.5GHz", 1500, 1510)

    print("\n" + "="*30)
    print("ANÁLISIS DE SEÑAL A 1.5 GHz")
    print(f"ADD195  (Corte 1.3G): {test_a1:.2f} dBuV")
    print(f"HE314A1 (Corte 3.5G): {test_a3:.2f} dBuV")
    print("-" * 30)
    
    diff = test_a3 - test_a1
    if diff > 5:
        print(f"RESULTADO: DIFERENCIA DE {diff:.2f} dB.")
        print("¡LA CONMUTACIÓN HA FUNCIONADO!")
    else:
        print(f"RESULTADO: DIFERENCIA DE {diff:.2f} dB (insuficiente).")
        print("El comando OUTP:AUX BIT no parece haber cambiado la antena física.")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
