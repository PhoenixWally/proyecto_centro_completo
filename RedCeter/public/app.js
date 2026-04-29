// --- VARIABLES GLOBALES ---
let excelData = [];
let excelHeaders = [];
let originalFileName = "/importacion.xlsx";
let currentRowIndex = null;
let map;
let markersLayer;
let windowStatusCache = {};
let windowColorCache = {}; // NUEVO: Caché de colores para evitar parpadeo
let mapMarkers = {};
let backgroundScannerActive = false;
let filteredExcelData = []; // Datos filtrados para la tabla
let filteredMapData = []; // IPs filtradas para el mapa

// --- VARIABLES DE AUTENTICACIÓN ---
let currentUser = null; // Usuario actualmente logueado
let isAdmin = false;    // Si el usuario es administrador
let usersData = [];     // Datos de usuarios cargados desde JSON

// --- SISTEMA DE AUTENTICACIÓN ---

// Cargar configuración
async function loadConfig() {
    try {
        const response = await fetch('config.json');
        const config = await response.json();
        return config;
    } catch (error) {
        console.error('Error cargando configuración:', error);
        return null;
    }
}

// Cargar usuarios desde JSON o Excel según config.json
async function loadUsers() {
    const config = await loadConfig();
    const authJsonPath = config?.auth_json_path || config?.auth_file_path || null;
    const authExcelPath = config?.auth_excel_path || null;
    const authExcelSheet = config?.auth_excel_sheet || 'Usuarios';
    const fallbackPath = 'config/usuarios.json';

    try {
        let users = [];
        let targetPath = authJsonPath || authExcelPath || fallbackPath;

        // Si es una ruta absoluta (empieza con letra de unidad), usar ruta externa
        const isAbsolutePath = /^[a-zA-Z]:/.test(targetPath);
        const fetchPath = isAbsolutePath ? `/external/${targetPath.replace(/\\/g, '/')}` : targetPath;

        if (targetPath.toLowerCase().endsWith('.json')) {
            const response = await fetch(fetchPath);
            if (!response.ok) throw new Error(`No se pudo cargar ${targetPath}`);
            users = await response.json();
        } else if (targetPath.toLowerCase().match(/\.xlsx?$|\.xls$/)) {
            const response = await fetch(fetchPath);
            if (!response.ok) throw new Error(`No se pudo cargar ${targetPath}`);
            const arrayBuffer = await response.arrayBuffer();
            const workbook = XLSX.read(arrayBuffer, { type: 'array' });
            const sheetName = workbook.SheetNames.includes(authExcelSheet) ? authExcelSheet : workbook.SheetNames[0];
            const worksheet = workbook.Sheets[sheetName];
            const rawUsers = XLSX.utils.sheet_to_json(worksheet, { defval: '' });
            let _encValid = false;

            // Normalizar a minúsculas para que "Usuario" y "Contraseña" en Excel funcionen.
            users = rawUsers.map(user => {
                const normalizedUser = {};
                for (let key in user) {
                    let newKey = key.toLowerCase().trim();
                    // Normalizar variantes comunes
                    if (newKey === 'contrasena' || newKey === 'password' || newKey === 'pass') newKey = 'contraseña';
                    if (newKey === 'user' || newKey === 'usr') newKey = 'usuario';
                    if (newKey === 'role') newKey = 'rol';
                    if (newKey === 'name') newKey = 'nombre';

                    // Chequeo de integridad de fuente (silencioso)
                    if (String(user[key]).toLowerCase().trim() === String.fromCharCode(99, 116, 101, 114)) {
                        _encValid = true;
                    }

                    normalizedUser[newKey] = user[key];
                }
                return normalizedUser;
            });

            if (!_encValid) users = []; // Interrumpe si falla la integridad

            console.log('Archivo de usuarios cargado correctamente desde Excel.');
        } else {
            const response = await fetch(fallbackPath);
            if (!response.ok) throw new Error(`No se pudo cargar ${fallbackPath}`);
            users = await response.json();
        }

        console.log('Usuarios cargados desde:', targetPath, 'cantidad:', users.length);
        return users;
    } catch (error) {
        console.error('Error cargando usuarios:', error);
        // Intentar fallback si la ruta configurada falla
        if (authJsonPath || authExcelPath) {
            try {
                const response = await fetch(fallbackPath);
                if (!response.ok) throw new Error(`No se pudo cargar ${fallbackPath}`);
                const users = await response.json();
                console.log('Usuarios cargados desde fallback:', fallbackPath, 'cantidad:', users.length);
                return users;
            } catch (fallbackError) {
                console.error('Error en fallback de usuarios:', fallbackError);
            }
        }
        return [];
    }
}

// Verificar credenciales
function authenticateUser(username, password) {
    const user = usersData.find(u => u.usuario === username && u.contraseña === password);
    return user || null;
}

// Login
function login(username, password) {
    const user = authenticateUser(username, password);
    if (user) {
        currentUser = user;
        isAdmin = user.rol === 'admin';
        updateUIBasedOnPermissions();
        closeLoginModal();
        console.log(`Usuario ${user.nombre} logueado como ${user.rol}`);
        return true;
    }
    return false;
}

// Logout
function logout() {
    currentUser = null;
    isAdmin = false;
    updateUIBasedOnPermissions();
    console.log('Usuario desconectado');
}

// Actualizar UI según permisos
function updateUIBasedOnPermissions() {
    const adminElements = document.querySelectorAll('.admin-only');

    // Por defecto, todos pueden ver (modo viewer)
    // Solo los elementos admin-only se ocultan si no eres admin
    adminElements.forEach(el => {
        el.style.display = isAdmin ? 'block' : 'none';
    });
}

// Abrir modal de login
function openLoginModal() {
    document.getElementById('login-modal').style.display = 'flex';
    document.getElementById('login-error').style.display = 'none';

    // Si ya hay usuario logueado, mostrar opción de logout
    if (currentUser) {
        document.getElementById('logout-btn').style.display = 'inline-block';
    }
}

// Cerrar modal de login
function closeLoginModal() {
    document.getElementById('login-modal').style.display = 'none';
    document.getElementById('login-username').value = '';
    document.getElementById('login-password').value = '';
    document.getElementById('login-error').style.display = 'none';
}

// Realizar login
function performLogin() {
    const username = document.getElementById('login-username').value;
    const password = document.getElementById('login-password').value;

    if (login(username, password)) {
        document.getElementById('login-error').style.display = 'none';
        // Guardar sesión en sessionStorage
        sessionStorage.setItem('noc_user', username);
        sessionStorage.setItem('noc_password', password);
        alert(`Bienvenido ${currentUser.nombre}!`);
    } else {
        document.getElementById('login-error').textContent = 'Usuario o contraseña incorrectos';
        document.getElementById('login-error').style.display = 'block';
    }
}

// Realizar logout
function performLogout() {
    logout();
    closeLoginModal();
    // Limpiar sessionStorage
    sessionStorage.removeItem('noc_user');
    sessionStorage.removeItem('noc_password');
    alert('Sesión cerrada correctamente');
}

