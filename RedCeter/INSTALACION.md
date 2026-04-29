# 📋 INSTRUCCIONES DE INSTALACIÓN - Windows 11

## 1️⃣ REQUISITOS PREVIOS
Instala en este orden:

### A) Node.js (Incluye npm)
- Descarga: https://nodejs.org/ (LTS - Long Term Support)
- Versión recomendada: 18.x o 20.x
- Durante la instalación, marca: ✓ Add to PATH
- Reinicia el equipo después de instalar

### B) Python (Opcional - Solo para test_conexion_smb.py)
- Descarga: https://www.python.org/
- Versión recomendada: 3.9+
- Durante la instalación, marca: ✓ Add Python to PATH
- No es necesario para que funcione el servidor

---

## 2️⃣ VERIFICAR INSTALACIÓN
Abre PowerShell o Cmd y ejecuta:

```bash
node --version
npm --version
```

Deberían mostrar versiones (ej: v18.16.0, v9.8.1)

---

## 3️⃣ CONFIGURAR EL PROYECTO
1. Abre PowerShell en la carpeta: `Remotas\estacionesJPIT\remodelado\`
2. Ejecuta:
```bash
npm install
```

---

## 4️⃣ INICIAR EL SERVIDOR
En PowerShell (en la carpeta remodelado):

```bash
node servidor.js
```

O usa el archivo `.bat`:
```bash
iniciar_todo.bat
```

Deberías ver:
```
📡 Servidor NOC activo en http://localhost:3000
```

---

## 5️⃣ ACCEDER A LA APLICACIÓN
- Abre navegador: http://localhost:3000
- Si ves el mapa, ¡todo funciona! ✓

---

## � CARACTERÍSTICAS PRINCIPALES

### 📍 Pestaña "Mapa y Estado"
- Visualización en tiempo real de todas las estaciones en un mapa
- Marcadores de colores: 🟢 OK | 🟠 Alerta | 🔴 Caída
- **Filtrado**: Busca por nombre, IP, provincia, ciudad, etc.
  - El contador muestra cuántas estaciones coinciden con el filtro
  - Los marcadores se atenúan cuando no cumplen el filtro

### 📊 Pestaña "Administración DB"
- Tabla completa de estaciones del Excel
- **Filtrado en tiempo real**: Busca mientras escribes
- El contador muestra registros visibles vs. totales
- Editar registros directamente desde la interfaz
- Descargar el Excel actualizado

### ✅ Información por estación
Cuando haces clic en un marcador del mapa:
- 📡 Red (Ping) - Conectividad básica
- 💻 RDP - Acceso remoto (Puerto 3389)
- 👁️ VNC - Visualización remota (Puerto 5900)
- 💾 Grabación - Estado de Argus en SMB

### 🎨 Estados de las estaciones
- 🟢 **Verde (Todo OK)**: Ping OK + RDP OK + VNC OK
- 🟠 **Naranja (Conexión Limitada)**: Ping OK + solo RDP o solo VNC (falta uno)
- 🔴 **Rojo (Caído)**: Sin ping O sin RDP ni VNC
- 💾 **Morado (Grabando)**: Estaciones que están grabando datos (solo en dashboard)
- 🔌 Telemando - Control de puertos PDU (si disponible)

---

## �🔧 SOLUCIÓN DE PROBLEMAS

### "node no es reconocido"
- Reinicia PowerShell/Cmd
- O reinicia el equipo completo

### "Puerto 3000 ya está en uso"
```bash
netstat -ano | findstr :3000
taskkill /PID [PID] /F
```

### "ERR_CONNECTION_REFUSED"
- Verifica que el servidor está corriendo
- Comprueba que `servidor.js` está en la carpeta correcta

### "❌ Sin grabaciones" en SMB
- El servidor no puede acceder a las carpetas compartidas SMB
- Verifica la conectividad a las IPs (192.168.29.x)
- Prueba con: `python test_conexion_smb.py`

---

## � SISTEMA DE AUTENTICACIÓN

### Usuarios y Roles
- **Admin**: Acceso completo a mapa, edición de Excel y configuración
- **Viewer**: Solo puede ver el mapa y hacer comprobaciones

### Archivo de Configuración
- `config.json`: Contiene la ruta al archivo de usuarios
- `config/usuarios.json`: Lista de usuarios (temporalmente JSON, después Excel)

### Credenciales de Prueba
- **Admin**: usuario: `admin`, contraseña: `admin123`
- **Viewer**: usuario: `visor`, contraseña: `visor123`

### Funcionalidades por Rol
- **Sin login**: Vista limitada (solo mapa de solo lectura)
- **Viewer**: Puede ver mapa y hacer comprobaciones
- **Admin**: Acceso completo a edición y administración

---

## 📂 NUEVOS CAMPOS EN EXCEL

El Excel de estaciones ahora incluye campos adicionales:

- `usuario telemando`: Usuario para acceso telemando
- `contraseña telemando`: Contraseña para telemando  
- `usuario sai`: Usuario para SAI
- `contraseña sai`: Contraseña para SAI

---

## 🎨 Estados de las estaciones
Cuando el servidor esté corriendo:
1. ✓ Abre http://localhost:3000
2. ✓ Haz clic en una estación
3. ✓ Verifica que muestre estados (Ping, RDP, VNC, Grabación)
4. ✓ Si hay telemando, verifica que muestre puertos ON/OFF

---

**¿Necesitas ayuda?** Copia los logs de error de PowerShell/consola del navegador (F12)
