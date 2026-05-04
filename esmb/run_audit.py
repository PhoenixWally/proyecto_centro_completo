import os
import subprocess
import re

devices_path = r'd:\jpit\Remotas\Proyecto completo\esmb\devices'
output_file = r'd:\jpit\Remotas\Proyecto completo\esmb\argus_commands_audit.md'

def get_strings(file_path):
    try:
        # Usamos el binario strings.exe si está en el path, si no, lo simulamos
        result = subprocess.run(['strings', file_path], capture_output=True, text=True, errors='ignore')
        return result.stdout.splitlines()
    except:
        # Fallback simple si strings no funciona
        with open(file_path, 'rb') as f:
            content = f.read()
            return re.findall(b'[ -~]{4,}', content)

def extract_commands(strings_list):
    scpi_pattern = re.compile(r'[:\*][a-zA-Z0-9_:]{3,}')
    commands = set()
    for s in strings_list:
        if isinstance(s, bytes): s = s.decode('ascii', errors='ignore')
        matches = scpi_pattern.findall(s)
        for m in matches:
            if len(m) > 4:
                commands.add(m.strip(':'))
    return sorted(list(commands))

audit_data = {}

print("Iniciando auditoría de comandos en DLLs de Argus...")

for root, dirs, files in os.walk(devices_path):
    for file in files:
        if file.endswith('.dll') or file.endswith('.UMF'):
            file_path = os.path.join(root, file)
            print(f"Procesando {file}...")
            strs = get_strings(file_path)
            cmds = extract_commands(strs)
            if cmds:
                audit_data[file] = cmds

with open(output_file, 'w', encoding='utf-8') as f:
    f.write("# Auditoría Técnica de Comandos Argus (R&S Devices)\n\n")
    f.write("Este documento resume los comandos SCPI y funciones detectadas en los drivers de Argus encontrados en el directorio de dispositivos.\n\n")
    
    for file, cmds in audit_data.items():
        f.write(f"## {file}\n")
        f.write("```scpi\n")
        for c in cmds:
            f.write(f"{c}\n")
        f.write("```\n\n")

print(f"Auditoría completada. Resultados en {output_file}")
