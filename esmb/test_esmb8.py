from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 2000
    
    commands = [
        "SWE:TIME?",
        "SENS:SWE:TIME?",
        "MTIM?",
        "SENS:MTIM?",
        "BAND?",
        "SENS:BAND?"
    ]
    
    print("Testing MTIM / SWE:TIME:")
    for cmd in commands:
        try:
            res = instr.query(cmd)
            print(f"{cmd} -> {res}")
        except:
            print(f"{cmd} -> Error")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
