import socket
import threading

# Configuración
LISTEN_IP = '0.0.0.0'
LISTEN_PORT = 5555
ESMB_IP = '192.168.29.72'
ESMB_PORT = 5555

def forward(source, destination, direction):
    while True:
        try:
            data = source.recv(4096)
            if not data:
                break
            
            # Imprimir lo que pasa por el proxy
            if direction == "ARGUS -> ESMB":
                print(f"\n[ARGUS ENVIA]: {data.decode('utf-8', errors='ignore').strip()}")
            # else:
            #     print(f"[ESMB RESPONDE]: {data.decode('utf-8', errors='ignore').strip()}")
                
            destination.sendall(data)
        except Exception as e:
            print(f"Desconexión en {direction}")
            break

def handle_client(client_socket):
    try:
        esmb_socket = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        esmb_socket.connect((ESMB_IP, ESMB_PORT))
        print("¡Argus se ha conectado al proxy y el proxy al ESMB!")

        # Crear hilos bidireccionales
        t1 = threading.Thread(target=forward, args=(client_socket, esmb_socket, "ARGUS -> ESMB"))
        t2 = threading.Thread(target=forward, args=(esmb_socket, client_socket, "ESMB -> ARGUS"))
        
        t1.start()
        t2.start()
        
        t1.join()
        t2.join()
    except Exception as e:
        print(f"Error conectando al ESMB: {e}")
    finally:
        client_socket.close()

def start_proxy():
    server = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    server.bind((LISTEN_IP, LISTEN_PORT))
    server.listen(5)
    print(f"=== PROXY ESPIA INICIADO EN EL PUERTO {LISTEN_PORT} ===")
    print(f"Esperando a que Argus se conecte...")
    
    while True:
        client, addr = server.accept()
        print(f"\nNueva conexión desde {addr[0]}:{addr[1]}")
        client_handler = threading.Thread(target=handle_client, args=(client,))
        client_handler.start()

if __name__ == "__main__":
    start_proxy()
