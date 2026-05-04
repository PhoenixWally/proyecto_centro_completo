from RsInstrument import *
import time

try:
    instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
    instr.visa_timeout = 5000
    instr.write("*CLS; ABORT")
    
    print("[+] Configurando modo SWE...")
    instr.write(":FREQ:MODE SWE")
    instr.write(":TRAC:FEED:CONT MTRACE, ALW")
    instr.write(":STAT:TRAC:ENAB #B10010")
    instr.write(":FREQ:STAR 101 MHz;STOP 103 MHz")
    instr.write(":SWE:STEP 100 kHz")
    instr.write(":FORM ASC")
    
    # Send the first sweep
    print("[+] Barrido 1...")
    instr.write(":SWE:COUN 1") # Let's make sure it's 1
    instr.write(":INIT")
    time.sleep(1)
    try:
        datos1 = instr.query(":TRAC? MTRACE")
        print(f"   -> Recibidos {len(datos1)} caracteres en barrido 1")
    except Exception as e:
        print(f"   -> Error barrido 1: {e}")

    # Send the second sweep
    print("[+] Barrido 2...")
    instr.write(":INIT")
    time.sleep(1)
    try:
        datos2 = instr.query(":TRAC? MTRACE")
        print(f"   -> Recibidos {len(datos2)} caracteres en barrido 2")
        print(f"   -> Datos 2: {datos2[:100]}")
    except Exception as e:
        print(f"   -> Error barrido 2: {e}")
        
    instr.close()
except Exception as e:
    print(f"Error: {e}")