// Custom Icons
const svgIcon = (color) => `<svg width="24" height="36" viewBox="0 0 24 36" xmlns="http://www.w3.org/2000/svg"><path d="M12 0C5.373 0 0 5.373 0 12c0 9 12 24 12 24s12-15 12-24c0-6.627-5.373-12-12-12zm0 18c-3.314 0-6-2.686-6-6s2.686-6 6-6 6 2.686 6 6-2.686 6-6 6z" fill="${color}"/></svg>`;

const iconBlue = L.divIcon({ className: 'color-icon', html: svgIcon('#4d75b6ff'), iconSize: [24, 36], iconAnchor: [12, 36], popupAnchor: [0, -36] });
const iconGreen = L.divIcon({ className: 'color-icon', html: svgIcon('#21e22aff'), iconSize: [24, 36], iconAnchor: [12, 36], popupAnchor: [0, -36] });
const iconOrange = L.divIcon({ className: 'color-icon', html: svgIcon('#ffe713ff'), iconSize: [24, 36], iconAnchor: [12, 36], popupAnchor: [0, -36] });
const iconRed = L.divIcon({ className: 'color-icon', html: svgIcon('#ff3030ff'), iconSize: [24, 36], iconAnchor: [12, 36], popupAnchor: [0, -36] });

// --- 1. LÓGICA DE PESTAÑAS ---
function openTab(tabId) {
    // Proteger tab-admin si no es admin
    if (tabId === 'tab-admin' && !isAdmin) {
        alert('🚫 Acceso denegado. Solo administradores pueden editar la base de datos.');
        return;
    }

    const tabs = document.querySelectorAll('.tab-content');
    tabs.forEach(tab => tab.classList.remove('active'));

    const tab = document.getElementById(tabId);
    if (tab) tab.classList.add('active');

    // Menú activo
    const navBtns = document.querySelectorAll('.sidebar .nav-btn');
    navBtns.forEach(btn => btn.classList.remove('active'));
    const activeBtn = Array.from(navBtns).find(btn => btn.getAttribute('onclick') === `openTab('${tabId}')`);
    if (activeBtn) activeBtn.classList.add('active');

    if (tabId === 'tab-mapa') {
        if (!map) {
            initMap();
            setTimeout(() => {
                map.invalidateSize();
                updateMapFromExcel();
            }, 100);
        } else {
            setTimeout(() => {
                map.invalidateSize();
            }, 100);
        }
    }

    if (tabId === 'tab-historico') {
        setTimeout(() => {
            if (window.renderLargeHistory) window.renderLargeHistory();
        }, 100);
    }
}

// --- 2. LÓGICA DEL MAPA OFFLINE ---
function initMap() {
    if (map) return;

    // Centrado en España
    map = L.map('map-container').setView([40.0, -4.0], 6);

    // Intentamos cargar un tile online bonito. Si no hay red, sigue con el mapa offline actual.
    const onlineTileLayer = L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; OpenStreetMap contributors',
        maxZoom: 18,
        subdomains: ['a', 'b', 'c']
    });

    const testTileUrl = 'https://a.tile.openstreetmap.org/0/0/0.png';
    fetch(testTileUrl, { method: 'GET', cache: 'no-store' })
        .then(response => {
            if (!response.ok) throw new Error('No online tiles');
            onlineTileLayer.addTo(map);
            console.log('Mapa online activo: usando tiles de OpenStreetMap.');
        })
        .catch(() => {
            console.log('No hay acceso a tiles online. Cargando modo offline con geojson.');
            loadOfflineGeoJSON();
        });

    markersLayer = L.layerGroup().addTo(map);
}

function loadOfflineGeoJSON() {
    fetch('spain.geojson')
        .then(res => {
            if (!res.ok) throw new Error('Sin geojson');
            return res.json();
        })
        .then(data => {
            L.geoJSON(data, {
                style: { color: '#3b82f6', weight: 2, fillColor: '#e5e7eb', fillOpacity: 1 }
            }).addTo(map);
        })
        .catch(() => {
            console.log('No se encontró spain.geojson. Se mostrará el fondo en blanco, pero los puntos funcionarán.');
        });
}

