from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def test_cmd(instr, cmd):
    try:
        instr.write(cmd)
        print(f"OK: {cmd}")
        return True
    except:
        return False

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 500
    
    print("Probando comandos de conmutación ZS129...")
    
    val = 1 # Antena 1
    
    # Lista de posibles comandos de puerto de usuario en ESMB/EB200
    test_cmd(instr, f"OUTP:USER:DIG {val}")
    test_cmd(instr, f"SYST:COMM:USER:OUTP {val}")
    test_cmd(instr, f"SYST:AUX:DIG {val}")
    test_cmd(instr, f"SYST:AUX:VAL {val}")
    test_cmd(instr, f"OUTP:AUX {val}")
    test_cmd(instr, f"OUTP:DIG {val}")
    test_cmd(instr, f"SYST:USER:DIG {val}")
    test_cmd(instr, f"SYST:COMM:AUX:DIG {val}")
    test_cmd(instr, f"ROUT:AUX {val}")
    test_cmd(instr, f"ROUT:USER {val}")
    
    # Algunos equipos usan el puerto paralelo o de usuario así:
    test_cmd(instr, f"OUTP:USER {val}")
    test_cmd(instr, f"SYST:OUTP {val}")

    instr.close()
except Exception as e:
    print(f"Error: {e}")
