import os
import sys

# Asegurar que la salida estándar siempre use UTF-8 en Windows
if sys.stdout.encoding != 'utf-8':
    try:
        sys.stdout.reconfigure(encoding='utf-8')
    except AttributeError:
        pass  # Fallback para versiones de python muy antiguas si las hubiera

def main():
    if len(sys.argv) < 2:
        print("<span style='color:red;'>Error: No se especificó la ruta del archivo.</span>")
        sys.exit(1)

    file_path = sys.argv[1]
    if not os.path.exists(file_path):
        print(f"<span style='color:red;'>Error: El archivo no existe o no es accesible: {file_path}</span>")
        sys.exit(1)

    ext = os.path.splitext(file_path)[1].lower()

    try:
        if ext == '.csv':
            import pandas as pd
            # Intentar primero con punto y coma (separador típico en España con decimal en coma)
            try:
                df = pd.read_csv(file_path, sep=';', decimal=',', on_bad_lines='skip', nrows=500, encoding='utf-8', comment='#')
                # Si resulta en una sola columna, puede que el separador real sea la coma
                if len(df.columns) <= 1:
                    df = pd.read_csv(file_path, sep=',', decimal='.', on_bad_lines='skip', nrows=500, encoding='utf-8', comment='#')
            except Exception:
                # Fallback a coma
                df = pd.read_csv(file_path, sep=',', decimal='.', on_bad_lines='skip', nrows=500, encoding='utf-8', comment='#')
            
            html_table = df.to_html(table_id="tabla-preview", classes='display nowrap', index=False)
            print(html_table)

        elif ext == '.xlsx':
            import pandas as pd
            df = pd.read_excel(file_path, nrows=500)
            html_table = df.to_html(table_id="tabla-preview", classes='display nowrap', index=False)
            print(html_table)

        elif ext == '.docx':
            # Intentar mammoth para máxima fidelidad
            try:
                import mammoth
                with open(file_path, "rb") as docx_file:
                    result = mammoth.convert_to_html(docx_file)
                    print(result.value)
            except ImportError:
                # Fallback robusto a python-docx (que sí está en site-packages)
                try:
                    from docx import Document
                    doc = Document(file_path)
                    paragraphs_html = "".join(f"<p>{p.text}</p>" for p in doc.paragraphs if p.text.strip())
                    warning = "<b>[Aviso: mammoth no instalado en python_embed. Mostrando texto sin formato complejo]</b><br><br>"
                    print(f"{warning}{paragraphs_html}")
                except Exception as e:
                    # Fallback de emergencia 100% nativo (sin dependencias lxml, zipfile + ElementTree estándar)
                    try:
                        import zipfile
                        import xml.etree.ElementTree as ET
                        
                        paragraphs = []
                        with zipfile.ZipFile(file_path) as docx_zip:
                            xml_content = docx_zip.read('word/document.xml')
                            root = ET.fromstring(xml_content)
                            
                            namespaces = {'w': 'http://schemas.openxmlformats.org/wordprocessingml/2006/main'}
                            for p_elem in root.findall('.//w:p', namespaces):
                                text_runs = [t.text for t in p_elem.findall('.//w:t', namespaces) if t.text]
                                p_text = "".join(text_runs)
                                if p_text.strip():
                                    paragraphs.append(p_text)
                                    
                        paragraphs_html = "".join(f"<p>{p}</p>" for p in paragraphs)
                        warning = "<b>[Aviso: Leyendo documento Word mediante motor nativo offline (sin lxml)]</b><br><br>"
                        print(f"{warning}{paragraphs_html}")
                    except Exception as e_native:
                        print(f"<span style='color:red;'>Error al leer documento Word: {str(e_native)} (Fallo lxml/python-docx: {str(e)})</span>")

        elif ext in ['.txt', '.log', '.json']:
            with open(file_path, 'r', encoding='utf-8', errors='ignore') as f:
                content = f.read(10000)
                # Escapar caracteres HTML básicos para evitar roturas
                content_escaped = content.replace('&', '&amp;').replace('<', '&lt;').replace('>', '&gt;')
                print(f"<pre style='color:#a9dbcf; white-space:pre-wrap; font-family: monospace; font-size: 13px; text-align: left; background: #0d1117; padding: 15px; border-radius: 6px;'>{content_escaped}</pre>")

        else:
            print("<div style='text-align:center; padding:20px;'>Vista previa no disponible para este formato. Descárgalo para verlo.</div>")

    except Exception as e:
        print(f"<span style='color:red;'>Error procesando vista previa: {str(e)}</span>")

if __name__ == '__main__':
    main()
