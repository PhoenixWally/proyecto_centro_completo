document.addEventListener('DOMContentLoaded', () => {
    const loginSection = document.getElementById('login-section');
    const dashboard    = document.getElementById('dashboard');
    const loginForm    = document.getElementById('loginForm');
    const errorMsg     = document.getElementById('error-msg');
    const submitBtn    = document.getElementById('submit-btn');
    const btnText      = document.getElementById('btn-text');
    const btnLoader    = document.getElementById('btn-loader');
    const appGrid      = document.getElementById('app-grid');
    const logoutBtn    = document.getElementById('logout-btn');

    // ── LOGIN ─────────────────────────────────────────────────────────────────
    loginForm.addEventListener('submit', async (e) => {
        e.preventDefault();
        setLoading(true);
        errorMsg.style.display = 'none';

        const username = document.getElementById('username').value.trim();
        const password = document.getElementById('password').value;

        try {
            const response = await fetch('/api/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ username, password })
            });

            if (response.ok) {
                const data = await response.json();

                // Guardar nivel_poder en sessionStorage para que admin.js lo lea
                if (data.nivel_poder !== undefined) {
                    sessionStorage.setItem('nivel_poder', data.nivel_poder);
                }

                // Ocultar sección de login completa
                loginSection.style.display = 'none';

                // Mostrar dashboard con flex (el CSS lo necesita)
                dashboard.style.display = 'flex';

                renderDashboard(data);
            } else {
                showError('Credenciales inválidas. Inténtalo de nuevo.');
            }
        } catch (error) {
            console.error('Login error:', error);
            showError('Error de conexión. Inténtalo más tarde.');
        } finally {
            setLoading(false);
        }
    });

    // ── RENDER DASHBOARD ──────────────────────────────────────────────────────
    function renderDashboard(data) {
        // Botón de administración condicional (nivel >= 60)
        const adminBtn = document.getElementById('admin-panel-btn');
        if (data.nivel_poder >= 60) {
            adminBtn.style.display = 'inline-block';
        }

        // Limpiar grid
        appGrid.innerHTML = '';

        const apps = data.aplicaciones;

        if (apps && Array.isArray(apps) && apps.length > 0) {
            apps.forEach((app, index) => {
                const card = document.createElement('a');
                card.href        = app.ruta_base || app.ruta || '#';
                card.className   = 'app-card fade-in';
                card.style.animationDelay = `${index * 0.1}s`;

                const iconDiv = document.createElement('div');
                iconDiv.className   = 'app-icon';
                iconDiv.textContent = getIcon(app.nombre);

                const title = document.createElement('h3');
                title.textContent = app.nombre;

                const desc = document.createElement('p');
                desc.textContent = 'Acceder a la aplicación';

                card.appendChild(iconDiv);
                card.appendChild(title);
                card.appendChild(desc);
                appGrid.appendChild(card);
            });
        } else {
            // Fallback de contingencia
            console.warn('No se recibieron aplicaciones desde el servidor. Cargando fallback.');
            const fallbackApps = [
                { nombre: 'Sentinel Radar', ruta_base: '/sentinel/', icon: '📡' },
                { nombre: 'VNC Remote',     ruta_base: '/vnc/',      icon: '🖥️' }
            ];

            fallbackApps.forEach((app, index) => {
                const card = document.createElement('a');
                card.href       = app.ruta_base;
                card.className  = 'app-card fade-in';
                card.style.animationDelay = `${index * 0.1}s`;

                const iconDiv = document.createElement('div');
                iconDiv.className   = 'app-icon';
                iconDiv.textContent = app.icon;

                const title = document.createElement('h3');
                title.textContent = app.nombre;

                const desc = document.createElement('p');
                desc.textContent = 'Acceso de contingencia';

                card.appendChild(iconDiv);
                card.appendChild(title);
                card.appendChild(desc);
                appGrid.appendChild(card);
            });
        }
    }

    // ── LOGOUT ────────────────────────────────────────────────────────────────
    logoutBtn.addEventListener('click', async () => {
        try {
            // Intentar invalidar la cookie JWT en el servidor
            await fetch('/api/logout', { method: 'POST' });
        } catch (_) { /* si falla, continuamos */ }
        sessionStorage.removeItem('nivel_poder');
        window.location.reload();
    });

    // ── HELPERS ───────────────────────────────────────────────────────────────
    function setLoading(isLoading) {
        if (isLoading) {
            submitBtn.disabled      = true;
            btnText.style.display   = 'none';
            btnLoader.style.display = 'block';
        } else {
            submitBtn.disabled      = false;
            btnText.style.display   = 'block';
            btnLoader.style.display = 'none';
        }
    }

    function showError(message) {
        errorMsg.textContent  = message;
        errorMsg.style.display = 'block';
    }

    function getIcon(name) {
        const n = name.toLowerCase();
        if (n.includes('sentinel')) return '📡';
        if (n.includes('analítico') || n.includes('analitico')) return '📊';
        if (n.includes('vnc'))      return '🖥️';
        if (n.includes('admin'))    return '⚙️';
        return '🚀';
    }
});
