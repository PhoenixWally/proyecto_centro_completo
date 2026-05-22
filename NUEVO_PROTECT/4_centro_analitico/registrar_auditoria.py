import sys
import os
import pandas as pd

if __name__ == "__main__":
    if len(sys.argv) < 6:
        print("Uso: registrar_auditoria.py <ruta_excel> <fecha> <hora> <estado> <resultado> <detalles_error> <alertas>")
        sys.exit(1)
        
    ruta_excel = sys.argv[1]
    fecha = sys.argv[2]
    hora = sys.argv[3]
    estado = sys.argv[4]      # "Finalizado" o "Error"
    resultado = sys.argv[5]   # "Correcto" o "Fallo"
    detalles_error = sys.argv[6] if len(sys.argv) > 6 else ""
    alertas = sys.argv[7] if len(sys.argv) > 7 else "No"

    # Columnas de auditoria en español
    nuevo_registro = {
        "Fecha": [fecha],
        "Hora": [hora],
        "Estado": [estado],
        "Resultado": [resultado],
        "Detalles Error": [detalles_error],
        "Alertas Detectadas": [alertas]
    }
    df_nuevo = pd.DataFrame(nuevo_registro)
    
    if os.path.exists(ruta_excel):
        try:
            df_existente = pd.read_excel(ruta_excel)
            # Concatenar respetando el orden
            df_final = pd.concat([df_existente, df_nuevo], ignore_index=True)
        except Exception as e:
            df_final = df_nuevo
    else:
        df_final = df_nuevo
        
    try:
        os.makedirs(os.path.dirname(ruta_excel), exist_ok=True)
        df_final.to_excel(ruta_excel, index=False)
        print(f"Registro añadido con éxito a: {os.path.basename(ruta_excel)}")
    except Exception as e:
        print("Error al guardar archivo de auditoria:", e)
        sys.exit(1)
