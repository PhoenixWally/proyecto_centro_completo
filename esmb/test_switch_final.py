from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    
    print("--- INTENTO DE CONMUTACIÓN ZA129A1 ---")
    
    # 1. Desbloquear puerto
    print("Poniendo :OUTP:AUXMODE en modo USER...")
    try:
        instr.write(":OUTP:AUXMODE USER")
        time.sleep(0.5)
        mode = instr.query(":OUTP:AUXMODE?")
        print(f"Modo actual: {mode}")
    except Exception as e:
        print(f"Error cambiando modo: {e}")

    # 2. Probar valores decimales para antenas
    # Antena 1 a 4
    for val in range(1, 5):
        print(f"\nProbando activación de Antena {val} (Valor decimal {val})...")
        
        # Sintaxis 1: :OUTP:AUX <val>
        try:
            instr.write(f":OUTP:AUX {val}")
            err = instr.query("SYST:ERR?")
            if "No error" in err:
                print(f"Sintaxis :OUTP:AUX {val} -> ACEPTADA")
        except: pass
            
        # Sintaxis 2: :OUTP:USER <val>
        try:
            instr.write(f":OUTP:USER {val}")
            err = instr.query("SYST:ERR?")
            if "No error" in err:
                print(f"Sintaxis :OUTP:USER {val} -> ACEPTADA")
        except: pass

        # Sintaxis 3: :SYST:COMM:USER:OUTP <val>
        try:
            instr.write(f":SYST:COMM:USER:OUTP {val}")
            err = instr.query("SYST:ERR?")
            if "No error" in err:
                print(f"Sintaxis :SYST:COMM:USER:OUTP {val} -> ACEPTADA")
        except: pass

        time.sleep(2) # Pausa para ver en Argus

    instr.close()
    print("\n--- PRUEBA FINALIZADA ---")

except Exception as e:
    print(f"Error: {e}")
