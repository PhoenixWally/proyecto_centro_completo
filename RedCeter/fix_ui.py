import os

with open('public/app.js', 'r', encoding='utf-8') as f:
    text = f.read()

# Emojis currupts to fix
text = text.replace('â ³', '⏳')
text = text.replace('ðŸŒ ', '🌐')
text = text.replace('âœ…', '✅')
text = text.replace('â Œ', '❌')
text = text.replace('âœ ï¸ ', '✏️')
text = text.replace('GrabaciÃ³n', 'Grabación')

# Colors in charts
text = text.replace('fill="rgba(255,255,255,0.4)"', 'fill="var(--text-secondary)"')
text = text.replace('stroke="rgba(255,255,255,0.05)"', 'stroke="rgba(128,128,128,0.2)"')
text = text.replace('stroke="rgba(255,255,255,0.1)"', 'stroke="rgba(128,128,128,0.2)"')
text = text.replace('fill="rgba(255,255,255,0.3)"', 'fill="var(--text-secondary)"')
text = text.replace('stroke="rgba(255,255,255,0.2)"', 'stroke="rgba(128,128,128,0.3)"')

# Inject fields
old_def = 'const ip = findValueInRow(estacion, ["ip estacion", "ip pc", "ip"]) || "Sin IP";'
new_def = old_def + '\n        const telefono = findValueInRow(estacion, ["telefono jpit", "telefono", "tlf"]) || "No disponible";\n        const correo = findValueInRow(estacion, ["correo jpit", "correo", "email"]) || "No disponible";'
text = text.replace(old_def, new_def)

old_ui = '<div><b>🌐 IP:</b> ${ip}</div>'
new_ui = '''<div style="background:rgba(0,0,0,0.2); padding:8px; border-radius:6px; border:1px solid rgba(255,255,255,0.05);">
                            <div><b>☎️ Teléfono JPIT:</b> ${telefono}</div>
                            <div><b>📧 Correo JPIT:</b> ${correo}</div>
                            <div style="margin-top:6px; padding-top:6px; border-top:1px dashed rgba(255,255,255,0.1);"><b>🌐 IP Nodo:</b> ${ip}</div>
                        </div>'''
text = text.replace(old_ui, new_ui)

with open('public/app.js', 'w', encoding='utf-8') as f:
    f.write(text)
