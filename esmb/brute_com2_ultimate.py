import serial
import time
import sys

def probar_fuerza_bruta():
    # Velocidades a probar
    baud_rates = [1200, 2400, 4800, 9600, 19200, 38400, 57600, 115200]
    
    # El comando descubierto en Argus es S:X donde X es el número de puerto (1 a 6)
    comandos = ["S:3", "S:1"]

    print("=== PROBANDO COMANDO S:3 EN COM2 ===")
    print("Presta atencion a la consola, vamos a ver qué contesta el equipo.\n")

    for baud in baud_rates:
        print(f"\n--- Abriendo COM2 a {baud} baudios ---")
        try:
            # timeout algo mayor para dar tiempo al equipo a contestar
            port = serial.Serial('COM2', baud, timeout=0.5)
            
            for cmd in comandos:
                print(f" -> Enviando: '{cmd}'")
                
                # Vamos a probar todos los finales de línea posibles que podría usar R&S
                variantes = [
                    (cmd, "Sin fin de línea"),
                    (cmd + '\r', "CR (Carriage Return)"),
                    (cmd + '\n', "LF (Line Feed)"),
                    (cmd + '\r\n', "CR+LF"),
                    ('\x02' + cmd + '\x03', "STX + Cmd + ETX") # Típico protocolo binario encapsulado
                ]
                
                for trama, desc in variantes:
                    try:
                        # Limpiamos buffer antes de enviar
                        port.reset_input_buffer()
                        
                        # Enviamos
                        port.write(trama.encode('ascii'))
                        time.sleep(0.3)
                        
                        # Leemos si el equipo contesta algo
                        if port.in_waiting > 0:
                            respuesta = port.read(port.in_waiting)
                            print(f"    [!] RESPUESTA (Variante {desc}): {respuesta}")
                    except Exception as e:
                        pass
                        
            port.close()
        except Exception as e:
            print(f"  Error al abrir puerto a {baud}: {e}")

    print("\n=== PRUEBA FINALIZADA ===")
    input("Pulsa ENTER para cerrar la ventana...")

if __name__ == '__main__':
    probar_fuerza_bruta()