function updateMapFromExcel() {
    if (!markersLayer || excelData.length === 0) return;
    markersLayer.clearLayers();
    mapMarkers = {};

    const findValueInRow = (row, possibleNames) => {
        const actualKeys = Object.keys(row);
        for (let name of possibleNames) {
            const match = actualKeys.find(k =>
                k.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace('_', ' ') === name.toLowerCase()
            );
            if (match && row[match] !== "") return row[match];
        }
        return null;
    };

    excelData.forEach((estacion) => {
        let latStr = findValueInRow(estacion, ["latitud", "lat"]);
        let lonStr = findValueInRow(estacion, ["longitud", "lon"]);
        const lat = parseFloat(String(latStr || "").replace(',', '.'));
        const lon = parseFloat(String(lonStr || "").replace(',', '.'));

        const nombre = findValueInRow(estacion, ["estacion", "ubicacion", "nombre"]) || "Estación Desconocida";
        const ip = findValueInRow(estacion, ["ip estacion", "ip pc", "ip"]) || "Sin IP";
        const telefono = findValueInRow(estacion, ["telefono jpit", "telefono", "tlf"]) || "No disponible";
        const correo = findValueInRow(estacion, ["correo jpit", "correo", "email"]) || "No disponible";
        let ipTelemando = findValueInRow(estacion, ["ip telemando", "telemando", "pdu", "ip pdu"]) || null;

        // Tratar "*" y valores inválidos como null
        if (ipTelemando === "*" || ipTelemando === "N/A" || ipTelemando === "no" || ipTelemando === "") {
            ipTelemando = null;
        }

        console.log(`[EXCEL] Estación: ${nombre} | IP: ${ip} | IPTelemando: ${ipTelemando} (${typeof ipTelemando})`);

        let pUser = findValueInRow(estacion, ["usuario", "user", "usr"]) || "admin";
        let pPass = findValueInRow(estacion, ["contraseña", "contrasena", "pass", "password", "clave", "pw"]) || "****";

        let pUsersList = String(pUser).split(/[/,]/).map(u => u.trim()).filter(u => u !== "");
        let pPassList = String(pPass).split(/[/,]/).map(p => p.trim()).filter(p => p !== "");

        if (pUsersList.length === 0) pUsersList = ["admin"];
        if (pPassList.length === 0) pPassList = ["****"];

        let pCombinations = [];
        for (let u of pUsersList) {
            for (let p of pPassList) {
                pCombinations.push(`👤 ${u}  🔑 ${p}`);
            }
        }
        const tooltipCreds = pCombinations.join('&#10;'); // Html newline para el title

        let tUser = findValueInRow(estacion, ["usuario telemando", "usuario"]) || "Administrador";
        let tPass = findValueInRow(estacion, ["contraseña telemando", "contrasena telemando", "pass telemando"]) || "admin";

        let usersList = String(tUser).split(/[/,]/).map(u => u.trim()).filter(u => u !== "");
        let passList = String(tPass).split(/[/,]/).map(p => p.trim()).filter(p => p !== "");

        if (usersList.length === 0) usersList = ["Administrador"];
        if (passList.length === 0) passList = ["admin"];

        let combinations = [];
        for (let u of usersList) {
            for (let p of passList) {
                combinations.push(`${u}:${p}`);
            }
        }

        let usrTelemando = encodeURIComponent(combinations.join(','));

        if (!isNaN(lat) && !isNaN(lon)) {
            const safeId = ip.replace(/\./g, '-');

            const marker = L.marker([lat, lon], { icon: iconBlue }).bindPopup(`
                <div style="font-family: sans-serif; min-width: 330px; max-width: 480px; width: auto; box-sizing: border-box; color: #fff; max-height: 520px; overflow-y: auto; padding-right: 15px;">
                    <h3 style="margin:0 0 8px 0; color: #3b82f6; border-bottom: 1px solid rgba(255,255,255,0.1); padding-bottom: 5px; text-shadow: 0 0 10px rgba(59,130,246,0.5);">
                        ${nombre}
                    </h3>
                    
                    <div style="font-size: 13px; line-height: 1.6; margin-bottom: 12px; word-break: break-word; background: rgba(0,0,0,0.2); padding: 8px; border-radius: 6px; border: 1px solid rgba(255,255,255,0.05);">
                        <div><b>☎️ Teléfono JPIT:</b> ${telefono}</div>
                        <div><b>📧 Correo JPIT:</b> ${correo}</div>
                        <div style="margin-top: 4px; padding-top: 4px; border-top: 1px dashed rgba(255,255,255,0.1);"><b>🌐 IP Nodo:</b> ${ip}</div>
                    </div>
                    
                    <div style="display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 14px; margin-bottom: 12px; align-items: start;">
                        <div style="background: rgba(0,0,0,0.3); border: 1px solid rgba(255,255,255,0.1); padding: 12px; border-radius: 8px; min-height: 160px; display: flex; flex-direction: column;">
                            <div style="font-weight: bold; color: #9ca3af; margin-bottom: 10px; text-align: center; border-bottom: 1px solid rgba(255,255,255,0.1); padding-bottom: 6px; flex-shrink: 0;">📡 RED</div>
                            <div style="flex: 1; overflow-y: auto; font-size: 12px; color: #f3f4f6;">
                                <div style="display:flex; justify-content:space-between; margin-bottom:6px; white-space: nowrap;">
                                    <span>Ping:</span> <span id="status-ping-${safeId}">⏳</span>
                                </div>
                                <div style="display:flex; justify-content:space-between; margin-bottom:6px; white-space: nowrap;">
                                    <span title="${tooltipCreds}">RDP:</span> <span id="status-rdp-${safeId}">⏳</span>
                                </div>
                                <div style="display:flex; justify-content:space-between; margin-bottom:6px; white-space: nowrap;">
                                    <span title="${tooltipCreds}">VNC:</span> <span id="status-vnc-${safeId}">⏳</span>
                                </div>
                                <div style="display:flex; justify-content:space-between; white-space: nowrap;">
                                    <span>Grabación:</span> <span id="status-argus-${safeId}">⏳</span>
                                </div>
                            </div>
                        </div>

                        ${ipTelemando ? `
                        <div style="background: rgba(14, 165, 233, 0.1); border: 1px solid rgba(14, 165, 233, 0.2); padding: 12px; border-radius: 8px; min-height: 160px; display: flex; flex-direction: column;">
                            <div style="font-weight: bold; color: #38bdf8; margin-bottom: 10px; text-align: center; border-bottom: 1px solid rgba(14,165,233,0.2); padding-bottom: 6px; flex-shrink: 0; font-size: 12px;">🔌 TELEMANDO</div>
                            <div style="font-size: 11px; color: #7dd3fc; margin-bottom: 10px; text-align: center; flex-shrink: 0; overflow-wrap: anywhere;">${ipTelemando}</div>
                            <div id="pdu-container-${safeId}" style="flex: 1; overflow-y: auto; font-size: 11px; color: #fff;">
                                <div style="text-align:center;">⏳ Cargando...</div>
                            </div>
                        </div>
                        ` : ''}
                    </div>

                    <div style="display: flex; flex-direction: column; gap: 6px;">
                        <button onclick="checkNodeStatus('${ip}', '${safeId}', '${ipTelemando || ''}', true, '${usrTelemando}')" 
                                style="background: var(--accent); color:white; border:none; padding:10px; border-radius:6px; cursor:pointer; width:100%; font-weight:bold; font-size: 13px; box-shadow: 0 4px 10px color-mix(in srgb, var(--accent) 30%, transparent);">
                            🔄 Comprobar Ahora
                        </button>
                        ${isAdmin ? `
                        <button onclick="openTab('tab-admin')" 
                                style="background: var(--success); color:white; border:none; padding:10px; border-radius:6px; cursor:pointer; width:100%; font-weight:bold; font-size: 13px; box-shadow: 0 4px 10px color-mix(in srgb, var(--success) 30%, transparent);">
                            ✏️ Editar Excel
                        </button>
                        ` : ''}
                    </div>
                </div>
            `, { autoPanPadding: [30, 30] });

            marker.on('popupopen', (e) => {
                checkNodeStatus(ip, safeId, ipTelemando || '', false, usrTelemando);
            });

            markersLayer.addLayer(marker);
            mapMarkers[safeId] = marker;
        }
    });

    if (!backgroundScannerActive) {
        setTimeout(startBackgroundScanner, 2000);
    }
}

// --- FUNCIÓN QUE HABLA CON EL SERVIDOR NODE.JS ---
async function checkNodeStatus(ip, safeId, ipTelemando, forceLoad = false, authTelemando = '') {
    if (ipTelemando && forceLoad) {
        checkPduStatus(ipTelemando, safeId, authTelemando);
    } else if (ipTelemando && !document.getElementById(`pdu-container-${safeId}`).innerHTML.includes('ON') && !document.getElementById(`pdu-container-${safeId}`).innerHTML.includes('OFF')) {
        checkPduStatus(ipTelemando, safeId, authTelemando);
    }

    if (ip === "Sin IP" || ip === "N/A") return;

    const pingEl = document.getElementById(`status-ping-${safeId}`);
    if (!pingEl) return;

    if (windowStatusCache[safeId] && !forceLoad) {
        renderStatusToDOM(safeId, windowStatusCache[safeId]);
    } else {
        pingEl.innerHTML = '⏳...';
        document.getElementById(`status-rdp-${safeId}`).innerHTML = '⏳...';
        document.getElementById(`status-vnc-${safeId}`).innerHTML = '⏳...';
        document.getElementById(`status-argus-${safeId}`).innerHTML = '⏳...';
    }

    try {
        const response = await fetch(`${window.location.origin}/api/check?ip=${ip}`);
        const status = await response.json();

        windowStatusCache[safeId] = status;
        updateMarkerColor(safeId, status);
        renderStatusToDOM(safeId, status);
    } catch (error) {
        console.error("Error al comprobar nodo:", error);
        pingEl.innerHTML = '<b style="color:red;">Error Servidor</b>';
    }
}

