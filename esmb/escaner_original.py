from RsInstrument import *
import time

# Conexión al equipo
instr = RsInstrument('TCPIP::192.168.29.22::5555::SOCKET', True, False)
instr.visa_timeout = 2000

# Limpiamos el equipo
instr.write("*CLS; ABORT")
instr.write("INIT:CONT ON") # Mantenemos el motor de medición encendido

# --- CONFIGURACIÓN DEL ESCÁNER ---
freq_inicio = 100.0  # MHz
freq_fin = 105.0     # MHz
paso = 0.1           # Saltos de 100 kHz (0.1 MHz)

# Puedes cambiar la antena antes de empezar el escaneo
# instr.write("INP:ANT 5") 

print(f"\n[+] Iniciando escaneo en tiempo real...")
print(f"[+] Rango: {freq_inicio} MHz a {freq_fin} MHz | Paso: {paso} MHz\n")

datos_escaneo = []
freq_actual = freq_inicio

# --- BUCLE DE ESCANEO (TIEMPO REAL) ---
while freq_actual <= freq_fin:
    # 1. Sintonizamos la frecuencia
    instr.write(f"FREQ {freq_actual} MHz")
    
    # Dependiendo de lo antiguo que sea el ESMB, a veces necesita 
    # un par de milisegundos para que el relé/filtro se asiente. 
    # Si ves que da errores, descomenta la siguiente línea:
    # time.sleep(0.01) 
    
    try:
        # 2. Pedimos la medición (Este método convierte la respuesta a número decimal automáticamente)
        nivel = instr.query_float("SENS:DATA?")
        
        # Guardamos el dato en nuestra matriz
        datos_escaneo.append((freq_actual, nivel))
        
        # 3. Lo mostramos por pantalla en tiempo real
        print(f"Frecuencia: {freq_actual:.3f} MHz  -->  Nivel: {nivel} dBuV")
        
    except Exception as e:
        print(f"Frecuencia: {freq_actual:.3f} MHz  -->  [Error de lectura]")

    # 4. Sumamos el paso para la siguiente frecuencia
    freq_actual += paso
    freq_actual = round(freq_actual, 3) # Redondeamos para evitar errores de coma flotante en Python

print("\n[V] Escaneo completado. Total de puntos capturados:", len(datos_escaneo))

# Aquí la variable 'datos_escaneo' tiene toda la ráfaga lista 
# para guardarse en CSV, graficarse con Matplotlib o enviarse a una web.

instr.close()