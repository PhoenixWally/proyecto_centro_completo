from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def try_cmd(instr, cmd):
    try:
        instr.write(cmd)
        err = instr.query("SYST:ERR?")
        if "No error" in err:
            print(f"ÉXITO: {cmd}")
            return True
        else:
            # print(f"FALLO: {cmd} -> {err}")
            pass
    except:
        pass
    return False

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 200
    
    print("Probando todas las variantes de comandos de bits auxiliares...")
    
    bases = [
        "SYST:COMM:USER",
        "SYST:COMM:AUX",
        "SYST:USER",
        "SYST:AUX",
        "OUTP:USER",
        "OUTP:AUX",
        "ROUT:USER",
        "ROUT:AUX",
        "INP:ANT",
        "SYST:PORT"
    ]
    
    suffixes = [
        ":DIG",
        ":OUTP",
        ":VAL",
        ":DATA",
        ""
    ]

    for b in bases:
        for s in suffixes:
            cmd = f"{b}{s} 1"
            try_cmd(instr, cmd)

    instr.close()
except Exception as e:
    print(f"Error: {e}")
