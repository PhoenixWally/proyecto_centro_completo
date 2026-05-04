from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def realizar_medida(instr, tag, f_start, f_stop):
    print(f"\n[Midiendo {tag}] Rango: {f_start}-{f_stop} MHz")
    try:
        instr.write(f"FREQ:STAR {f_start}MHz")
        instr.write(f"FREQ:STOP {f_stop}MHz")
        # El ESMB en modo SCAN/SWE necesita INIT
        instr.write("INIT")
        time.sleep(2.5) # Esperamos el barrido
        # Intentamos obtener el nivel máximo del barrido
        # Si MEAS:SCAL:LEV:MAX? no existe, usamos un comando de traza
        res = instr.query("TRAC? MTRACE")
        data = [float(x) for x in res.split(',')]
        max_val = max(data)
        print(f"   > Nivel Máximo detectado: {max_val:.2f} dBuV")
        return max_val
    except Exception as e:
        print(f"   > Error en medida: {e}")
        return -999

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    
    print("Iniciando prueba de antenas...")
    instr.write("ROUT:HF ANTV") # Entrada común a ZA129A1
    instr.write("FREQ:MODE SWE") # Modo barrido

    # --- ANTENA 1: ADD195 ---
    print("\n--- PASO 1: Seleccionando ADD195 (Antena 1) ---")
    instr.write("SYST:COMM:USER:DIG:OUTP 1") 
    time.sleep(1)
    
    valido_a1 = realizar_medida(instr, "ADD195 - Rango Válido", 100, 200)
    invalido_a1 = realizar_medida(instr, "ADD195 - Rango Fuera (1.5GHz)", 1500, 1600)

    # --- ANTENA 3: HE314A1 ---
    print("\n--- PASO 2: Seleccionando HE314A1 (Antena 3) ---")
    instr.write("SYST:COMM:USER:DIG:OUTP 3")
    time.sleep(1)
    
    valido_a3 = realizar_medida(instr, "HE314A1 - Rango 1.5GHz", 1500, 1600)

    print("\n" + "="*40)
    print("        RESULTADOS DEL EXPERIMENTO")
    print("="*40)
    print(f"ADD195  a 1.5 GHz (Fuera de rango): {invalido_a1:.2f} dBuV")
    print(f"HE314A1 a 1.5 GHz (Rango válido):   {valido_a3:.2f} dBuV")
    print("-" * 40)
    if valido_a3 > invalido_a1 + 5:
        print(f"ÉXITO: La antena HE314A1 recibe {valido_a3 - invalido_a1:.2f} dB más que la ADD195.")
        print("El comando de conmutación es CORRECTO.")
    else:
        print("RESULTADO NO CONCLUYENTE: Los niveles son muy similares.")
        print("Es posible que el comando de conmutación no se esté aplicando o las antenas reciban igual en ese punto.")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
