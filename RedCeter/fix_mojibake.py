import codecs

def fix_mojibake(filepath):
    with open(filepath, 'r', encoding='utf-8', errors='ignore') as f:
        text = f.read()
    
    # We will do literal replacements for the common ones
    replacements = {
        'ðŸŒ ': '🌐',
        'â˜Žï¸ ': '☎️',
        'ðŸ“§': '📧',
        'âŒ›': '⏳',
        'âœ…': '✅',
        'â Œ': '❌',
        'ðŸ”„': '🔄',
        'ðŸ‘¤': '👤',
        'ðŸ”‘': '🔑',
        'ðŸ“¡': '📡',
        'ðŸ”Œ': '🔌',
        'âš ï¸ ': '⚠️',
        'âœ ï¸ ': '✏️',
        'ðŸ” ': '🔍',
        'ðŸ’¾': '💾',
        'ðŸ” ': '🔐',
        'ðŸ ¡': '🏠',
        'ðŸ—ºï¸ ': '🗺️',
        '⚙ï¸ ': '⚙️',
        'ðŸŒ ': '🌍',
        'ðŸŸ¢': '🟢',
        'ðŸŸ ': '🟠',
        'ðŸ”´': '🔴',
        '🌙': '🌙',
        '☀️': '☀️',
        '?? Modo': '🌙 Modo'
    }
    
    for k, v in replacements.items():
        text = text.replace(k, v)
        
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(text)
    print("Fixed mojibake in", filepath)

if __name__ == '__main__':
    fix_mojibake('public/app.js')
