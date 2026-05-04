from RsInstrument import *
IP_ESMB = '192.168.29.72'
instr = RsInstrument(f'TCPIP::{IP_ESMB}::5555::SOCKET', True, False)
instr.visa_timeout = 1000

print("Consultando opciones y sistema...")
try: print("*IDN? ->", instr.query("*IDN?"))
except: pass
try: print("*OPT? ->", instr.query("*OPT?"))
except: pass
try: print("SYST:HELP? ->", instr.query("SYST:HELP?"))
except: pass

instr.close()
