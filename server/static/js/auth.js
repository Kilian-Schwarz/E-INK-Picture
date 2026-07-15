// Auth UI bootstrap (E5.1) — loaded as the FIRST script on the designer page.
// 1. Wraps window.fetch once (idempotent): a same-origin 401 response
//    (except /api/auth/*, which handles its own errors) redirects to /login.
//    The login page redirects back to /designer on success (no returnTo —
//    the server login flow deliberately has no ?next= redirect surface).
// 2. GET /api/auth/status on load: no password set -> dismissible warning
//    banner with an inline setup dialog; authenticated -> logout button.
(function () {
    'use strict';

    // --- global 401 interceptor (installed synchronously) ----------------
    if (!window.fetch.__einkAuthWrapped) {
        var baseFetch = window.fetch.bind(window);
        var wrappedFetch = function (input, init) {
            return baseFetch(input, init).then(function (response) {
                if (response.status === 401 && shouldRedirectToLogin(input)) {
                    window.location = '/login';
                }
                return response;
            });
        };
        wrappedFetch.__einkAuthWrapped = true;
        window.fetch = wrappedFetch;
    }

    function shouldRedirectToLogin(input) {
        var raw = (typeof Request !== 'undefined' && input instanceof Request)
            ? input.url : String(input);
        var url;
        try {
            url = new URL(raw, window.location.href);
        } catch (err) {
            return false;
        }
        // Own API only; /api/auth/* is exempt (login/setup handle their own
        // 401s — prevents redirect loops).
        return url.origin === window.location.origin &&
            url.pathname.indexOf('/api/auth/') !== 0;
    }

    // Server-side setup validation only rejects an empty password
    // (handlers/auth.go) — the client mirrors exactly that, no stricter rule.
    var MIN_PASSWORD_LENGTH = 1;
    var BANNER_DISMISSED_KEY = 'eink-auth-banner-dismissed';

    function onReady(fn) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', fn);
        } else {
            fn();
        }
    }

    function notify(message, type) {
        if (typeof showNotification === 'function') {
            showNotification(message, type);
        }
    }

    onReady(function () {
        fetch('/api/auth/status')
            .then(function (res) { return res.json(); })
            .then(function (status) {
                if (status.password_set === false) {
                    showBanner();
                }
                if (status.authenticated === true) {
                    showLogoutButton();
                }
            })
            .catch(function () { /* banner/logout are conveniences only */ });
    });

    // --- "no admin password" banner ---------------------------------------
    function showBanner() {
        var dismissed = false;
        try {
            dismissed = sessionStorage.getItem(BANNER_DISMISSED_KEY) === '1';
        } catch (err) { /* storage unavailable: always show */ }
        if (dismissed) return;

        var container = document.getElementById('auth-banner-container');
        if (!container || document.getElementById('auth-banner')) return;

        var banner = document.createElement('div');
        banner.className = 'auth-banner';
        banner.id = 'auth-banner';
        banner.setAttribute('role', 'alert');
        banner.innerHTML =
            '<span class="auth-banner-text">No admin password set &mdash; anyone ' +
            'on your network can control this device.</span>' +
            '<button type="button" class="btn-primary" id="auth-banner-setup-btn">Set one now</button>' +
            '<button type="button" class="auth-banner-close" id="auth-banner-close" ' +
            'title="Dismiss" aria-label="Dismiss">&times;</button>';
        container.appendChild(banner);

        document.getElementById('auth-banner-setup-btn')
            .addEventListener('click', openSetupDialog);
        document.getElementById('auth-banner-close')
            .addEventListener('click', function () {
                try {
                    sessionStorage.setItem(BANNER_DISMISSED_KEY, '1');
                } catch (err) { /* private mode: dismiss for this view only */ }
                banner.remove();
            });
    }

    function removeBanner() {
        var banner = document.getElementById('auth-banner');
        if (banner) banner.remove();
    }

    // --- setup dialog (POST /api/auth/setup, then login for a session) ----
    function openSetupDialog() {
        var modal = document.getElementById('auth-setup-modal') || buildSetupDialog();
        var passwordInput = document.getElementById('auth-setup-password');
        var repeatInput = document.getElementById('auth-setup-password-repeat');
        passwordInput.value = '';
        repeatInput.value = '';
        showSetupError('');
        modal.style.display = 'flex';
        passwordInput.focus();
    }

    function closeSetupDialog() {
        var modal = document.getElementById('auth-setup-modal');
        if (modal) modal.style.display = 'none';
    }

    function showSetupError(message) {
        var errorEl = document.getElementById('auth-setup-error');
        if (!errorEl) return;
        errorEl.textContent = message;
        errorEl.style.display = message ? 'block' : 'none';
    }

    function buildSetupDialog() {
        var modal = document.createElement('div');
        modal.id = 'auth-setup-modal';
        modal.className = 'modal';
        modal.innerHTML =
            '<div class="modal-overlay"></div>' +
            '<div class="modal-content auth-setup-content">' +
                '<div class="modal-header">' +
                    '<h2>Set admin password</h2>' +
                    '<button type="button" class="modal-close" id="auth-setup-close">&times;</button>' +
                '</div>' +
                '<div class="modal-body">' +
                    '<form id="auth-setup-form">' +
                        '<div class="property-group">' +
                            '<label for="auth-setup-password">Password</label>' +
                            '<input type="password" id="auth-setup-password" class="prop-input" ' +
                            'autocomplete="new-password">' +
                        '</div>' +
                        '<div class="property-group">' +
                            '<label for="auth-setup-password-repeat">Repeat password</label>' +
                            '<input type="password" id="auth-setup-password-repeat" class="prop-input" ' +
                            'autocomplete="new-password">' +
                        '</div>' +
                        '<p class="auth-setup-error" id="auth-setup-error"></p>' +
                        '<div class="modal-actions">' +
                            '<button type="submit" class="btn-primary" id="auth-setup-submit">Set password</button>' +
                            '<button type="button" class="btn-secondary" id="auth-setup-cancel">Cancel</button>' +
                        '</div>' +
                    '</form>' +
                '</div>' +
            '</div>';
        document.body.appendChild(modal);

        document.getElementById('auth-setup-close')
            .addEventListener('click', closeSetupDialog);
        document.getElementById('auth-setup-cancel')
            .addEventListener('click', closeSetupDialog);
        modal.querySelector('.modal-overlay')
            .addEventListener('click', closeSetupDialog);
        document.getElementById('auth-setup-form')
            .addEventListener('submit', submitSetup);
        return modal;
    }

    function submitSetup(event) {
        event.preventDefault();
        showSetupError('');

        var password = document.getElementById('auth-setup-password').value;
        var repeat = document.getElementById('auth-setup-password-repeat').value;
        if (password.length < MIN_PASSWORD_LENGTH) {
            showSetupError('Password must not be empty.');
            return;
        }
        if (password !== repeat) {
            showSetupError('Passwords do not match.');
            return;
        }

        var submitBtn = document.getElementById('auth-setup-submit');
        submitBtn.disabled = true;
        fetch('/api/auth/setup', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password: password })
        }).then(function (res) {
            if (res.status === 200) {
                // Auth is active from this moment — sign in right away so the
                // page keeps working instead of 401-redirecting to /login.
                return fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password: password })
                }).then(function (loginRes) {
                    if (loginRes.status === 200) {
                        closeSetupDialog();
                        removeBanner();
                        showLogoutButton();
                        notify('Admin password set — you are signed in.', 'success');
                        return;
                    }
                    // Password is set but the session is missing (e.g. rate
                    // limit hit) — the login page is the recovery path.
                    window.location = '/login';
                });
            }
            if (res.status === 403) {
                showSetupError('A password is already set — reload and sign in.');
                return;
            }
            if (res.status === 429) {
                var retryAfter = res.headers.get('Retry-After');
                showSetupError('Too many attempts. Try again in ' +
                    (retryAfter ? retryAfter + ' s.' : 'a minute.'));
                return;
            }
            showSetupError('Setting the password failed.');
        }).catch(function () {
            showSetupError('Server not reachable.');
        }).then(function () {
            submitBtn.disabled = false;
        });
    }

    // --- logout ------------------------------------------------------------
    function showLogoutButton() {
        var btn = document.getElementById('logout-btn');
        if (!btn) return;
        btn.style.display = '';
        if (btn.__einkLogoutBound) return;
        btn.__einkLogoutBound = true;
        btn.addEventListener('click', function () {
            fetch('/api/auth/logout', { method: 'POST' })
                .catch(function () { /* redirect regardless */ })
                .then(function () { window.location = '/login'; });
        });
    }
})();
