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
    // Si el usuario actual tiene acceso a la aplicación de Procesos
    let currentUserHasProcesos = false;

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

            // Obtener nivel propio de forma ultra confiable desde la sesión real
            try {
                const sessRes = await fetch('/api/session');
                if (sessRes.ok) {
                    const sessData = await sessRes.json();
                    if (sessData.nivel_poder !== undefined) {
                        currentUserLevel = sessData.nivel_poder;
                    }
                    if (sessData.aplicaciones) {
                        currentUserHasProcesos = sessData.aplicaciones.some(app => app.ruta === '/procesos/');
                    }
                }
            } catch (e) {
                console.warn("No se pudo consultar el nivel real desde la sesión:", e);
                // Fallback a sessionStorage
                const storedLevel = parseInt(sessionStorage.getItem('nivel_poder'), 10);
                if (!isNaN(storedLevel) && storedLevel > 0) {
                    currentUserLevel = storedLevel;
                }
            }

            if (currentUserLevel < 80) {
                const estSection = document.getElementById('estaciones-section');
                if (estSection) {
                    estSection.style.display = 'none';
                }
            }

            // Cargar la lista de roles disponibles en el sistema
            await fetchRoles();
            // Cargar provincias y poblar todos los selects
            await fetchProvincias();

            adminApp.style.display = 'block';
            populateLevelOptions();
            renderUsers(users);
            if (currentUserLevel >= 80) {
                fetchStations();
            }
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
            if (user.puesto) {
                const puestoDiv = document.createElement('div');
                puestoDiv.style.fontSize = '11px';
                puestoDiv.style.color = 'var(--text-secondary)';
                puestoDiv.style.marginTop = '2px';
                puestoDiv.textContent = user.puesto;
                tdUser.appendChild(puestoDiv);
            }
            if (user.email) {
                const emailDiv = document.createElement('div');
                emailDiv.style.fontSize = '11px';
                emailDiv.style.color = 'var(--text-secondary)';
                emailDiv.style.marginTop = '2px';
                emailDiv.textContent = user.email;
                tdUser.appendChild(emailDiv);
            }
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

        const helpText = select.parentNode.querySelector('p');
        if (helpText) {
            helpText.textContent = currentUserLevel >= 80 ? 'Niveles autorizados para tu rango (incluido el tuyo).' : 'Solo se muestran niveles inferiores al tuyo.';
        }

        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = 'Selecciona un nivel';
        select.appendChild(placeholder);

        // Solo roles con nivel_poder < currentUserLevel (o <= si es superadmin nivel >= 80)
        const eligibleRoles = allRoles
            .filter(r => currentUserLevel >= 80 ? r.nivel_poder <= currentUserLevel : r.nivel_poder < currentUserLevel)
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

        // Campo contraseña visible en edición pero no requerido
        const passGroup = document.getElementById('password-group');
        passGroup.style.display = 'block';
        const passLabel = passGroup.querySelector('label');
        if (passLabel) passLabel.textContent = 'Contraseña (vacío para mantener)';
        const passInput = document.getElementById('password');
        
        passInput.type = 'text'; // Cambiar temporalmente a texto para engañar al gestor de contraseñas y evitar autocompletado
        passInput.required = false;
        passInput.placeholder = 'Dejar vacío para no cambiar';
        passInput.value = ''; // Mantener vacío si no se cambia

        // Cuando el usuario haga foco en el input, revertimos el tipo a password de forma segura
        const onPassFocus = () => {
            passInput.type = 'password';
        };
        passInput.removeEventListener('focus', onPassFocus);
        passInput.addEventListener('focus', onPassFocus, { once: true });

        // Rellenar campos
        document.getElementById('user-id').value = user.id;
        document.getElementById('username').value = user.usuario;
        document.getElementById('username').readOnly = true;
        document.getElementById('puesto').value = user.puesto || '';
        document.getElementById('email').value = user.email || '';

        // Rellenar sub-permisos individuales
        document.getElementById('perm_extraccion').checked = !!user.permiso_extraccion;
        document.getElementById('perm_alarmas').checked = !!user.permiso_alarmas;
        document.getElementById('perm_visor').checked = !!user.permiso_visor;
        document.getElementById('app_procesos').checked = !!user.permiso_procesos;

        // Ocultar la opción de Procesos si el admin de nivel 60 o inferior no la tiene asignada
        const procesosContainer = document.getElementById('app-procesos-container');
        if (currentUserLevel < 80 && !currentUserHasProcesos) {
            procesosContainer.style.display = 'none';
            document.getElementById('app_procesos').checked = false;
        } else {
            procesosContainer.style.display = 'flex';
        }

        // Ocultar opciones de Extracción y Alarmas a administradores de nivel < 80 (Provinciales)
        if (currentUserLevel < 80) {
            document.getElementById('perm_extraccion').checked = false;
            document.getElementById('perm_alarmas').checked = false;
            document.getElementById('perm-extraccion-container').style.display = 'none';
            document.getElementById('perm-alarmas-container').style.display = 'none';
        } else {
            document.getElementById('perm-extraccion-container').style.display = 'flex';
            document.getElementById('perm-alarmas-container').style.display = 'flex';
        }

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

        const passGroup = document.getElementById('password-group');
        passGroup.style.display = 'block';
        const passLabel = passGroup.querySelector('label');
        if (passLabel) passLabel.textContent = 'Contraseña Inicial';
        const passInput = document.getElementById('password');
        passInput.type = 'password'; // Asegurar que sea password al crear
        passInput.required = true;
        passInput.placeholder = '';
        document.getElementById('username').readOnly = false;
        document.getElementById('puesto').value = '';
        document.getElementById('email').value = '';

        // Resetear checkboxes de permisos individuales
        document.getElementById('perm_extraccion').checked = false;
        document.getElementById('perm_alarmas').checked = false;
        document.getElementById('perm_visor').checked = false;
        document.getElementById('app_procesos').checked = false;

        const procesosContainerReset = document.getElementById('app-procesos-container');
        if (currentUserLevel < 80 && !currentUserHasProcesos) {
            procesosContainerReset.style.display = 'none';
        } else {
            procesosContainerReset.style.display = 'flex';
        }

        // Ajustar visibilidad según jerarquía del admin
        if (currentUserLevel < 80) {
            document.getElementById('perm-extraccion-container').style.display = 'none';
            document.getElementById('perm-alarmas-container').style.display = 'none';
        } else {
            document.getElementById('perm-extraccion-container').style.display = 'flex';
            document.getElementById('perm-alarmas-container').style.display = 'flex';
        }

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

        // Opciones Centro Analítico
        document.getElementById('st-ruta-bin').value = st.ruta_bin || '';
        document.getElementById('st-ruta-res').value = st.ruta_res || '';
        document.getElementById('st-usr-red').value = st.usr_red || '';
        document.getElementById('st-pwd-red').value = st.pwd_red || '';
        document.getElementById('analitico-fields').style.display = (st.ruta_bin || st.ruta_res || st.usr_red || st.pwd_red) ? 'block' : 'none';

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

        document.getElementById('st-ruta-bin').value = '';
        document.getElementById('st-ruta-res').value = '';
        document.getElementById('st-usr-red').value = '';
        document.getElementById('st-pwd-red').value = '';
        document.getElementById('analitico-fields').style.display = 'none';

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

        // Cerrar modal al pulsar fuera del card (usando mousedown para evitar cierres al seleccionar texto)
        userModal.addEventListener('mousedown', (e) => {
            if (e.target === userModal) {
                userModal.classList.remove('active');
                resetUserForm();
            }
        });
        stationModal.addEventListener('mousedown', (e) => {
            if (e.target === stationModal) {
                stationModal.classList.remove('active');
                resetStationForm();
            }
        });

        backBtn.onclick = () => window.location.href = '/sso/';

        // Cambiar por defecto los checks del Centro Analítico si se selecciona un nivel <= 60
        const nivelSelect = document.getElementById('nivel_poder');
        if (nivelSelect) {
            nivelSelect.addEventListener('change', () => {
                const selectedOpt = nivelSelect.options[nivelSelect.selectedIndex];
                const nivelElegido = parseInt(selectedOpt?.dataset?.nivel ?? '-1', 10);
                if (!isNaN(nivelElegido) && nivelElegido > 0 && nivelElegido <= 60) {
                    document.getElementById('perm_extraccion').checked = true;
                    // Los administradores provinciales (nivel 60) no pueden activar Extracción ni Alarmas para sí mismos
                    // pero si un nivel >= 80 les gestiona el perfil, se marcan por defecto los tres.
                    // Si el administrador actual es nivel 60, solo puede habilitar visor de archivos
                    if (currentUserLevel < 80) {
                        document.getElementById('perm_extraccion').checked = false;
                        document.getElementById('perm_alarmas').checked = false;
                    } else {
                        document.getElementById('perm_alarmas').checked = true;
                    }
                    document.getElementById('perm_visor').checked = true;
                }
            });
        }

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

            const isSuperAdmin = currentUserLevel >= 80;
            const isInvalidLevel = isSuperAdmin ? (nivelElegido > currentUserLevel) : (nivelElegido >= currentUserLevel);

            if (!rolId || isNaN(nivelElegido) || isInvalidLevel) {
                alert(isSuperAdmin 
                    ? `Error: Selecciona un nivel válido (menor o igual al tuyo: ${currentUserLevel}).` 
                    : `Error: Selecciona un nivel válido (inferior al tuyo: ${currentUserLevel}).`);
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
                    id:                 userId,
                    username:           document.getElementById('username').value,
                    rol_id:             parseInt(selectEl.value, 10),
                    provincia_id:       provId,
                    puesto:             document.getElementById('puesto').value.trim() || null,
                    email:              document.getElementById('email').value.trim() || null,
                    permiso_extraccion: document.getElementById('perm_extraccion').checked,
                    permiso_alarmas:    document.getElementById('perm_alarmas').checked,
                    permiso_visor:      document.getElementById('perm_visor').checked,
                    permiso_procesos:   document.getElementById('app_procesos').checked
                };
                const passVal = document.getElementById('password').value;
                if (passVal !== '') {
                    payload.password = passVal;
                }
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
                    username:           document.getElementById('username').value.trim(),
                    password:           document.getElementById('password').value,
                    rol_id:             parseInt(selectEl.value, 10),
                    provincia_id:       provId,
                    puesto:             document.getElementById('puesto').value.trim() || null,
                    email:              document.getElementById('email').value.trim() || null,
                    permiso_extraccion: document.getElementById('perm_extraccion').checked,
                    permiso_alarmas:    document.getElementById('perm_alarmas').checked,
                    permiso_visor:      document.getElementById('perm_visor').checked,
                    permiso_procesos:   document.getElementById('app_procesos').checked
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
                provincia: stationForm.provincia && stationForm.provincia.value !== '' ? stationForm.provincia.value.trim() : null,
                ruta_bin: document.getElementById('st-ruta-bin').value.trim() || null,
                ruta_res: document.getElementById('st-ruta-res').value.trim() || null,
                usr_red: document.getElementById('st-usr-red').value.trim() || null,
                pwd_red: document.getElementById('st-pwd-red').value.trim() || null
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
