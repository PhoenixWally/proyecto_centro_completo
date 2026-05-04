from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    instr.write("*CLS; ABORT")
    
    print("Testing INST:CAT?")
    try:
        res = instr.query("INST:CAT?")
        print(f"Modes: {res}")
    except:
        pass
        
    print("Testing Trace read in Receiver mode")
    try:
        print("TRAC? :", instr.query("TRAC?"))
    except Exception as e:
        print("TRAC? error:", e)
        
    print("Testing SCAN commands")
    try:
        instr.write("SCAN:STAR 100 MHz")
        instr.write("SCAN:STOP 105 MHz")
        instr.write("SCAN:STEP 100 kHz")
        instr.write("INIT:SCAN")
        # How to read scan data?
        print("SCAN data?", instr.query("READ:SCAN?"))
    except Exception as e:
        print("SCAN error:", e)

    instr.close()
except Exception as e:
    print(f"Error: {e}")
