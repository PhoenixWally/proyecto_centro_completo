#!/usr/bin/env python3
"""
Script independiente para verificar conexión SMB
Prueba conexión a dos servidores SMB con debug extenso
Ubicación: Remotas/estacionesJPIT/remodelado/
"""

import socket
import sys
import time
import os
from datetime import datetime

# Configuración de IPs
IPS_OBJETIVO = ["192.168.29.11", "192.168.29.71"]
PUERTO_SMB = 445

def log_debug(mensaje, tipo="INFO"):
    """Registra mensajes de debug con timestamp"""
    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]
    print(f"[{timestamp}] [{tipo}] {mensaje}")
    sys.stdout.flush()

def verificar_conectividad_ping(ip):
    """Intenta conectar a puerto SMB (445)"""
    log_debug(f"Iniciando prueba de conectividad a {ip}:{PUERTO_SMB}", "INICIO")
    
    try:
        log_debug(f"Creando socket para {ip}...", "DEBUG")
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(5)
        
        log_debug(f"Configurando socket con timeout de 5 segundos", "DEBUG")
        
        log_debug(f"Intentando conectar a {ip}:{PUERTO_SMB}...", "DEBUG")
        inicio = time.time()
        resultado = sock.connect_ex((ip, PUERTO_SMB))
        duracion = time.time() - inicio
        
        if resultado == 0:
            log_debug(f"✓ CONEXIÓN EXITOSA a {ip} en {duracion:.2f}s", "EXITO")
            sock.close()
            return True
        else:
            log_debug(f"✗ FALLO DE CONEXIÓN a {ip} - Código de error: {resultado}", "ERROR")
            sock.close()
            return False
            
    except socket.gaierror as e:
        log_debug(f"✗ ERROR DE RESOLUCIÓN DE DNS para {ip}: {e}", "ERROR")
        return False
    except socket.timeout:
        log_debug(f"✗ TIMEOUT - No se pudo conectar a {ip} en 5 segundos", "ERROR")
        return False
    except Exception as e:
        log_debug(f"✗ EXCEPCIÓN INESPERADA para {ip}: {type(e).__name__}: {e}", "ERROR")
        return False

def verificar_puerto_abierto(ip, puerto):
    """Verifica si un puerto específico está abierto"""
    log_debug(f"Verificando puerto {puerto} en {ip}...", "DEBUG")
    
    try:
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.settimeout(3)
        resultado = sock.connect_ex((ip, puerto))
        sock.close()
        
        if resultado == 0:
            log_debug(f"✓ Puerto {puerto} ABIERTO en {ip}", "EXITO")
            return True
        else:
            log_debug(f"✗ Puerto {puerto} CERRADO en {ip}", "ADVERTENCIA")
            return False
    except Exception as e:
        log_debug(f"✗ Error al verificar puerto {puerto}: {e}", "ERROR")
        return False

def verificar_red_local():
    """Verifica la red local y configuración"""
    log_debug("Verificando configuración de red local...", "DEBUG")
    
    try:
        hostname = socket.gethostname()
        ip_local = socket.gethostbyname(hostname)
        log_debug(f"Host local: {hostname}", "INFO")
        log_debug(f"IP local: {ip_local}", "INFO")
    except Exception as e:
        log_debug(f"Error al obtener info de red: {e}", "ADVERTENCIA")

def info_sistema():
    """Muestra información del sistema"""
    log_debug("=" * 60, "INFO")
    log_debug("INFORMACIÓN DEL SISTEMA", "INFO")
    log_debug("=" * 60, "INFO")
    log_debug(f"Plataforma: {sys.platform}", "INFO")
    log_debug(f"Versión Python: {sys.version}", "INFO")
    log_debug(f"Directorio actual: {os.getcwd()}", "INFO")
    log_debug("=" * 60, "INFO")

def main():
    log_debug("INICIANDO PRUEBA DE CONEXIÓN SMB", "INICIO")
    log_debug(f"Objetivo: Verificar conexión a {len(IPS_OBJETIVO)} servidor(es) SMB", "INFO")
    
    info_sistema()
    verificar_red_local()
    
    resultados = {}
    
    for ip in IPS_OBJETIVO:
        log_debug("-" * 60, "INFO")
        log_debug(f"PRUEBA PARA: {ip}", "INFO")
        log_debug("-" * 60, "INFO")
        
        # Prueba puerto SMB
        log_debug(f"Intentando conectar a puerto SMB (445)...", "DEBUG")
        conexion_exitosa = verificar_conectividad_ping(ip)
        
        # Prueba puertos comunes adicionales
        puertos_adicionales = [139, 137, 138]
        log_debug(f"Verificando puertos adicionales: {puertos_adicionales}", "DEBUG")
        
        puertos_abiertos = []
        for puerto in puertos_adicionales:
            if verificar_puerto_abierto(ip, puerto):
                puertos_abiertos.append(puerto)
        
        resultados[ip] = {
            "puerto_445": conexion_exitosa,
            "puertos_adicionales": puertos_abiertos
        }
        
        time.sleep(1)
    
    # Resumen final
    log_debug("\n" + "=" * 60, "INFO")
    log_debug("RESUMEN DE RESULTADOS", "INFO")
    log_debug("=" * 60, "INFO")
    
    for ip, datos in resultados.items():
        estado_445 = "✓ CONECTADO" if datos["puerto_445"] else "✗ DESCONECTADO"
        puertos_extra = ", ".join(map(str, datos["puertos_adicionales"])) or "Ninguno"
        
        log_debug(f"{ip}: Puerto 445 -> {estado_445}", "INFO")
        log_debug(f"{ip}: Otros puertos abiertos -> {puertos_extra}", "INFO")
    
    log_debug("=" * 60, "INFO")
    log_debug("PRUEBA FINALIZADA", "FINALIZACION")

if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        log_debug("\nPrueba interrumpida por el usuario", "ADVERTENCIA")
        sys.exit(0)
    except Exception as e:
        log_debug(f"Error fatal: {e}", "CRITICO")
        sys.exit(1)