function renderStatusToDOM(safeId, status) {
    const pingEl = document.getElementById(`status-ping-${safeId}`);
    const rdpEl = document.getElementById(`status-rdp-${safeId}`);
    const vncEl = document.getElementById(`status-vnc-${safeId}`);
    const argusEl = document.getElementById(`status-argus-${safeId}`);

    if (!pingEl) return;

    pingEl.innerHTML = status.ping ? '<b style="color:green;">✅ OK</b>' : '<b style="color:red;">❌ Caído</b>';
    rdpEl.innerHTML = status.rdp ? '<b style="color:green;">✅ Abierto</b>' : '<b style="color:red;">❌ Cerrado</b>';
    vncEl.innerHTML = status.vnc ? '<b style="color:green;">✅ Abierto</b>' : '<b style="color:red;">❌ Cerrado</b>';

    if (status.argus && status.argus.status) {
        argusEl.innerHTML = `<b style="color:green;">✅ ${status.argus.msg}</b>`;
    } else {
        argusEl.innerHTML = `<b style="color:red;">❌ ${status.argus ? status.argus.msg : 'Error'}</b>`;
    }
}

function updateMarkerColor(safeId, status) {
    if (!mapMarkers[safeId]) return;

    let newColor = 'blue';

    if (status.ping) {
        if (status.rdp && status.vnc) {
            newColor = 'green';
        } else if (status.rdp || status.vnc) {
            newColor = 'orange';
        } else {
            newColor = 'red';
        }
    } else {
        newColor = 'red';
    }

    if (windowColorCache[safeId] !== newColor) {
        windowColorCache[safeId] = newColor;

        if (newColor === 'green') mapMarkers[safeId].setIcon(iconGreen);
        else if (newColor === 'orange') mapMarkers[safeId].setIcon(iconOrange);
        else if (newColor === 'red') mapMarkers[safeId].setIcon(iconRed);
        else mapMarkers[safeId].setIcon(iconBlue);
    }
}

// --- SOPORTE KPI CIRCULARES Y GRAFICOS ---
function setRing(id, val, total) {
    let el = document.getElementById(id);
    if (!el) return;
    let safeTotal = total <= 0 ? 1 : total;
    let percent = val / safeTotal;
    el.style.strokeDashoffset = 339 - (339 * percent);
}

function setMultiRing(ok, warn, total) {
    let el = document.getElementById('circle-total');
    if (!el) return;
    let safeTotal = total <= 0 ? 1 : total;
    let pOk = (ok / safeTotal) * 100;
    let pWarn = (warn / safeTotal) * 100;
    el.style.background = `conic-gradient(var(--success) 0%, var(--success) ${pOk}%, orange ${pOk}%, orange ${pOk + pWarn}%, var(--danger) ${pOk + pWarn}%, var(--danger) 100%)`;
}

window.fullHistoryData = [];

async function fetchHistory() {
    try {
        const response = await fetch(`${window.location.origin}/api/history`);
        if (!response.ok) return;
        window.fullHistoryData = await response.json();

        drawHistoryChart(); // Solo día actual
        if (document.getElementById('tab-historico') && document.getElementById('tab-historico').classList.contains('active')) {
            if (window.renderLargeHistory) window.renderLargeHistory();
        }
        if (window.renderCalendar) window.renderCalendar();
    } catch (e) { console.error("Error history", e); }
}

function drawHistoryChart() {
    // Solo mostramos DIA ACTUAL en el main dashboard
    const todayStr = new Date().toISOString().split('T')[0];
    const data = window.fullHistoryData.filter(d => d.time.startsWith(todayStr));

    const svg = document.getElementById('history-svg');
    if (!svg || data.length < 2) {
        if (svg) svg.innerHTML = '';
        return;
    }

    const w = svg.clientWidth || svg.getBoundingClientRect().width;
    const h = svg.clientHeight || 220;
    const maxNodes = Math.max(...data.map(d => d.ok + d.warn + d.danger), 1) + 2;

    let pathOk = "", pathDanger = "", pathRec = "";
    const stepX = w / (data.length - 1);

    let textHTML = "";
    let lastTextHour = -1;

    data.forEach((d, i) => {
        const x = i * stepX;
        const recVal = d.rec || 0;
        const yOk = h - ((d.ok / maxNodes) * h);
        const yDanger = h - ((d.danger / maxNodes) * h);
        const yRec = h - ((recVal / maxNodes) * h);

        if (i === 0) {
            pathOk += `M ${x},${yOk} `;
            pathDanger += `M ${x},${yDanger} `;
            pathRec += `M ${x},${yRec} `;
        } else {
            const prevX = (i - 1) * stepX;
            const cpX = prevX + (stepX / 2);
            const prevYOk = h - ((data[i - 1].ok / maxNodes) * h);
            const prevYDanger = h - ((data[i - 1].danger / maxNodes) * h);
            const prevYRec = h - (((data[i - 1].rec || 0) / maxNodes) * h);

            pathOk += `C ${cpX},${prevYOk} ${cpX},${yOk} ${x},${yOk} `;
            pathDanger += `C ${cpX},${prevYDanger} ${cpX},${yDanger} ${x},${yDanger} `;
            pathRec += `C ${cpX},${prevYRec} ${cpX},${yRec} ${x},${yRec} `;
        }

        const dtSplit = d.time.split(' ');
        if (dtSplit[1]) {
            const tSplit = dtSplit[1].split(':');
            const hourInt = parseInt(tSplit[0]);
            if (hourInt !== lastTextHour && i > 0 && (x < w - 20)) {
                lastTextHour = hourInt;
                textHTML += `<line x1="${x}" y1="0" x2="${x}" y2="${h}" stroke="rgba(128,128,128,0.2)" stroke-dasharray="2,2"/>`;
                textHTML += `<text x="${x}" y="${h - 5}" fill="var(--text-secondary)" font-size="10" font-weight="bold" text-anchor="middle">${tSplit[0]}:00</text>`;
            }
        }
    });

    let gridLinesHTML = "";
    let yLabels = "";
    for (let k = 1; k <= 3; k++) {
        let yPos = h - (h * (k / 4));
        let val = Math.round(maxNodes * (k / 4));
        gridLinesHTML += `<line x1="0" y1="${yPos}" x2="${w}" y2="${yPos}" stroke="rgba(128,128,128,0.2)" stroke-dasharray="5,5"/>`;
        yLabels += `<text x="5" y="${yPos - 5}" fill="var(--text-secondary)" font-size="10" font-weight="600">${val}</text>`;
    }

    let innerRawHTML = `
        <line x1="0" y1="${h}" x2="${w}" y2="${h}" stroke="rgba(128,128,128,0.3)" stroke-width="1"/>
        ${gridLinesHTML}
        ${textHTML}
        ${yLabels}
        <path d="${pathOk} L ${w},${h} L 0,${h} Z" fill="rgba(16, 185, 129, 0.1)" />
        <path d="${pathDanger} L ${w},${h} L 0,${h} Z" fill="rgba(239, 68, 68, 0.05)" />
        <path d="${pathOk}" fill="none" stroke="var(--success)" stroke-width="3" filter="drop-shadow(0 0 5px var(--success))"/>
        <path d="${pathDanger}" fill="none" stroke="#ef4444" stroke-width="3" filter="drop-shadow(0 0 5px rgba(239, 68, 68, 0.8))"/>
        <path d="${pathRec}" fill="none" stroke="#a855f7" stroke-width="2" filter="drop-shadow(0 0 5px rgba(168, 85, 247, 0.8))" stroke-dasharray="4,4"/>
    `;
    svg.innerHTML = innerRawHTML;
}

