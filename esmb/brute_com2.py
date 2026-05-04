import serial
import time

try:
    # Abrimos COM2 a la velocidad confirmada
    port = serial.Serial('COM2', 2400, timeout=1)
    print("COM2 Abierto a 2400 baudios. Iniciando fuerza bruta de comandos RSU...")

    # Diccionario de comandos típicos de selectores R&S
    comandos_a_probar = [
        "3", "03", "A3", "ANT3", "P3", "PATH3", "INP3", "*3", "@3", "#3"
    ]

    for cmd in comandos_a_probar:
        print(f"Probando comando: '{cmd}'")
        
        # Variante 1: Sin salto de línea
        port.write(cmd.encode('ascii'))
        time.sleep(0.5)
        
        # Variante 2: Con Carriage Return (\r)
        port.write((cmd + '\r').encode('ascii'))
        time.sleep(0.5)
        
        # Variante 3: Con CR+LF (\r\n)
        port.write((cmd + '\r\n').encode('ascii'))
        time.sleep(0.5)

    # Variante 4: Bytes puros (DCI Decoders)
    print("Probando bytes binarios puros...")
    port.write(bytes([3]))       # Byte 3
    time.sleep(0.5)
    port.write(bytes([0, 3]))    # Word 0x0003
    time.sleep(0.5)
    port.write(bytes([255, 3]))  # Formato Header FF + 03
    time.sleep(0.5)

    port.close()
    print("\nPrueba finalizada. Cierra esta ventana cuando acabes.")
    input("Pulsa ENTER para salir...")

except Exception as e:
    print(f"Error abriendo puerto: {e}")
    input("Pulsa ENTER para salir...")
