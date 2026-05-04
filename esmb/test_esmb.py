from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    
    print("Test 1: No INIT command, sleep 0.05")
    instr.write("*CLS; ABORT")
    instr.write("FREQ 101 MHz")
    time.sleep(0.05)
    print(instr.query("SENS:DATA?"))
    
    print("Test 2: INIT:CONT ON, sleep 0.05")
    instr.write("*CLS; ABORT")
    instr.write("INIT:CONT ON")
    instr.write("FREQ 102 MHz")
    time.sleep(0.05)
    print(instr.query("SENS:DATA?"))
    
    print("Test 3: INIT:CONT ON, sleep 0.2")
    instr.write("FREQ 103 MHz")
    time.sleep(0.2)
    print(instr.query("SENS:DATA?"))
    
    print("Test 4: INIT:CONT OFF, sleep 0.05")
    instr.write("*CLS; ABORT")
    instr.write("INIT:CONT OFF")
    instr.write("FREQ 101 MHz")
    time.sleep(0.05)
    print(instr.query("SENS:DATA?"))

    instr.close()
except Exception as e:
    print(f"Error: {e}")