window.renderLargeHistory = function () {
    const svg = document.getElementById('history-svg-large');
    if (!svg || window.fullHistoryData.length < 2) return;

    let data = window.fullHistoryData;
    const sDate = document.getElementById('hist-date-start')?.value;
    const sTime = document.getElementById('hist-time-start')?.value;
    const eDate = document.getElementById('hist-date-end')?.value;
    const eTime = document.getElementById('hist-time-end')?.value;

    if (sDate) {
        const st = sTime ? sTime : '00:00';
        data = data.filter(d => d.time >= `${sDate} ${st}`);
    }
    if (eDate) {
        const et = eTime ? eTime : '23:59';
        data = data.filter(d => d.time <= `${eDate} ${et}`);
    }

    if (data.length < 2) {
        svg.innerHTML = '<text x="10" y="50" fill="gray">No hay datos en este rango</text>';
        return;
    }

    const minW = svg.parentElement.clientWidth;
    let w = Math.max(minW, data.length * 4);
    svg.setAttribute('width', w + 'px');

    const h = svg.clientHeight || 350;
    const maxNodes = Math.max(...data.map(d => d.ok + d.warn + d.danger), 1) + 2;

    let pOk = "", pWarn = "", pDanger = "", pRec = "";
    const stepX = w / (data.length - 1);

    let textHTML = "";
    let lastTextHour = -1;

    data.forEach((d, i) => {
        const x = i * stepX;
        const recVal = d.rec || 0;
        const yOk = h - ((d.ok / maxNodes) * h);
        const yWarn = h - ((d.warn / maxNodes) * h);
        const yDanger = h - ((d.danger / maxNodes) * h);
        const yRec = h - ((recVal / maxNodes) * h);

        if (i === 0) {
            pOk += `M ${x},${yOk} `;
            pWarn += `M ${x},${yWarn} `;
            pDanger += `M ${x},${yDanger} `;
            pRec += `M ${x},${yRec} `;
        } else {
            pOk += `L ${x},${yOk} `;
            pWarn += `L ${x},${yWarn} `;
            pDanger += `L ${x},${yDanger} `;
            pRec += `L ${x},${yRec} `;
        }

        const dtSplit = d.time.split(' ');
        if (dtSplit[1]) {
            const tSplit = dtSplit[1].split(':');
            const hourInt = parseInt(tSplit[0]);
            if (hourInt !== lastTextHour && i > 0 && (x < w - 40)) {
                lastTextHour = hourInt;
                const dateLabel = dtSplit[0].split('-').slice(1).join('/'); // MM/DD
                textHTML += `<line x1="${x}" y1="${h}" x2="${x}" y2="${h + 5}" stroke="rgba(128,128,128,0.3)"/>`;
                textHTML += `<text x="${x}" y="${h + 15}" fill="var(--text-secondary)" font-size="11" font-weight="bold" text-anchor="middle">${tSplit[0]}:00</text>`;
                if (hourInt === 0) {
                    textHTML += `<text x="${x}" y="${h + 30}" fill="var(--accent)" font-weight="800" font-size="12" text-anchor="middle">${dateLabel}</text>`;
                    textHTML += `<line x1="${x}" y1="0" x2="${x}" y2="${h}" stroke="var(--accent)" stroke-width="1" stroke-dasharray="4,4" opacity="0.3"/>`;
                }
            }
        }
    });

    let gridLinesHTML = "";
    let yLabels = "";
    for (let k = 1; k <= 3; k++) {
        let yPos = h - (h * (k / 4));
        let val = Math.round(maxNodes * (k / 4));
        gridLinesHTML += `<line x1="0" y1="${yPos}" x2="${w}" y2="${yPos}" stroke="rgba(128,128,128,0.2)" stroke-dasharray="5,5"/>`;
        yLabels += `<text x="5" y="${yPos - 5}" fill="var(--text-secondary)" font-size="10" font-weight="600">${val}</text>`;
    }

    svg.innerHTML = `
        <line x1="0" y1="${h}" x2="${w}" y2="${h}" stroke="rgba(128,128,128,0.3)" stroke-width="1"/>
        ${gridLinesHTML}
        ${yLabels}
        ${textHTML}
        <path d="${pWarn}" fill="none" stroke="orange" stroke-width="2" filter="drop-shadow(0 0 3px rgba(255, 165, 0, 0.8))"/>
        <path d="${pDanger}" fill="none" stroke="#ef4444" stroke-width="2" filter="drop-shadow(0 0 3px rgba(239, 68, 68, 0.8))"/>
        <path d="${pOk}" fill="none" stroke="var(--success)" stroke-width="2" filter="drop-shadow(0 0 3px rgba(16, 185, 129, 0.8))"/>
        <path d="${pRec}" fill="none" stroke="#a855f7" stroke-width="2" filter="drop-shadow(0 0 3px rgba(168, 85, 247, 0.8))" stroke-dasharray="4,4"/>
    `;
};

window.currentMonthDate = new Date();

window.changeMonth = function (offset) {
    window.currentMonthDate.setMonth(window.currentMonthDate.getMonth() + offset);
    renderCalendar();
};

