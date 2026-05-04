import serial
import time
import sys

def test_selector(puerto="COM2"):
    baudios = 9600
    try:
        ser = serial.Serial(
            port=puerto,
            baudrate=baudios,
            bytesize=serial.EIGHTBITS,
            parity=serial.PARITY_NONE,
            stopbits=serial.STOPBITS_TWO, # El log muestra que usa 2 bits de parada
            timeout=1
        )
        print(f"\n[+] Abierto {puerto} a {baudios} baudios, 8N2...")
        
        # Enviar comando de Antena 3
        comando = b'\nS3\r'
        print(f"[>] Enviando: {comando}")
        ser.write(comando)
        
        time.sleep(0.5)
        respuesta = ser.read(ser.in_waiting or 100)
        print(f"[<] Respuesta: {respuesta}")

        time.sleep(1)
        # Enviar comando de Antena 1
        comando2 = b'\nS1\r'
        print(f"\n[>] Enviando: {comando2}")
        ser.write(comando2)
        
        time.sleep(0.5)
        respuesta2 = ser.read(ser.in_waiting or 100)
        print(f"[<] Respuesta: {respuesta2}")

        ser.close()
    except Exception as e:
        print(f"[-] Error: {e}")

if __name__ == "__main__":
    puerto = sys.argv[1] if len(sys.argv) > 1 else "COM2"
    test_selector(puerto)
