from RsInstrument import *
import time

IP_ESMB = '192.168.29.72'

def test_cmd(instr, cmd):
    try:
        instr.write(cmd)
        # Algunos equipos tardan un poco en registrar el error
        time.sleep(0.05)
        err = instr.query("SYST:ERR?")
        if "No error" in err:
            print(f"   >>> ÉXITO: {cmd}")
            return True
        else:
            # print(f"   Fallo: {cmd} -> {err}")
            pass
    except:
        pass
    return False

try:
    instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
    instr.visa_timeout = 200
    
    print(f"Escaneando sintaxis para conmutación en ESMB {IP_ESMB}...")
    
    prefixes = ["", ":"]
    roots = ["SYST", "OUTP", "ROUT", "COMM", "PORT"]
    subs = ["USER", "AUX", "COMM:USER", "COMM:AUX"]
    cmds = ["DIG", "OUTP", "BIT", "DATA", "VAL"]
    
    # Pruebas de formato decimal
    for p in prefixes:
        for r in roots:
            for s in subs:
                for c in cmds:
                    test_cmd(instr, f"{p}{r}:{s}:{c} 1")
                    test_cmd(instr, f"{p}{r}:{s} {c} 1")
                    test_cmd(instr, f"{p}{r}:{s} 1")

    # Pruebas de formato bits individuales (Legacy)
    for p in prefixes:
        test_cmd(instr, f"{p}OUTP:AUX BIT1,1")
        test_cmd(instr, f"{p}OUTP:AUX:BIT1 1")
        test_cmd(instr, f"{p}SYST:AUX:BIT1 1")
        test_cmd(instr, f"{p}SYST:USER:BIT1 1")
        test_cmd(instr, f"{p}AUXBIT 1")
        test_cmd(instr, f"{p}AUXBIT1 1")

    # Pruebas de sintaxis específica de EB200/ESMB encontradas en manuales
    test_cmd(instr, "SYST:PORT:OUTP 1")
    test_cmd(instr, "OUTP:PORT 1")
    test_cmd(instr, "ROUT:PORT 1")

    instr.close()
    print("Escaneo finalizado.")
except Exception as e:
    print(f"Error: {e}")
