from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    instr.write("*CLS; ABORT")
    
    # Let's try some trace parameters
    params = ["1", "CH1", "A", "MAX", "AVER", "CLR/WRITE", "ACT"]
    for p in params:
        try:
            res = instr.query(f"TRAC? {p}")
            print(f"TRAC? {p} -> SUCCESS (len: {len(res)})")
            break
        except Exception as e:
            err = instr.query("SYST:ERR?")
            print(f"TRAC? {p} -> {err}")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
