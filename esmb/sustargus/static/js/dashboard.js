// ═══════════════════════════════════════════════════════
// dashboard.js  –  Panel Admin
// ═══════════════════════════════════════════════════════

// ─── Modal helpers ───────────────────────────────────────
function openStationModal() {
    document.getElementById('modalTitle').textContent = 'Nueva Estación';
    document.getElementById('editStationId').value = '';
    document.getElementById('stationForm').reset();
    document.getElementById('stationModal').classList.add('show');
}

function openEditStationModal(id) {
    fetch(`/api/stations/${id}`)
        .then(r => r.json())
        .then(st => {
            document.getElementById('modalTitle').textContent = 'Editar Estación';
            document.getElementById('editStationId').value = id;
            document.getElementById('stName').value     = st.name;
            document.getElementById('stIpEsmb').value   = st.ip_esmb;
            document.getElementById('stIpStation').value= st.ip_station;
            document.getElementById('stOutputDir').value = st.output_dir;
            document.getElementById('stUser').value     = st.username || '';
            document.getElementById('stPass').value     = '';  // no mostrar contraseña
            document.getElementById('stObs').value      = st.observations || '';
            document.getElementById('stationModal').classList.add('show');
        });
}

function closeStationModal() {
    document.getElementById('stationModal').classList.remove('show');
}

// ─── Guardar (añadir o editar) estación ──────────────────
document.getElementById('stationForm').addEventListener('submit', async e => {
    e.preventDefault();
    const id = document.getElementById('editStationId').value;
    const data = {
        name:         document.getElementById('stName').value.trim(),
        ip_esmb:      document.getElementById('stIpEsmb').value.trim(),
        ip_station:   document.getElementById('stIpStation').value.trim(),
        output_dir:   document.getElementById('stOutputDir').value.trim(),
        username:     document.getElementById('stUser').value.trim(),
        password:     document.getElementById('stPass').value,
        observations: document.getElementById('stObs').value.trim()
    };

    const method = id ? 'PUT' : 'POST';
    const url    = id ? `/api/stations/${id}` : '/api/stations';

    const res = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });

    if (res.ok) {
        closeStationModal();
        window.location.reload();
    } else {
        alert('Error al guardar la estación. Comprueba los datos.');
    }
});

async function deleteStation(id) {
    if (!confirm('¿Eliminar esta estación? Se borrarán también sus grabaciones programadas.')) return;
    const res = await fetch(`/api/stations/${id}`, { method: 'DELETE' });
    if (res.ok) {
        document.getElementById(`st-${id}`)?.remove();
    } else {
        alert('Error al eliminar la estación.');
    }
}

// ─── Grabaciones ──────────────────────────────────────────
let currentRecordingId = null;

async function loadRecordings(filterStationId = null) {
    const res  = await fetch('/api/recordings');
    let list = await res.json();
    const container = document.getElementById('recordingsTable');

    if (filterStationId) {
        list = list.filter(r => r.station_id == filterStationId);
    }

    if (!list.length) {
        container.innerHTML = '<p class="text-muted">No hay grabaciones programadas.</p>';
        return;
    }

    const rows = list.map(r => {
        const statusClass = {
            pending:  'badge-warning',
            running:  'badge-success',
            done:     'badge-muted',
            error:    'badge-danger'
        }[r.status] || 'badge-warning';

        return `<tr>
            <td>${r.station_name}</td>
            <td>Ant. ${r.antenna}</td>
            <td>${r.freq_start} – ${r.freq_end} MHz</td>
            <td>${r.date_start} al ${r.date_end}</td>
            <td>${r.time_start} - ${r.time_end}</td>
            <td><span class="badge ${statusClass}">${r.status}</span></td>
            <td>
                ${(USER_ROLE !== 'viewer') ? `
                    <button class="btn btn-small btn-outline" onclick="editRecording(${r.id})">✏️</button>
                    <button class="btn btn-small btn-danger" onclick="deleteRecording(${r.id})">🗑</button>
                ` : `<span class="badge guest">Solo lectura</span>`}
            </td>
        </tr>`;
    }).join('');

    container.innerHTML = `
        <div class="table-wrap">
        <table class="data-table">
            <thead>
                <tr>
                    <th>Estación</th><th>Antena</th><th>Frecuencias</th>
                    <th>Inicio</th><th>Fin</th><th>Estado</th><th></th>
                </tr>
            </thead>
            <tbody>${rows}</tbody>
        </table>
        </div>`;
}

async function editRecording(id) {
    const res = await fetch(`/api/recordings/${id}`);
    const r = await res.json();
    currentRecordingId = id;
    
    document.getElementById('stationSelect').value = r.station_id;
    document.getElementById('antennaSelect').value = r.antenna;
    document.getElementById('freqStart').value    = r.freq_start;
    document.getElementById('freqEnd').value      = r.freq_end;
    document.getElementById('dateStart').value = r.date_start;
    document.getElementById('dateEnd').value   = r.date_end;
    document.getElementById('timeStart').value = r.time_start;
    document.getElementById('timeEnd').value   = r.time_end;
    document.getElementById('recordOutputDir').value = r.output_dir;
    
    document.getElementById('btnSubmitRecord').textContent = '💾 ACTUALIZAR GRABACIÓN';
}

async function deleteRecording(id) {
    if (!confirm('¿Eliminar esta grabación programada?')) return;
    const res = await fetch(`/api/recordings/${id}`, { method: 'DELETE' });
    if (res.ok) {
        loadRecordings();
    } else {
        alert('Error al eliminar.');
    }
}

