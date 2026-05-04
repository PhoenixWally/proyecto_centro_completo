from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 500
    
    print("Capturando estado actual del puerto (Antena add195 seleccionada en Argus)...")
    
    queries = [
        "SYST:COMM:USER:OUTP?",
        "SYST:AUX:DIG?",
        "OUTP:USER:DIG?",
        "SYST:USER:DIG?",
        "ROUT:HF?",
        "OUTP:AUX?",
        "SYST:AUX:VAL?",
        "INP:ANT?",
        "ROUT:ANT?"
    ]

    for q in queries:
        try:
            res = instr.query(q)
            print(f"VALOR {q} -> {res}")
        except:
            pass

    instr.close()
except Exception as e:
    print(f"Error: {e}")
