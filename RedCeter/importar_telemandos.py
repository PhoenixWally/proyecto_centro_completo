import pandas as pd
import openpyxl
import os

source_file = r'd:/jpit/Remotas/estacionesJPIT/Direcciones IP Red CTER_actualizado.xlsx'
target_file = r'd:/jpit/Remotas/estacionesJPIT/remodelado/importacion.xlsx'

print("Cargando archivo de importación destino...")
import_df = pd.read_excel(target_file)

# Diccionario para mapear: IP_PC -> IP_Telemando
ip_map = {}

print("Analizando hojas del archivo fuente de CTER...")
excel_src = pd.ExcelFile(source_file)
skip_sheets = ['Cód.Prov. y Pref.', 'Teléf.Sedes', 'VPN_EUROPEAS_PALOALTO', 'RESUMEN ACC.', 'Unidades Moviles', 'Provincia X', 'TOTAL', 'TOTAL CON NUEVOS USOS']

for sheet in excel_src.sheet_names:
    if sheet in skip_sheets: continue
    
    try:
        df = pd.read_excel(source_file, sheet_name=sheet, header=1)
        
        # Omitir si la hoja esta vacia o no tiene las columnas requeridas (por tener nombre ligeramente distinto)
        cols_lower = [str(c).lower().replace('á', 'a').replace('é', 'e').replace('í', 'i').replace('ó', 'o').replace('ú', 'u') for c in df.columns]
        
        # Encontrar el indice de las columnas necesarias
        idx_lugar = -1
        idx_ip = -1
        idx_equipo = -1
        
        for i, c in enumerate(cols_lower):
            if 'lugar' in c: idx_lugar = i
            elif 'direcci' in c and 'ip' in c: idx_ip = i
            elif 'equipo' in c: idx_equipo = i
            
        if idx_lugar == -1 or idx_ip == -1 or idx_equipo == -1:
            continue
            
        col_lugar = df.columns[idx_lugar]
        col_ip = df.columns[idx_ip]
        col_equipo = df.columns[idx_equipo]
        
        # Forward fill "Lugar"
        df[col_lugar] = df[col_lugar].ffill()
        
        # Group by Lugar
        for lugar, group in df.groupby(col_lugar):
            ip_pc = None
            ip_telemando = None
            
            for _, row in group.iterrows():
                eq = str(row[col_equipo]).lower().strip() if pd.notna(row[col_equipo]) else ''
                ip = str(row[col_ip]).strip() if pd.notna(row[col_ip]) else ''
                
                # Descartar IPs inválidas o nan
                if not ip or ip.lower() == 'nan': continue
                
                if 'pc' in eq or 'argus' in eq:
                    ip_pc = ip
                elif 'telemando' in eq or 'pdu' in eq:
                    ip_telemando = ip
                    
            if ip_pc and ip_telemando:
                ip_map[ip_pc] = ip_telemando
                
    except Exception as e:
        print(f"Error procesando hoja {sheet}: {e}")

print(f"Se encontraron {len(ip_map)} correspondencias de Telemando vinculadas a PC IP.")

print("Escribiendo datos en importacion.xlsx...")
# Rellenar en el import_df usando la fila y actualizar la fila original
# abrimos con openpyxl para no destruir las modificaciones existentes, o simplemente sobreescribimos pandas
# ya que importacion.xlsx es muy simple y no tiene un formato complejo o podemos sobrescribir usando Pandas
# que es 1000 veces mas simple

# El usuario ha pedido que 'TODO EL QUE TENGA IP DE TELEMANDO SERÁ Administrador/admin y admin'
# Si en importacion.xlsx ya tenia IP_Telemando y era vacía su password, la rellenamos. O en general a todos!

cambios = 0
for idx, row in import_df.iterrows():
    ip_estacion = str(row.get('IP_estacion', '')).strip()
    
    if ip_estacion in ip_map:
        import_df.at[idx, 'IP_telemando'] = ip_map[ip_estacion]
        cambios += 1
        
    # Verificar si TIENE ip telemando (cualquiera, lo tuviera de antes o nuevo)
    current_telemando = str(import_df.at[idx, 'IP_telemando']).strip()
    if current_telemando and current_telemando.lower() not in ['nan', 'none', '*']:
        import_df.at[idx, 'USUARIO TELEMANDO'] = 'Administrador/admin'
        import_df.at[idx, 'CONTRASEÑA TELEMANDO'] = 'admin'

import_df.to_excel(target_file, index=False)
print(f"MIGRACIÓN COMPLETADA EXITOSAMENTE. {cambios} nuevas IPs de telemandos importadas de la base de CTER.")
