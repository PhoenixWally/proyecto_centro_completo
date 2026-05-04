from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    
    print("Buscando más posiciones de antena en ROUT:HF...")
    
    for i in range(1, 10):
        cmd = f"ROUT:HF ANT{i}"
        try:
            instr.write(cmd)
            # Si no da error, comprobamos si se aplicó
            current = instr.query("ROUT:HF?")
            print(f"Prueba {cmd} -> Aplicado. Actual: {current}")
        except:
            pass

    print("\nBuscando en INP:ANT...")
    for i in range(1, 10):
        cmd = f"INP:ANT {i}"
        try:
            instr.write(cmd)
            current = instr.query("INP:ANT?")
            print(f"Prueba {cmd} -> Aplicado. Actual: {current}")
        except:
            pass

    print("\nBuscando en OUTP:AUX:BIT...")
    for i in range(1, 9):
        cmd = f"OUTP:AUX:BIT{i} 1"
        try:
            instr.write(cmd)
            print(f"Prueba {cmd} -> OK")
        except:
            pass

    instr.close()
except Exception as e:
    print(f"Error: {e}")
