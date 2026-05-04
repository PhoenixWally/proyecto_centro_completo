from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    instr.write("*CLS; ABORT")
    
    print("Testing Trace dump")
    instr.write("FREQ:STAR 101 MHz")
    instr.write("FREQ:STOP 102 MHz")
    # SWE:STEP doesn't exist? Try SWE:POIN
    try:
        instr.write("SWE:POIN 101")
    except:
        pass
        
    instr.write("INIT:CONT OFF")
    instr.write("INIT:IMM;*WAI")
    
    try:
        res = instr.query("TRAC? 1")
        print(f"TRAC? 1 -> len {len(res)}, start: {res[:50]}")
    except Exception as e:
        print(f"TRAC? 1 Error: {e}")
        print("SYST:ERR:", instr.query("SYST:ERR?"))

    try:
        res = instr.query("TRAC? TRACE1")
        print(f"TRAC? TRACE1 -> len {len(res)}, start: {res[:50]}")
    except Exception as e:
        print(f"TRAC? TRACE1 Error: {e}")
        print("SYST:ERR:", instr.query("SYST:ERR?"))

    instr.close()
except Exception as e:
    print(f"Error: {e}")
