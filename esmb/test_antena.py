from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    
    print("Probando comandos de ANTENA...")
    
    # Lista de posibles comandos encontrados en la DLL
    comandos = [
        "ROUT:HF?",
        "ROUT:HF ANTV",
        "ROUT:HF?",
        "ROUT:HF ANTH",
        "ROUT:HF?",
        "INP:ANT?",
        "INP:ANT1",
        "INP:ANT2",
        "ROUT:ANT?",
        "ROUT:ANT1",
        "ROUT:ANT2"
    ]
    
    for cmd in comandos:
        try:
            if "?" in cmd:
                res = instr.query(cmd)
                print(f"QUERY {cmd} -> {res}")
            else:
                instr.write(cmd)
                print(f"WRITE {cmd} -> OK")
        except Exception as e:
            print(f"ERROR en {cmd}: {e}")
            # Ver error SCPI
            try: print("   SYST:ERR:", instr.query("SYST:ERR?"))
            except: pass

    instr.close()
except Exception as e:
    print(f"Error: {e}")
