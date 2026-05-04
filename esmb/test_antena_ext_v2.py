from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def test_cmd(instr, cmd, query=None):
    try:
        instr.write(cmd)
        if query:
            res = instr.query(query)
            print(f"OK: {cmd} -> {res}")
        else:
            print(f"OK: {cmd}")
        return True
    except:
        return False

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 500
    
    print("Iniciando escaneo exhaustivo...")
    
    # Probar ROUT:HF con números
    for i in range(1, 7):
        test_cmd(instr, f"ROUT:HF ANT{i}", "ROUT:HF?")
        
    # Probar INP:ANT
    for i in range(1, 7):
        test_cmd(instr, f"INP:ANT {i}", "INP:ANT?")

    # Probar el switch de antena genérico de R&S
    for i in range(1, 7):
        test_cmd(instr, f"SENS:CORR:OFFS:ANT:SEL {i}", "SENS:CORR:OFFS:ANT:SEL?")

    # Probar bits de salida (User Port)
    # Algunos equipos usan SYST:OUTP o OUTP:USER
    for i in range(0, 8):
        test_cmd(instr, f"OUTP:USER:BIT{i} 1")
        test_cmd(instr, f"SYST:AUX:BIT{i} 1")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