document.getElementById('recordForm').addEventListener('submit', async e => {
    e.preventDefault();
        const data = {
            station_id: document.getElementById('stationSelect').value,
            antenna: document.getElementById('antennaSelect').value,
            freq_start: document.getElementById('freqStart').value,
            freq_end: document.getElementById('freqEnd').value,
            date_start: document.getElementById('dateStart').value,
            date_end: document.getElementById('dateEnd').value,
            time_start: document.getElementById('timeStart').value,
            time_end: document.getElementById('timeEnd').value,
            output_dir: document.getElementById('recordOutputDir').value
        };

    if (!data.station_id) { alert('Selecciona una estación.'); return; }
    if (data.freq_start >= data.freq_end) { alert('La frecuencia de fin debe ser mayor que la de inicio.'); return; }

    const method = currentRecordingId ? 'PUT' : 'POST';
    const url = currentRecordingId ? `/api/recordings/${currentRecordingId}` : '/api/recordings';

    const res = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });

    const result = await res.json();
    if (res.ok && result.success) {
        document.getElementById('recordForm').reset();
        currentRecordingId = null;
        document.getElementById('btnSubmitRecord').textContent = '📅 PROGRAMAR GRABACIÓN';
        loadRecordings();
    } else {
        alert('Error: ' + (result.error || 'No se pudo programar. Comprueba colisiones horarias.'));
    }
});

// Evento para filtrar tabla por estación seleccionada
document.getElementById('stationSelect').addEventListener('change', e => {
    loadRecordings(e.target.value);
});

// ─── Gestión de Usuarios (Admin Only) ──────────────────────
async function loadUsers() {
    const res = await fetch('/api/users');
    const users = await res.json();
    const container = document.getElementById('usersListTable');
    
    if (!users.length) {
        container.innerHTML = '<p class="text-muted">No hay usuarios.</p>';
        return;
    }

    const rolesMap = { admin: 'Administrador', manager: 'Gestor', viewer: 'Visor' };
    
    container.innerHTML = `
        <table class="data-table">
            <thead><tr><th>Usuario</th><th>Rol</th><th></th></tr></thead>
            <tbody>
                ${users.map(u => `
                    <tr>
                        <td>${u.username}</td>
                        <td><span class="badge badge-outline">${rolesMap[u.role] || u.role}</span></td>
                        <td style="text-align:right">
                            <button class="btn btn-small btn-outline" onclick='editUser(${JSON.stringify(u)})'>✏️</button>
                            <button class="btn btn-small btn-danger" onclick="deleteUser(${u.id})">🗑</button>
                        </td>
                    </tr>
                `).join('')}
            </tbody>
        </table>`;
}

function openUserModal() {
    document.getElementById('userModal').classList.add('show');
    loadUsers();
}

function closeUserModal() {
    document.getElementById('userModal').classList.remove('show');
    document.getElementById('userForm').reset();
    document.getElementById('editUserId').value = '';
    document.getElementById('btnSubmitUser').textContent = '➕ CREAR USUARIO';
    document.getElementById('usrName').disabled = false;
}

function editUser(user) {
    document.getElementById('editUserId').value = user.id;
    document.getElementById('usrName').value = user.username;
    document.getElementById('usrName').disabled = true; // No permitir cambiar el nombre de usuario
    document.getElementById('usrRole').value = user.role;
    document.getElementById('usrPass').value = '';
    document.getElementById('btnSubmitUser').textContent = '💾 GUARDAR CAMBIOS';
}

document.getElementById('userForm')?.addEventListener('submit', async e => {
    e.preventDefault();
    const id = document.getElementById('editUserId').value;
    const data = {
        username: document.getElementById('usrName').value.trim(),
        password: document.getElementById('usrPass').value,
        role:     document.getElementById('usrRole').value
    };

    const method = id ? 'PUT' : 'POST';
    const url    = id ? `/api/users/${id}` : '/api/users';

    const res = await fetch(url, {
        method,
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data)
    });

    const result = await res.json();
    if (res.ok) {
        document.getElementById('userForm').reset();
        document.getElementById('editUserId').value = '';
        document.getElementById('btnSubmitUser').textContent = '➕ CREAR USUARIO';
        document.getElementById('usrName').disabled = false;
        loadUsers();
    } else {
        alert('Error: ' + (result.error || 'No se pudo procesar.'));
    }
});

async function deleteUser(id) {
    if (!confirm('¿Eliminar este usuario?')) return;
    const res = await fetch(`/api/users/${id}`, { method: 'DELETE' });
    if (res.ok) {
        loadUsers();
    } else {
        const err = await res.json();
        alert('Error: ' + (err.error || 'No se pudo eliminar.'));
    }
}

// Inicializar tabla al cargar
loadRecordings();
// ─── Inicialización de Flatpickr (Fechas y Horas modernas) ──
document.addEventListener('DOMContentLoaded', () => {
    // Configuración común para fechas
    const dateConfig = {
        locale: "es",
        dateFormat: "Y-m-d",
        altInput: true,
        altFormat: "d/m/Y",
        allowInput: true,
        theme: "dark"
    };

    flatpickr("#dateStart", dateConfig);
    flatpickr("#dateEnd", dateConfig);

    // Configuración común para horas
    const timeConfig = {
        locale: "es",
        enableTime: true,
        noCalendar: true,
        dateFormat: "H:i",
        time_24hr: true,
        theme: "dark"
    };

    flatpickr("#timeStart", timeConfig);
    flatpickr("#timeEnd", timeConfig);
});
