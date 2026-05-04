from RsInstrument import *

# El string de conexión exacto para tu cacharro
instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
print(instr.query('*IDN?')) # Esto te devolverá el IDN del ESMB para confirmar