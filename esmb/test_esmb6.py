from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    instr.write("*CLS; ABORT")
    instr.write("INIT:CONT ON")
    
    # Try polling SENS:DATA until valid
    print("Test: Busy wait polling")
    start = time.time()
    
    for f in [101.0, 101.1, 101.2, 101.3, 101.4]:
        instr.write(f"FREQ {f} MHz")
        
        # Busy wait
        polls = 0
        while True:
            val = instr.query_float("SENS:DATA?")
            polls += 1
            if val > -9e37:
                print(f"Freq: {f}, Val: {val}, Polls: {polls}")
                break
            if polls > 50:
                print(f"Freq: {f}, Timeout!")
                break
                
    print("Total time:", time.time() - start)
    instr.close()
except Exception as e:
    print(f"Error: {e}")
