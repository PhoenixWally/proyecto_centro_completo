from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    instr.write("*CLS; ABORT")
    
    # Test 1: *WAI
    print("Test 1: *WAI")
    start = time.time()
    for f in [101.0, 101.1, 101.2]:
        instr.write(f"FREQ {f} MHz;*WAI")
        val = instr.query("SENS:DATA?")
        print(f"Freq: {f}, Val: {val}")
    print("Time taken:", time.time() - start)

    # Test 2: *OPC?
    print("\nTest 2: *OPC?")
    start = time.time()
    for f in [101.0, 101.1, 101.2]:
        instr.query(f"FREQ {f} MHz;*OPC?")
        val = instr.query("SENS:DATA?")
        print(f"Freq: {f}, Val: {val}")
    print("Time taken:", time.time() - start)
    
    # Test 3: Sleep 0.05 but with INIT:IMM
    print("\nTest 3: INIT:IMM")
    instr.write("INIT:CONT OFF")
    start = time.time()
    for f in [101.0, 101.1, 101.2]:
        instr.write(f"FREQ {f} MHz")
        instr.write("INIT:IMM;*WAI")
        val = instr.query("SENS:DATA?")
        print(f"Freq: {f}, Val: {val}")
    print("Time taken:", time.time() - start)

    instr.close()
except Exception as e:
    print(f"Error: {e}")