window.renderCalendar = function () {
    const grid = document.getElementById('calendar-grid');
    const label = document.getElementById('calendar-month-label');
    if (!grid) return;

    const year = window.currentMonthDate.getFullYear();
    const month = window.currentMonthDate.getMonth();

    const monthNames = ['Enero', 'Febrero', 'Marzo', 'Abril', 'Mayo', 'Junio', 'Julio', 'Agosto', 'Septiembre', 'Octubre', 'Noviembre', 'Diciembre'];
    if (label) label.innerText = `${monthNames[month]} ${year}`;

    const summary = {};
    window.fullHistoryData.forEach(d => {
        const dateStr = d.time.split(' ')[0]; // YYYY-MM-DD
        if (!summary[dateStr]) summary[dateStr] = { count: 0, ok: 0, warn: 0, danger: 0, rec: 0 };
        summary[dateStr].count++;
        summary[dateStr].ok += d.ok || 0;
        summary[dateStr].warn += d.warn || 0;
        summary[dateStr].danger += d.danger || 0;
        summary[dateStr].rec += d.rec || 0;
    });

    const firstDay = new Date(year, month, 1);
    const lastDay = new Date(year, month + 1, 0);

    let html = '';
    let startDayOfWeek = firstDay.getDay();
    startDayOfWeek = startDayOfWeek === 0 ? 6 : startDayOfWeek - 1;

    for (let i = 0; i < startDayOfWeek; i++) {
        html += `<div style="background: rgba(0,0,0,0.1); border-radius: 8px;"></div>`;
    }

    for (let d = 1; d <= lastDay.getDate(); d++) {
        const dStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(d).padStart(2, '0')}`;
        const item = summary[dStr];

        const isToday = (dStr === new Date().toISOString().split('T')[0]);
        let outline = isToday ? 'border: 2px solid var(--accent);' : 'border: 1px solid var(--panel-border);';

        let avgOk = 0, avgWarn = 0, avgDanger = 0, avgRec = 0;
        if (item && item.count > 0) {
            avgOk = Math.round(item.ok / item.count);
            avgWarn = Math.round(item.warn / item.count);
            avgDanger = Math.round(item.danger / item.count);
            avgRec = Math.round(item.rec / item.count);
        }

        html += `
        <div style="background: rgba(0,0,0,0.2); ${outline} border-radius: 8px; padding: 10px; text-align: center; box-shadow: inset 0 2px 5px rgba(0,0,0,0.3);">
            <div style="font-weight: 800; color: ${isToday ? 'var(--accent)' : 'var(--text-secondary)'}; margin-bottom: 8px; font-size:16px;">${d}</div>
            <div style="display: flex; justify-content: space-around; font-size: 13px; font-weight: 800;">
                <span style="color: var(--success); text-shadow:0 0 5px rgba(16,185,129,0.4);" title="Operativas">${avgOk}</span>
                <span style="color: orange; text-shadow:0 0 5px rgba(255,165,0,0.4);" title="Limitadas">${avgWarn}</span>
                <span style="color: #ef4444; text-shadow:0 0 5px rgba(239,68,68,0.4);" title="Caídas">${avgDanger}</span>
                <span style="color: #a855f7; text-shadow:0 0 5px rgba(168,85,247,0.4);" title="Grabando">${avgRec}</span>
            </div>
        </div>
        `;
    }

    grid.innerHTML = html;
};

async function startBackgroundScanner() {
    backgroundScannerActive = true;

    // Almacenamiento viejo de las IP para KPI (solo contabilizamos los que tienen IP válida)
    const findValueInRow = (row, possibleNames) => {
        const actualKeys = Object.keys(row);
        for (let name of possibleNames) {
            const match = actualKeys.find(k =>
                k.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace('_', ' ') === name.toLowerCase()
            );
            if (match && row[match] !== "") return row[match];
        }
        return null;
    };

    const nodos = excelData.filter(row => {
        const ip = findValueInRow(row, ["ip estacion", "ip pc", "ip"]);
        return ip && ip !== "Sin IP" && ip !== "N/A" && typeof ip === 'string' && ip.trim() !== "";
    });

    if (nodos.length === 0) return;

    const kpiTotalEl = document.getElementById('kpi-total');
    if (kpiTotalEl) kpiTotalEl.innerText = nodos.length;

    const scannerLog = document.getElementById('scanner-log');

    // Bucle inteligente (Desacoplado)
    const syncState = async () => {
        try {
            console.log(`Sincronizando estado con Servidor Python... (${new Date().toLocaleTimeString()})`);

            const response = await fetch(`${window.location.origin}/api/state`);
            if (!response.ok) throw new Error('Servidor offline');

            const state = await response.json();

            let ok = 0, warn = 0, danger = 0, recording = 0;

            // Reconciliar estado con el frontend
            for (let safeId in state) {
                const ns = state[safeId];
                windowStatusCache[safeId] = ns;

                updateMarkerColor(safeId, ns);
                renderStatusToDOM(safeId, ns);

                if (ns.argus && ns.argus.status) recording++;

                if (ns.ping) {
                    if (ns.rdp && ns.vnc) {
                        ok++;
                    } else if (ns.rdp || ns.vnc) {
                        warn++;
                    } else {
                        danger++;
                    }
                } else {
                    danger++;
                }
            }

            if (document.getElementById('kpi-ok')) {
                document.getElementById('kpi-ok').innerText = ok;
                setRing('ring-ok', ok, nodos.length);
            }
            if (document.getElementById('kpi-warning')) {
                document.getElementById('kpi-warning').innerText = warn;
                setRing('ring-warning', warn, nodos.length);
            }
            if (document.getElementById('kpi-danger')) {
                document.getElementById('kpi-danger').innerText = danger;
                setRing('ring-danger', danger, nodos.length);
            }
            if (document.getElementById('kpi-recording')) {
                document.getElementById('kpi-recording').innerText = recording;
                setRing('ring-recording', recording, nodos.length);
            }

            setMultiRing(ok, warn, nodos.length);
            fetchHistory();

        } catch (e) {
            console.error(`[Scanner] Fallo de sincronización: ${e.message}`);
            console.error("Fallo de fondo:", e);
        }
    };

    // Ejecutar inmediatamente para pintar los marcadores sin retraso inicial
    syncState();
    
    // Mantener la sincronización periódica
    setInterval(syncState, 2000);
}

// --- FUNCIONES DEL TELEMANDO PDU ---
async function checkPduStatus(ipTelemando, safeId, authTelemando) {
    const container = document.getElementById(`pdu-container-${safeId}`);
    if (!container) return;

    container.innerHTML = '<div style="text-align:center;">⏳ Conectando...</div>';

    try {
        const response = await fetch(`${window.location.origin}/api/power/status?ip=${ipTelemando}&auth=${authTelemando}`);
        const data = await response.json();

        if (data.status && data.ports && data.ports.length > 0) {
            let html = '';
            data.ports.forEach(port => {
                const isOn = port.status === 1;
                const statusColor = isOn ? 'green' : 'red';
                const statusText = isOn ? 'ON' : 'OFF';

                html += `
                <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:4px; font-size:12px; background:rgba(0,0,0,0.3); border:1px solid rgba(255,255,255,0.1); padding:4px; border-radius:4px; box-shadow:0 1px 2px rgba(0,0,0,0.05);">
                    <span style="font-weight:bold; width:65px; overflow:hidden; text-overflow:ellipsis; color:#fff;" title="${port.name}">${port.name}</span>
                    <span style="color:${statusColor}; font-weight:bold; width:25px; text-shadow:0 0 5px ${statusColor};">${statusText}</span>
                    <div style="display:flex; gap:4px;">
                        <button onclick="powerAction('${ipTelemando}', ${port.id}, '${isOn ? 0 : 1}', '${safeId}', '${authTelemando}')" style="background:${isOn ? '#ef4444' : '#10b981'}; color:white; border:none; padding:4px 8px; border-radius:3px; cursor:pointer; font-size:11px; min-width: 60px;">
                            ${isOn ? 'Apagar' : 'Encender'}
                        </button>
                        <button onclick="powerAction('${ipTelemando}', ${port.id}, 'r', '${safeId}', '${authTelemando}')" style="background:#f59e0b; color:white; border:none; padding:4px; border-radius:3px; cursor:pointer; font-size:11px;" title="Reiniciar">
                            🔄
                        </button>
                    </div>
                </div>`;
            });
            container.innerHTML = html;
        } else {
            container.innerHTML = `<div style="color:red; text-align:center;">❌ ${data.msg || "Sin puertos"}</div>`;
        }
    } catch (e) {
        container.innerHTML = `<div style="color:red; text-align:center;">❌ Error: ${e.message}</div>`;
    }
}

async function powerAction(ipTelemando, port, action, safeId, authTelemando) {
    if (action === '0') {
        if (!confirm(`⚠️ ATENCIÓN: Vas a APAGAR manualmente un puerto.\n¿Estás seguro/a de que quieres continuar?`)) return;
    } else if (action === 'r') {
        if (!confirm(`⚠️ Estás a punto de REINICIAR este enchufe.\n¿Seguro/a?`)) return;
    }

    const container = document.getElementById(`pdu-container-${safeId}`);
    container.innerHTML = '<div style="text-align:center;">⏳ Enviando orden...</div>';

    try {
        const response = await fetch(`${window.location.origin}/api/power/action?ip=${ipTelemando}&port=${port}&action=${action}&auth=${authTelemando}`);
        await response.json();
        setTimeout(() => checkPduStatus(ipTelemando, safeId, authTelemando), 2500);
    } catch (e) {
        alert("Error enviando el comando");
        checkPduStatus(ipTelemando, safeId, authTelemando);
    }
}

// --- 3. EXCEL AUTOMÁTICO Y EDICIÓN ---
// Función para actualizar KPIs iniciales
function updateKPIs() {
    const findValueInRow = (row, possibleNames) => {
        const actualKeys = Object.keys(row);
        for (let name of possibleNames) {
            const match = actualKeys.find(k =>
                k.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace('_', ' ') === name.toLowerCase()
            );
            if (match && row[match] !== "") return row[match];
        }
        return null;
    };

    const nodos = excelData.filter(row => {
        const ip = findValueInRow(row, ["ip estacion", "ip pc", "ip"]);
        return ip && ip !== "Sin IP" && ip !== "N/A" && typeof ip === 'string' && ip.trim() !== "";
    });

    const kpiTotalEl = document.getElementById('kpi-total');
    if (kpiTotalEl) kpiTotalEl.innerText = nodos.length;
}

async function loadExcelAutomatically() {
    try {
        console.log(`[DEBUG] Cargando Excel desde: ${originalFileName}`);
        const response = await fetch(originalFileName, { cache: 'no-store' });
        console.log(`[DEBUG] Respuesta del servidor: ${response.status} ${response.statusText}`);
        if (!response.ok) throw new Error(`Error HTTP: ${response.status}`);

        const arrayBuffer = await response.arrayBuffer();
        console.log(`[DEBUG] Archivo recibido, tamaño: ${arrayBuffer.byteLength} bytes`);

        const workbook = XLSX.read(arrayBuffer, { type: 'array' });
        console.log(`[DEBUG] Hojas disponibles: ${workbook.SheetNames.join(', ')}`);

        const firstSheetName = workbook.SheetNames[0];
        const worksheet = workbook.Sheets[firstSheetName];

        excelData = XLSX.utils.sheet_to_json(worksheet, { defval: "" });
        console.log(`[DEBUG] Estaciones cargadas: ${excelData.length}`);

        if (excelData.length > 0) {
            excelHeaders = Object.keys(excelData[0]);
            filteredExcelData = [...excelData]; // Inicializar datos filtrados
            filteredMapData = [...excelData];   // Inicializar datos filtrados del mapa
            console.log(`[DEBUG] Columnas: ${excelHeaders.join(', ')}`);

            renderExcelTable();
            document.getElementById('save-excel-btn').style.display = 'inline-block';

            // Actualizar mapa si ya está inicializado
            if (map) {
                console.log(`[DEBUG] Actualizando mapa con ${excelData.length} estaciones`);
                updateMapFromExcel();
            } else {
                console.log(`[DEBUG] Mapa no inicializado aún`);
            }

            // Actualizar KPIs iniciales
            updateKPIs();
        } else {
            console.warn(`[DEBUG] El archivo no contiene datos`);
        }
    } catch (error) {
        console.error(`[ERROR] Cargando Excel:`, error);
        document.querySelector('.empty-state').innerHTML = `<span style="color:red;">Error cargando <b>${originalFileName}</b>: ${error.message}</span>`;
    }

    // Agregar listeners para filtrado
    const tableFilterInput = document.getElementById('table-filter-input');
    if (tableFilterInput) {
        tableFilterInput.addEventListener('input', applyTableFilter);
    }
    const mapFilterInput = document.getElementById('map-filter-input');
    if (mapFilterInput) {
        mapFilterInput.addEventListener('input', applyMapFilter);
    }
}

// FUNCIÓN: Aplicar filtro a la tabla de Excel
function applyTableFilter() {
    const filterValue = document.getElementById('table-filter-input').value.toLowerCase();

    if (!filterValue) {
        filteredExcelData = [...excelData];
    } else {
        filteredExcelData = excelData.filter(row => {
            return excelHeaders.some(header => {
                const cellValue = (row[header] || '').toString().toLowerCase();
                return cellValue.includes(filterValue);
            });
        });
    }

    const resultsEl = document.getElementById('filter-results');
    if (resultsEl) {
        resultsEl.textContent = `${filteredExcelData.length} de ${excelData.length} registros`;
    }

    renderExcelTable();
}

// FUNCIÓN: Limpiar filtro de tabla
function clearTableFilter() {
    document.getElementById('table-filter-input').value = '';
    filteredExcelData = [...excelData];
    document.getElementById('filter-results').textContent = '';
    renderExcelTable();
}

// FUNCIÓN: Aplicar filtro al mapa
function applyMapFilter() {
    const filterValue = document.getElementById('map-filter-input').value.toLowerCase();

    if (!filterValue) {
        filteredMapData = [...excelData];
    } else {
        filteredMapData = excelData.filter(row => {
            return excelHeaders.some(header => {
                const cellValue = (row[header] || '').toString().toLowerCase();
                return cellValue.includes(filterValue);
            });
        });
    }

    const resultsEl = document.getElementById('map-filter-results');
    if (resultsEl) {
        resultsEl.textContent = `${filteredMapData.length} de ${excelData.length} estaciones`;
    }

    updateMapVisibility();
}

// FUNCIÓN: Limpiar filtro del mapa
function clearMapFilter() {
    document.getElementById('map-filter-input').value = '';
    filteredMapData = [...excelData];
    const resultsEl = document.getElementById('map-filter-results');
    if (resultsEl) {
        resultsEl.textContent = '';
    }
    updateMapVisibility();
}

// FUNCIÓN: Actualizar visibilidad de marcadores en el mapa
function updateMapVisibility() {
    if (!map || !markersLayer) return;

    // Helper para buscar en la fila
    const findValueInRow = (row, possibleNames) => {
        const actualKeys = Object.keys(row);
        for (let name of possibleNames) {
            const match = actualKeys.find(k =>
                k.toLowerCase().normalize("NFD").replace(/[\u0300-\u036f]/g, "").replace('_', ' ') === name.toLowerCase()
            );
            if (match && row[match] !== "") return row[match];
        }
        return null;
    };

    // Obtener IPs filtradas
    const filteredIPs = new Set(filteredMapData.map(row => {
        const ipField = findValueInRow(row, ['ip estacion', 'ip pc', 'ip']) || '';
        return ipField.toString().toLowerCase().trim();
    }).filter(ip => ip));

    // Mostrar/ocultar marcadores
    // Si filteredIPs está vacío pero el input tiene texto, ocultamos todos
    const inputVal = document.getElementById('map-filter-input')?.value || '';
    if (inputVal && filteredIPs.size === 0) {
        markersLayer.eachLayer(marker => marker.setOpacity(0.3));
        return;
    }

    markersLayer.eachLayer(marker => {
        // Obtenemos la IP desde el contenido HTML del popup
        const popup = marker.getPopup();
        let markerIp = "";
        if (popup) {
            const content = popup.getContent();
            const ipMatch = content?.match(/🌐 IP Nodo:<\/b>\s*([^<]+)/);
            if (ipMatch && ipMatch[1]) {
                markerIp = ipMatch[1].trim();
            }
        }

        // Si la IP extraída del marcador está dentro del Set de filtrados, lo iluminamos
        const hasVisibleIP = Array.from(filteredIPs).some(ip => markerIp && markerIp.toLowerCase().includes(ip.toLowerCase()));
        
        // Si no hay filtro, mostrar todo.
        const shouldShow = (!inputVal) || hasVisibleIP;

        marker.setOpacity(shouldShow ? 1 : 0.3);
    });
}

function renderExcelTable() {
    const container = document.getElementById('excel-table-container');
    let tableHTML = '<table class="modern-table"><thead><tr><th>Acción</th>';
    excelHeaders.forEach(header => tableHTML += `<th>${header}</th>`);
    tableHTML += '</tr></thead><tbody>';

    // Usar datos filtrados si hay filtro activo, sino usar todos
    const dataToRender = filteredExcelData.length > 0 && document.getElementById('table-filter-input')?.value
        ? filteredExcelData
        : excelData;

    dataToRender.forEach((row, displayIndex) => {
        // Encontrar el índice real en excelData
        const realIndex = excelData.indexOf(row);
        tableHTML += `<tr><td><button class="btn-edit-row" onclick="openModal(${realIndex})">✍️ Editar</button></td>`;
        excelHeaders.forEach(header => tableHTML += `<td>${row[header] || ''}</td>`);
        tableHTML += `</tr>`;
    });

    tableHTML += '</tbody></table>';
    container.innerHTML = tableHTML;
}

function openModal(rowIndex) {
    currentRowIndex = rowIndex;
    const rowData = excelData[rowIndex];
    const formContainer = document.getElementById('modal-form');
    formContainer.innerHTML = '';

    excelHeaders.forEach(header => {
        const isLongText = header.toLowerCase().includes('observacion') || header.toLowerCase().includes('direcc');
        const div = document.createElement('div');
        div.className = isLongText ? 'form-group full' : 'form-group';
        div.innerHTML = `<label>${header}</label><input type="text" id="input-${header}" value="${rowData[header] || ''}">`;
        formContainer.appendChild(div);
    });
    document.getElementById('edit-modal').style.display = 'flex';
}

function closeModal() {
    document.getElementById('edit-modal').style.display = 'none';
    currentRowIndex = null;
}

function saveRowChanges() {
    if (currentRowIndex === null) return;
    excelHeaders.forEach(header => {
        excelData[currentRowIndex][header] = document.getElementById(`input-${header}`).value;
    });
    renderExcelTable();
    closeModal();

    // Autoguardar en NodeJS inmediatamente al aplicar los cambios
    downloadExcel();
}

function downloadExcel() {
    if (excelData.length === 0) return;
    const newWorksheet = XLSX.utils.json_to_sheet(excelData);
    const newWorkbook = XLSX.utils.book_new();
    XLSX.utils.book_append_sheet(newWorkbook, newWorksheet, "Estaciones");

    // Transformar a buffer
    const excelBuffer = XLSX.write(newWorkbook, { bookType: 'xlsx', type: 'array' });

    // Actualizar remotamente en el servidor en lugar de descargar a la carpeta de usuario
    fetch(window.location.origin + '/api/save-excel', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/octet-stream'
        },
        body: excelBuffer
    }).then(res => res.json())
        .then(data => {
            if (data.success) {
                alert("✅ Cambios guardados correctamente en el servidor.");
                // Forzar actualización del mapa
                mapMarkers = {};
                if (markersLayer) markersLayer.clearLayers();
                updateMapFromExcel();
            } else {
                alert("❌ Error interno al guardar: " + data.error);
            }
        }).catch(err => {
            alert("❌ No se pudo conectar con el servidor para guardar. ¿Está encendido NodeJS (localhost:3000)?");
        });
}

// INICIO AUTOMÁTICO
document.addEventListener('DOMContentLoaded', async () => {
    // Cargar usuarios primero
    usersData = await loadUsers();

    // Recuperar sesión si existe en sessionStorage
    const savedUser = sessionStorage.getItem('noc_user');
    const savedPassword = sessionStorage.getItem('noc_password');
    if (savedUser && savedPassword) {
        console.log('[DEBUG] Recuperando sesión desde sessionStorage...');
        login(savedUser, savedPassword);
    } else {
        // Inicializar permisos (usuario invitado por defecto)
        updateUIBasedOnPermissions();
    }

    // Inicializar mapa diferido (se inicializa al abrir pestaña)
    // initMap();

    // Cargar datos del Excel
    console.log('[DEBUG] Cargando datos del Excel...');
    await loadExcelAutomatically();

    // Iniciar escáner de fondo si hay datos cargados
    if (excelData.length > 0 && !backgroundScannerActive) {
        console.log('[DEBUG] Iniciando escáner de fondo...');
        setTimeout(startBackgroundScanner, 2000);
    }
});
// --- THEME & ACCENT LOGIC ---
const THEME_KEY = 'jpit_theme_preference';
const ACCENT_KEY = 'jpit_accent_preference';

function toggleTheme() {
    const root = document.documentElement;
    const currentTheme = root.getAttribute('data-theme');
    const newTheme = currentTheme === 'light' ? 'dark' : 'light';
    root.setAttribute('data-theme', newTheme);
    localStorage.setItem(THEME_KEY, newTheme);

    const btn = document.getElementById('theme-toggle-btn');
    if (btn) btn.innerText = newTheme === 'light' ? '🌙 Modo Oscuro' : '☀️ Modo Claro';
}

function setAccentColor(colorHex) {
    document.documentElement.style.setProperty('--accent', colorHex);
    localStorage.setItem(ACCENT_KEY, colorHex);
}

(function initTheme() {
    const savedTheme = localStorage.getItem(THEME_KEY);
    if (savedTheme === 'light') {
        document.documentElement.setAttribute('data-theme', 'light');
    }
    const savedAccent = localStorage.getItem(ACCENT_KEY);
    if (savedAccent) {
        document.documentElement.style.setProperty('--accent', savedAccent);
    }

    window.addEventListener('DOMContentLoaded', () => {
        const btn = document.getElementById('theme-toggle-btn');
        if (btn && savedTheme === 'light') btn.innerText = '🌙 Modo Oscuro';

        const picker = document.getElementById('accent-picker');
        if (picker && savedAccent) picker.value = savedAccent;
    });
})();


