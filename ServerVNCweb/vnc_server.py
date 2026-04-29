import sys
import os
import json
import logging
from websockify import WebSocketProxy
from websockify.token_plugins import TokenFile
import pandas as pd # Para cuando lea el excel

if getattr(sys, 'frozen', False):
    BASE_DIR = os.path.dirname(sys.executable)
else:
    BASE_DIR = os.path.dirname(os.path.abspath(__file__))

NOVNC_DIR = os.path.join(BASE_DIR, 'noVNC')
TARGETS_FILE = os.path.join(BASE_DIR, 'targets.conf')
DATA_JSON = os.path.join(BASE_DIR, 'data.json')

def check_novnc():
    """Verifica que la carpeta noVNC existe para poder servir el cliente web."""
    if not os.path.isdir(NOVNC_DIR):
        print("="*70)
        print(" [!] ADVERTENCIA CRÍTICA: Carpeta 'noVNC' no encontrada.")
        print(" Para que el visor funcione en el navegador, necesitas el cliente noVNC.")
        print(" 1. Descarga el ZIP desde: https://github.com/novnc/noVNC/archive/refs/tags/v1.6.0.zip")
        print(" 2. Descomprímelo.")
        print(" 3. Cambia el nombre de la carpeta extraída a exactamente 'noVNC'")
        print(f" 4. Colócala aquí: {NOVNC_DIR}")
        print("="*70)
        sys.exit(1)

def generate_mock_data():
    """Genera datos de prueba como último recurso si no se encuentra el Excel."""
    equipos = [
        {"id": "pc_oficina", "nombre": "PC Oficina Principal", "ciudad": "Málaga", "ip": "192.168.1.15", "puerto": 5900},
        {"id": "radar_norte", "nombre": "Sistema Radar Norte", "ciudad": "Sevilla", "ip": "192.168.1.20", "puerto": 5900}
    ]
    with open(TARGETS_FILE, 'w', encoding='utf-8') as f:
        for eq in equipos: f.write(f"{eq['id']}: {eq['ip']}:{eq['puerto']}\n")
    with open(DATA_JSON, 'w', encoding='utf-8') as f:
        json.dump(equipos, f, ensure_ascii=False, indent=4)
        
def parse_excel_data():
    """Analiza automáticamente el importacion.xlsx del directorio superior."""
    excel_path = os.path.join(os.path.dirname(BASE_DIR), 'importacion.xlsx')
    
    if not os.path.exists(excel_path):
        print(f"[!] Error: No veo el archivo en {excel_path}")
        return generate_mock_data()
        
    try:
        df = pd.read_excel(excel_path)
        # Limpiamos los nombres de las columnas de espacios raros
        df.columns = [str(c).strip() for c in df.columns]
        
        # --- BUSQUEDA MANUAL SEGÚN TAZA DE DATOS ---
        # Buscamos la columna que se llame 'IP ESTACION' o simplemente 'IP'
        col_ip = next((c for c in df.columns if c.lower() in ['ip estacion', 'ip', 'ip_estacion']), None)
        # Buscamos 'NOMBRE' o 'ESTACION'
        col_nombre = next((c for c in df.columns if c.lower() in ['nombre', 'estacion', 'identificador']), None)
        # Buscamos 'CIUDAD' o 'UBICACION'
        col_ciudad = next((c for c in df.columns if c.lower() in ['ciudad', 'localizacion', 'ubicacion', 'provincia']), None)

        if not col_ip:
            print(f"[!] ERROR: No encuentro la columna 'ip estacion'. Columnas vistas: {list(df.columns)}")
            return generate_mock_data()

        print(f"[*] Vinculando columna IP: '{col_ip}'")
        
        equipos = []
        for _, row in df.iterrows():
            ip_val = str(row[col_ip]).strip()
            # Saltamos celdas vacías o con texto de error
            if not ip_val or ip_val.lower() in ['nan', 'none', 'sin ip', 'n/a', '']: continue
            
            # Cogemos el nombre (o la IP si no hay nombre)
            nombre_val = str(row[col_nombre]).strip() if col_nombre and pd.notna(row[col_nombre]) else ip_val
            # Cogemos la ciudad
            ciudad_val = str(row[col_ciudad]).strip() if col_ciudad and pd.notna(row[col_ciudad]) else "General"
            
            # Generamos el ID para la URL
            safe_id = ip_val.replace('.', '-')
            
            equipos.append({
                "id": safe_id,
                "nombre": nombre_val,
                "ciudad": ciudad_val,
                "ip": ip_val,
                "puerto": 5900
            })
            
        # 1. targets.conf (para el motor interno de VNC)
        with open(TARGETS_FILE, 'w', encoding='utf-8') as f:
            for eq in equipos:
                f.write(f"{eq['id']}: {eq['ip']}:{eq['puerto']}\n")
                
        # 2. data.json (para la lista visual de la web)
        with open(DATA_JSON, 'w', encoding='utf-8') as f:
            json.dump(equipos, f, ensure_ascii=False, indent=4)
            
        print(f"[*] CARGA COMPLETA: {len(equipos)} estaciones listas para conectar.")

    except Exception as e:
        print(f"[!] Error al procesar Excel: {e}")
        return generate_mock_data()

def run_server():
    print("Iniciando Sistema de Control VNC Web...")
    check_novnc()
    parse_excel_data()
    
    print("\n" + "="*50)
    print("🚀 Servidor Proxy Websocket + HTTP Inicializado")
    print("🌐 URL del Panel: http://localhost:8085")
    print("="*50 + "\n")
    
    try:
        # Instanciamos el plugin Token antes de inyectárselo a la clase Servidor
        token_auth = TokenFile(TARGETS_FILE)

        server = WebSocketProxy(
            listen_host='0.0.0.0',
            listen_port=8085,
            token_plugin=token_auth,
            web=BASE_DIR,
            record=None,
            daemon=False
        )
        server.start_server()
    except PermissionError:
        print("\n[!] ERROR FATAL: Permisos denegados para el puerto 8085.")
        print("-> Es posible que necesites permisos, vuelve a ejecutar el script.\n")
        sys.exit(1)
    except Exception as e:
        print(f"\n[!] Error inesperado del servidor: {str(e)}\n")
        sys.exit(1)

if __name__ == '__main__':
    run_server()
