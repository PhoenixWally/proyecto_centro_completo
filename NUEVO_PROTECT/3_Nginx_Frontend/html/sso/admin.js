document.addEventListener('DOMContentLoaded', async () => {
    const adminApp = document.getElementById('admin-app');
    const usersBody = document.getElementById('users-body');
    const stationsBody = document.getElementById('stations-body');
    const userModal = document.getElementById('user-modal');
    const stationModal = document.getElementById('station-modal');
    const userForm = document.getElementById('user-form');
    const stationForm = document.getElementById('station-form');
    const backBtn = document.getElementById('back-btn');

    // Nivel del administrador actual (se rellena tras la auth)
    let currentUserLevel = 0;
    // Lista de roles disponibles en el sistema
    let allRoles = [];
    // Lista de provincias disponibles
    let allProvincias = [];

    // ── 1. CARGA SEGURA Y AUTORIZACIÓN ────────────────────────────────────────
    async function checkAuthAndLoad() {
        try {
            const response = await fetch('/api/admin/usuarios');

            if (response.status === 403 || response.status === 401) {
                window.location.replace('/sso/');
                return;
            }
            if (!response.ok) {
                window.location.replace('/sso/');
                return;
            }

            const users = await response.json();

            // Obtener nivel propio desde sessionStorage (guardado en el login)
            const storedLevel = parseInt(sessionStorage.getItem('nivel_poder'), 10);
            if (!isNaN(storedLevel) && storedLevel > 0) {
                currentUserLevel = storedLevel;
            }

            // Cargar la lista de roles disponibles en el sistema
            await fetchRoles();
            // Cargar provincias y poblar todos los selects
            await fetchProvincias();

            adminApp.style.display = 'block';
            populateLevelOptions();
            renderUsers(users);
            fetchStations();
            setupEventListeners();
        } catch (error) {
            console.error('Error de carga:', error);
            window.location.replace('/sso/');
        }
    }

    // ── 2. CARGA DE ROLES ─────────────────────────────────────────────────────
    async function fetchRoles() {
        try {
            const res = await fetch('/api/admin/roles');
            if (res.ok) {
                allRoles = await res.json(); // [{id, nombre, nivel_poder}]
            }
        } catch (e) {
            console.warn('No se pudo cargar /api/admin/roles. Usando roles genéricos.', e);
        }

        // Si la API de roles no existe aún, generamos un fallback con los
        // niveles estándar del sistema para no bloquear la UI.
        if (!allRoles || allRoles.length === 0) {
            // IDs provisionales — cuando /api/admin/roles exista, se usarán los reales del sistema
            allRoles = [
                { nombre: 'Visor', nivel_poder: 20 },
                { nombre: 'Usuario', nivel_poder: 40 },
                { nombre: 'Admin Provincial', nivel_poder: 60 },
                { nombre: 'Admin Global', nivel_poder: 80 },
                { nombre: 'Super Admin', nivel_poder: 100 }
            ];
        }
    }

    // ── 2b. CARGA DE PROVINCIAS ────────────────────────────────────────────────
    async function fetchProvincias() {
        try {
            const res = await fetch('/api/admin/provincias');
            if (res.ok) {
                allProvincias = await res.json(); // [{id, nombre}]
            }
        } catch (e) {
            console.warn('No se pudo cargar /api/admin/provincias:', e);
        }
        populateProvinciaSelects();
    }

    /**
     * Rellena todos los <select> de provincia con las provincias cargadas.
     * @param {number|null} selectedId  ID de la provincia a preseleccionar
     * @param {string}      selectId    ID del elemento <select> a poblar
     */
    function populateProvinciaSelect(selectId, selectedId = null) {
        const sel = document.getElementById(selectId);
        if (!sel) return;
        // Limpiar manteniedo solo el placeholder
        while (sel.firstChild) sel.removeChild(sel.firstChild);
        const ph = document.createElement('option');
        ph.value = '';
        ph.textContent = '-- Sin asignar --';
        sel.appendChild(ph);

        allProvincias.forEach(p => {
            const opt = document.createElement('option');
            opt.value = p.id;
            opt.textContent = p.nombre;
            // eslint-disable-next-line eqeqeq
            if (selectedId !== null && p.id == selectedId) opt.selected = true;
            sel.appendChild(opt);
        });
    }

    function populateProvinciaSelects(selectedId = null) {
        populateProvinciaSelect('provincia', selectedId);
        populateProvinciaSelect('st-provincia', null);
    }

    // ── 3. RENDERIZADO SEGURO (PREVENCIÓN XSS CRÍTICA) ────────────────────────
    function renderUsers(users) {
        usersBody.innerHTML = '';

        if (!users || users.length === 0) {
            const tr = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 4;
            td.textContent = 'No hay usuarios gestionables.';
            td.style.textAlign = 'center';
            td.style.color = 'var(--text-secondary)';
            tr.appendChild(td);
            usersBody.appendChild(tr);
            return;
        }

        users.forEach(user => {
            const tr = document.createElement('tr');

            // Columna: Usuario
            const tdUser = document.createElement('td');
            tdUser.textContent = user.usuario;
            tr.appendChild(tdUser);

            // Columna: Nivel con badge
            const tdLevel = document.createElement('td');
            const badge = document.createElement('span');
            badge.className = 'level-badge';
            badge.textContent = `${user.nivel_poder} — ${getRoleName(user.nivel_poder)}`;
            tdLevel.appendChild(badge);
            tr.appendChild(tdLevel);

            // Columna: Provincia
            const tdProv = document.createElement('td');
            tdProv.textContent = user.provincia || 'Global';
            tr.appendChild(tdProv);

            // Columna: Acciones
            const tdActions = document.createElement('td');

            const editBtn = document.createElement('button');
            editBtn.textContent = 'Editar';
            editBtn.className = 'action-btn btn-edit';
            editBtn.setAttribute('aria-label', `Editar usuario ${user.usuario}`);
            editBtn.onclick = () => openEditUserModal(user);
            tdActions.appendChild(editBtn);

            const delBtn = document.createElement('button');
            delBtn.textContent = 'Eliminar';
            delBtn.className = 'action-btn btn-delete';
            delBtn.setAttribute('aria-label', `Eliminar usuario ${user.usuario}`);
            delBtn.onclick = () => deleteUser(user.id);
            tdActions.appendChild(delBtn);

            tr.appendChild(tdActions);
            usersBody.appendChild(tr);
        });
    }

    function renderStations(stations) {
        stationsBody.innerHTML = '';

        if (!stations || stations.length === 0) {
            const tr = document.createElement('tr');
            const td = document.createElement('td');
            td.colSpan = 4;
            td.textContent = 'No hay estaciones registradas.';
            td.style.textAlign = 'center';
            td.style.color = 'var(--text-secondary)';
            tr.appendChild(td);
            stationsBody.appendChild(tr);
            return;
        }

        stations.forEach(st => {
            const tr = document.createElement('tr');

            const tdName = document.createElement('td');
            tdName.textContent = st.nombre;
            tr.appendChild(tdName);

            const tdIp = document.createElement('td');
            tdIp.textContent = st.ip || st.ip_red || 'N/A';
            tr.appendChild(tdIp);

            const tdProv = document.createElement('td');
            tdProv.textContent = st.provincia || 'N/A';
            tr.appendChild(tdProv);

            const tdActions = document.createElement('td');
            
            const editBtn = document.createElement('button');
            editBtn.textContent = 'Editar';
            editBtn.className = 'action-btn btn-edit';
            editBtn.onclick = () => openEditStationModal(st);
            tdActions.appendChild(editBtn);

            const delBtn = document.createElement('button');
            delBtn.textContent = 'Eliminar';
            delBtn.className = 'action-btn btn-delete';
            delBtn.onclick = () => deleteStation(st.id);
            tdActions.appendChild(delBtn);
            tr.appendChild(tdActions);

            stationsBody.appendChild(tr);
        });
    }

    // ── 4. LÓGICA DE ROLES / OPCIONES ─────────────────────────────────────────
    /**
     * Rellena el <select id="nivel_poder"> con los roles cuyo nivel_poder
     * es ESTRICTAMENTE INFERIOR al del administrador actual.
     *
     * Cada <option> tiene:
     *   value       = role.id   ← lo que el backend espera como «rol_id»
     *   data-nivel  = role.nivel_poder  ← para la validación de jerarquía
     *
     * @param {string|number|null} selectedRolId  ID del rol a preseleccionar
     */
    function populateLevelOptions(selectedRolId = null) {
        const select = document.getElementById('nivel_poder');
        // Limpiar
        while (select.firstChild) select.removeChild(select.firstChild);

        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = 'Selecciona un nivel';
        select.appendChild(placeholder);

        // Solo roles con nivel_poder < currentUserLevel
        const eligibleRoles = allRoles
            .filter(r => r.nivel_poder < currentUserLevel)
            .sort((a, b) => a.nivel_poder - b.nivel_poder);

        eligibleRoles.forEach(role => {
            const opt = document.createElement('option');
            opt.value = role.id;            // ← FK esperada por el backend
            opt.dataset.nivel = role.nivel_poder;  // ← para validar jerarquía
            opt.textContent = `${role.nombre} (nivel ${role.nivel_poder})`;
            // eslint-disable-next-line eqeqeq
            if (selectedRolId !== null && role.id == selectedRolId) {
                opt.selected = true;
            }
            select.appendChild(opt);
        });
    }

    /** Devuelve el nombre del rol más cercano a un nivel dado */
    function getRoleName(nivel) {
        if (!allRoles || allRoles.length === 0) return '';
        const exact = allRoles.find(r => r.nivel_poder === nivel);
        if (exact) return exact.nombre;
        // Si no hay coincidencia exacta, devolver el rol más cercano por debajo
        const lower = allRoles
            .filter(r => r.nivel_poder <= nivel)
            .sort((a, b) => b.nivel_poder - a.nivel_poder);
        return lower.length > 0 ? lower[0].nombre : '';
    }

    // ── 5. MODAL EDICIÓN DE USUARIO ───────────────────────────────────────────
    function openEditUserModal(user) {
        const modalTitle = userModal.querySelector('.modal-title');
        const submitBtn = userForm.querySelector('.btn-save');

        modalTitle.textContent = 'Editar Usuario';
        submitBtn.textContent = 'Guardar Cambios';

        // Campo contraseña oculto en edición
        document.getElementById('password-group').style.display = 'none';
        document.getElementById('password').required = false;

        // Rellenar campos
        document.getElementById('user-id').value = user.id;
        document.getElementById('username').value = user.usuario;
        document.getElementById('username').readOnly = true;

        // Poblar select de provincia y preseleccionar por provincia_id
        populateProvinciaSelect('provincia', user.provincia_id ?? null);

        // Poblar el select y preseleccionar por rol_id del usuario
        populateLevelOptions(user.rol_id ?? null);

        // Marcar modo edición
        userForm.dataset.editMode = 'true';
        userForm.dataset.userId = user.id;

        userModal.classList.add('active');
    }

    function resetUserForm() {
        const modalTitle = userModal.querySelector('.modal-title');
        const submitBtn = userForm.querySelector('.btn-save');

        modalTitle.textContent = 'Nuevo Usuario';
        submitBtn.textContent = 'Crear Usuario';

        document.getElementById('password-group').style.display = 'block';
        document.getElementById('password').required = true;
        document.getElementById('username').readOnly = false;

        userForm.dataset.editMode = 'false';
        userForm.dataset.userId = '';
        userForm.reset();

        // Resetear selects a sin preselección
        populateLevelOptions();
        populateProvinciaSelect('provincia', null);
    }

    function openEditStationModal(st) {
        const modalTitle = stationModal.querySelector('.modal-title');
        const submitBtn = stationForm.querySelector('.btn-save');

        if (modalTitle) modalTitle.textContent = 'Editar Estación';
        if (submitBtn) submitBtn.textContent = 'Guardar Cambios';

        stationForm.nombre.value = st.nombre;
        stationForm.ip.value = st.ip || st.ip_red || st.ruta_unc || '';
        populateProvinciaSelect('st-provincia', st.provincia_id ?? null);

        stationForm.dataset.editMode = 'true';
        stationForm.dataset.stationId = st.id;

        stationModal.classList.add('active');
    }

    function resetStationForm() {
        const modalTitle = stationModal.querySelector('.modal-title');
        const submitBtn = stationForm.querySelector('.btn-save');

        if (modalTitle) modalTitle.textContent = 'Nueva Estación';
        if (submitBtn) submitBtn.textContent = 'Registrar Estación';

        stationForm.dataset.editMode = 'false';
        stationForm.dataset.stationId = '';
        stationForm.reset();

        populateProvinciaSelect('st-provincia', null);
    }

    // ── 6. FETCH ESTACIONES ───────────────────────────────────────────────────
    async function fetchStations() {
        try {
            const res = await fetch('/api/admin/estaciones');
            if (res.ok) {
                const stations = await res.json();
                renderStations(stations);
            }
        } catch (e) { console.error('Error al cargar estaciones:', e); }
    }

    // ── 7. EVENT LISTENERS ────────────────────────────────────────────────────
    function setupEventListeners() {
        // Abrir modales
        document.getElementById('new-user-btn').onclick = () => {
            resetUserForm();
            userModal.classList.add('active');
        };
        document.getElementById('new-station-btn').onclick = () => {
            resetStationForm();
            stationModal.classList.add('active');
        };

        // Cerrar modales (botones con clase .modal-close)
        document.querySelectorAll('.modal-close').forEach(btn => {
            btn.onclick = () => {
                userModal.classList.remove('active');
                stationModal.classList.remove('active');
                resetUserForm();
                resetStationForm();
            };
        });

        // Cerrar modal al pulsar fuera del card
        userModal.addEventListener('click', (e) => {
            if (e.target === userModal) {
                userModal.classList.remove('active');
                resetUserForm();
            }
        });
        stationModal.addEventListener('click', (e) => {
            if (e.target === stationModal) {
                stationModal.classList.remove('active');
                resetStationForm();
            }
        });

        backBtn.onclick = () => window.location.href = '/sso/';

        // ── FORMULARIO USUARIO ────────────────────────────────────────────────
        userForm.onsubmit = async (e) => {
            e.preventDefault();

            const isEditMode = userForm.dataset.editMode === 'true';
            const userId = userForm.dataset.userId;

            // Leer el <option> seleccionado para obtener rol_id Y nivel
            const selectEl = document.getElementById('nivel_poder');
            const selectedOpt = selectEl.options[selectEl.selectedIndex];
            const rolId = parseInt(selectEl.value, 10);                           // int — lo que el backend espera
            const nivelElegido = parseInt(selectedOpt?.dataset?.nivel ?? '-1', 10); // para validar jerarquía

            if (!rolId || isNaN(nivelElegido) || nivelElegido >= currentUserLevel) {
                alert(`Error: Selecciona un nivel válido (inferior al tuyo: ${currentUserLevel}).`);
                return;
            }

            if (isEditMode) {
                // ── PUT /api/admin/usuarios  (ID en el body) ──────────────────
                const provSelEdit = document.getElementById('provincia');
                let provId = provSelEdit.value;
                if (provId === '') {
                    provId = null;
                }
                const payload = {
                    id:           userId,
                    username:     document.getElementById('username').value,
                    rol_id:       parseInt(selectEl.value, 10),
                    provincia_id: provId
                };
                console.log("PAYLOAD SEGURO:", JSON.stringify(payload));

                try {
                    const res = await fetch('/api/admin/usuarios/editar', {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });

                    if (res.ok) {
                        userModal.classList.remove('active');
                        resetUserForm();
                        await refreshUsers();
                    } else {
                        const err = await res.json().catch(() => ({}));
                        alert('Error al guardar: ' + (err.error || res.statusText));
                    }
                } catch (error) {
                    console.error(error);
                    alert('Error de red al actualizar el usuario.');
                }

            } else {
                // ── POST /api/admin/usuarios  (crear nuevo) ───────────────────
                const provSelPost = document.getElementById('provincia');
                let provId = provSelPost.value;
                if (provId === '') {
                    provId = null;
                }
                const payload = {
                    username:     document.getElementById('username').value.trim(),
                    password:     document.getElementById('password').value,
                    rol_id:       parseInt(selectEl.value, 10),
                    provincia_id: provId
                };
                console.log("PAYLOAD SEGURO:", JSON.stringify(payload));

                try {
                    const res = await fetch('/api/admin/usuarios', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });

                    if (res.ok) {
                        userModal.classList.remove('active');
                        resetUserForm();
                        await refreshUsers();
                    } else {
                        const err = await res.json().catch(() => ({}));
                        alert('Error al crear: ' + (err.error || res.statusText));
                    }
                } catch (error) {
                    console.error(error);
                    alert('Error de red al crear el usuario.');
                }
            }
        };

        // ── FORMULARIO ESTACIÓN ───────────────────────────────────────────────
        stationForm.onsubmit = async (e) => {
            e.preventDefault();

            const isEditMode = stationForm.dataset.editMode === 'true';
            const stationId = stationForm.dataset.stationId;

            const payload = {
                nombre: stationForm.nombre.value.trim(),
                ip: stationForm.ip.value.trim(),
                provincia: stationForm.provincia && stationForm.provincia.value !== '' ? stationForm.provincia.value.trim() : null
            };

            if (isEditMode) {
                payload.id = stationId;
                try {
                    const res = await fetch('/api/admin/estaciones/editar', {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });

                    if (res.ok) {
                        stationModal.classList.remove('active');
                        resetStationForm();
                        fetchStations();
                    } else {
                        const err = await res.json().catch(() => ({}));
                        alert('Error al guardar estación: ' + (err.error || res.statusText));
                    }
                } catch (error) {
                    console.error(error);
                }
            } else {
                try {
                    const res = await fetch('/api/admin/estaciones', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify(payload)
                    });

                    if (res.ok) {
                        stationModal.classList.remove('active');
                        resetStationForm();
                        fetchStations();
                    } else {
                        const err = await res.json().catch(() => ({}));
                        alert('Error al registrar estación: ' + (err.error || res.statusText));
                    }
                } catch (error) {
                    console.error(error);
                }
            }
        };
    }

    // ── 8. HELPERS ────────────────────────────────────────────────────────────
    async function refreshUsers() {
        try {
            const res = await fetch('/api/admin/usuarios');
            if (res.ok) renderUsers(await res.json());
        } catch (e) { console.error('Error al refrescar usuarios:', e); }
    }

    async function deleteUser(id) {
        if (!confirm('¿Eliminar este usuario? Esta acción no se puede deshacer.')) return;
        try {
            const res = await fetch('/api/admin/usuarios/eliminar', {
                method: 'DELETE',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id })
            });
            if (res.ok) {
                await refreshUsers();
            } else {
                const err = await res.json().catch(() => ({}));
                alert('Error al eliminar: ' + (err.error || res.statusText));
            }
        } catch (e) {
            console.error(e);
            alert('Error de red al eliminar el usuario.');
        }
    }

    async function deleteStation(id) {
        if (!confirm('¿Eliminar esta estación?')) return;
        try {
            await fetch(`/api/admin/estaciones/${id}`, { method: 'DELETE' });
            fetchStations();
        } catch (e) { console.error(e); }
    }

    // ── ARRANQUE ──────────────────────────────────────────────────────────────
    checkAuthAndLoad();
});
