from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    instr.write("*CLS; ABORT")
    instr.write("INIT:CONT ON")
    
    print("Test limits")
    for s in [0.15, 0.12, 0.10, 0.08, 0.06]:
        instr.write(f"FREQ 101.5 MHz")
        time.sleep(0.3) # stabilize
        instr.write(f"FREQ 101.6 MHz")
        time.sleep(s)
        val = instr.query_float("SENS:DATA?")
        print(f"Sleep {s}s -> Val: {val}")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
