// Setup wizard (spec E2.3) — guided first-run flow for fresh installations.
//
// Appears when GET /api/setup/status says wizard:true (factory-fresh data
// dir), when ?setup=1 forces it, or when a mid-wizard reload finds resume
// state (sessionStorage 'eink-setup-progress' — never the password). Five
// steps: display type, location (optional), refresh interval (single
// POST /update_settings), admin password (setup + immediate auto-login, so
// the global 401 interceptor in auth.js never fires), starter design from
// the template gallery (create + activate + trigger — the wizard click IS
// the deliberate activation). Finish and "Skip setup" both persist
// {setup_completed:true} and reload, restoring stock behavior.
//
// Pure DOM overlay: no fabric/canvas dependency, click/input/submit
// handlers only, existing design tokens, mobile-first.
var SetupWizard = {
    STORAGE_KEY: 'eink-setup-progress',
    TOTAL_STEPS: 5,
    STEP_LABELS: ['Display', 'Location', 'Refresh', 'Password', 'Design'],
    INTERVALS: [
        { seconds: 900, label: '15 min', note: 'Frequent updates' },
        { seconds: 1800, label: '30 min', note: 'Balanced' },
        { seconds: 3600, label: '60 min', note: 'Recommended' }
    ],

    status: null,          // GET /api/setup/status response
    state: null,           // wizard progress (mirrored to sessionStorage)
    _root: null,
    _isOpen: false,
    _profiles: [],         // GET /display_profiles
    _settings: null,       // GET /settings (preselection + preview baseline)
    _previewBaseline: null, // display_type the gallery previews rendered against
    _searchBusy: false,
    _lastSearchAt: 0,      // Nominatim politeness: >= 1 s between searches
    _choosing: false,
    _finishing: false,
    _created: null,        // {id, name, templateId, design} from step 5

    // --- appearance / resume decision (Richtung 3 + 6) --------------------
    async boot() {
        var status;
        try {
            var resp = await fetch('/api/setup/status');
            if (!resp.ok) return;
            status = await resp.json();
        } catch (e) {
            return; // no status, no wizard — stock behavior
        }
        this.status = status;

        var progress = this._loadProgress();
        var forced = new URLSearchParams(window.location.search).get('setup') === '1';

        var shouldOpen = false;
        if (status.wizard === true || forced) {
            shouldOpen = true;
        } else if (progress && status.setup_completed === false) {
            // Resume rule: a mid-wizard reload keeps working even after
            // step 3 created the freshness trace or step 4 set the password.
            // Another device/incognito has no resume entry — no wizard.
            if (status.password_set === false) {
                shouldOpen = true;
            } else {
                try {
                    var authResp = await fetch('/api/auth/status');
                    var auth = await authResp.json();
                    shouldOpen = auth.authenticated === true;
                } catch (e) {
                    shouldOpen = false;
                }
            }
        }
        if (!shouldOpen) return;

        this.state = progress || this._freshState();
        await this.open();
    },

    _freshState() {
        return {
            step: 1,
            displayType: null,
            location: null,        // {name, short, lat, lon} — strings
            locationSkipped: false,
            refreshInterval: 3600,
            renderQuality: 'high',
            passwordDone: false,
            completed: null        // {id, name, templateId} | {blank:true}
        };
    },

    _loadProgress() {
        try {
            var raw = sessionStorage.getItem(this.STORAGE_KEY);
            if (!raw) return null;
            var state = JSON.parse(raw);
            if (!state || typeof state.step !== 'number') return null;
            return state;
        } catch (e) {
            return null;
        }
    },

    // Persists selections + step — NEVER the password (AC5).
    _saveProgress() {
        try {
            sessionStorage.setItem(this.STORAGE_KEY, JSON.stringify(this.state));
        } catch (e) { /* private mode: resume simply unavailable */ }
    },

    _clearProgress() {
        try {
            sessionStorage.removeItem(this.STORAGE_KEY);
        } catch (e) { /* ignore */ }
    },

    // --- shell -------------------------------------------------------------
    async open() {
        var root = document.getElementById('setup-wizard-root');
        if (!root) return;
        this._root = root;
        this._isOpen = true;
        document.body.classList.add('setup-wizard-open');

        root.innerHTML =
            '<div class="setup-wizard" id="setup-wizard">' +
            '<div class="setup-wizard-inner">' +
            '<header class="setup-wizard-header">' +
            '<span class="setup-wizard-brand">E-Ink Display Setup</span>' +
            '<button type="button" class="setup-wizard-skip" id="setup-wizard-skip-btn">Skip setup</button>' +
            '</header>' +
            '<div class="setup-wizard-progress" id="setup-wizard-progress"></div>' +
            '<main class="setup-wizard-content" id="setup-wizard-content"></main>' +
            '</div>' +
            '</div>';

        var self = this;
        document.getElementById('setup-wizard-skip-btn')
            .addEventListener('click', function () { self._skipSetup(); });

        try {
            var responses = await Promise.all([
                fetch('/display_profiles'),
                fetch('/settings')
            ]);
            this._profiles = responses[0].ok ? await responses[0].json() : [];
            this._settings = responses[1].ok ? await responses[1].json() : null;
        } catch (e) {
            this._profiles = [];
            this._settings = null;
        }
        if (!this.state.displayType && this._settings) {
            this.state.displayType = this._settings.display_type;
        }
        // Any previews rendered before the wizard saves settings used the
        // server's current display_type — the invalidation baseline.
        this._previewBaseline = this._settings ? this._settings.display_type : null;

        this.showStep(this.state.step || 1);
    },

    showStep(step) {
        this.state.step = step;
        this._saveProgress();
        this._renderProgress();

        var content = document.getElementById('setup-wizard-content');
        if (!content) return;
        switch (step) {
            case 1: this._renderStep1(content); break;
            case 2: this._renderStep2(content); break;
            case 3: this._renderStep3(content); break;
            case 4: this._renderStep4(content); break;
            case 5: this._renderStep5(content); break;
            default: this._renderSuccess(content); break;
        }
        var overlay = document.getElementById('setup-wizard');
        if (overlay) overlay.scrollTop = 0;
    },

    _renderProgress() {
        var el = document.getElementById('setup-wizard-progress');
        if (!el) return;
        var step = this.state.step;
        var skipBtn = document.getElementById('setup-wizard-skip-btn');
        if (step > this.TOTAL_STEPS) {
            el.innerHTML = '';
            el.style.display = 'none';
            if (skipBtn) skipBtn.style.display = 'none';
            return;
        }
        el.style.display = '';
        var dots = '';
        for (var i = 1; i <= this.TOTAL_STEPS; i++) {
            var cls = 'setup-step-dot' +
                (i < step ? ' done' : '') +
                (i === step ? ' current' : '');
            dots += '<span class="' + cls + '">' + (i < step ? '&#10003;' : i) + '</span>';
            if (i < this.TOTAL_STEPS) {
                dots += '<span class="setup-step-line' + (i < step ? ' done' : '') + '"></span>';
            }
        }
        el.innerHTML =
            '<div class="setup-step-dots">' + dots + '</div>' +
            '<div class="setup-step-label">Step ' + step + ' of ' + this.TOTAL_STEPS +
            ' &mdash; ' + this.STEP_LABELS[step - 1] + '</div>';
    },

    // --- step 1: welcome + display type (Richtung 4) -----------------------
    _renderStep1(content) {
        var self = this;
        var cardsHtml = '';
        this._profiles.forEach(function (p) {
            var chips = (p.colors || []).map(function (c) {
                return '<span class="setup-color-chip" style="background:' +
                    self.escapeHtml(c) + '"></span>';
            }).join('');
            cardsHtml +=
                '<button type="button" class="setup-display-card" data-type="' +
                self.escapeHtml(p.type) + '">' +
                '<span class="setup-display-illustration">' + self._panelSvg(p) + '</span>' +
                '<span class="setup-display-name">' + self.escapeHtml(p.name) + '</span>' +
                '<span class="setup-color-chips">' + chips + '</span>' +
                '<span class="setup-display-driver">Driver: ' + self.escapeHtml(p.driver) + '</span>' +
                '</button>';
        });
        if (!cardsHtml) {
            cardsHtml = '<p class="setup-error-text">Could not load the display profiles &mdash; ' +
                'please reload the page.</p>';
        }

        content.innerHTML =
            '<h2>Welcome!</h2>' +
            '<p class="setup-intro">Let&rsquo;s get your e-ink display ready &mdash; this takes ' +
            'about two minutes. First, pick the panel connected to this device.</p>' +
            '<div class="setup-display-cards" id="setup-display-cards">' + cardsHtml + '</div>' +
            '<div class="setup-server-time" id="setup-server-time"></div>' +
            '<div class="setup-wizard-actions">' +
            '<button type="button" class="btn-primary setup-continue" id="setup-step1-continue">Continue</button>' +
            '</div>';

        var timeEl = document.getElementById('setup-server-time');
        var serverTime = (this.status && this.status.server_time) || '';
        var hhmm = serverTime.length >= 16 ? serverTime.substring(11, 16) : '?';
        var tz = (this.status && this.status.server_timezone) ||
            (serverTime.length > 19 ? 'UTC offset ' + serverTime.substring(19) : 'system default');
        timeEl.textContent = 'Server time: ' + hhmm + ' (' + tz + ') — if this looks wrong, ' +
            'fix the timezone on the device (TZ env).';

        var cards = content.querySelectorAll('.setup-display-card');
        function applySelection() {
            cards.forEach(function (card) {
                card.classList.toggle('selected', card.dataset.type === self.state.displayType);
            });
        }
        cards.forEach(function (card) {
            card.addEventListener('click', function () {
                self.state.displayType = card.dataset.type;
                self._saveProgress();
                applySelection();
            });
        });
        applySelection();

        document.getElementById('setup-step1-continue')
            .addEventListener('click', function () {
                if (!self.state.displayType) return;
                self.showStep(2);
            });
    },

    // Schematic panel illustration (Richtung 4): 5:3 frame with color
    // stripes from the profile palette; B/W panels get a grayscale scheme.
    // Inline SVG only — no photos, no binary assets.
    _panelSvg(profile) {
        var colors = (profile.colors && profile.colors.length > 2)
            ? profile.colors
            : ['#000000', '#3d3d3d', '#7a7a7a', '#b5b5b5', '#e6e6e6', '#ffffff'];
        var inset = 10;
        var width = (100 - 2 * inset) / colors.length;
        var stripes = '';
        for (var i = 0; i < colors.length; i++) {
            stripes += '<rect x="' + (inset + i * width).toFixed(2) + '" y="' + inset +
                '" width="' + width.toFixed(2) + '" height="' + (60 - 2 * inset) +
                '" fill="' + this.escapeHtml(colors[i]) + '"/>';
        }
        return '<svg viewBox="0 0 100 60" role="img" aria-hidden="true" ' +
            'xmlns="http://www.w3.org/2000/svg">' +
            '<rect x="1.5" y="1.5" width="97" height="57" rx="4" fill="#f6f3ea" ' +
            'stroke="currentColor" stroke-width="2"/>' +
            stripes +
            '<rect x="' + inset + '" y="' + inset + '" width="' + (100 - 2 * inset) +
            '" height="' + (60 - 2 * inset) + '" fill="none" stroke="#00000033" stroke-width="0.5"/>' +
            '</svg>';
    },

    // --- step 2: location (Richtung 5, optional) ---------------------------
    _renderStep2(content) {
        var self = this;
        content.innerHTML =
            '<h2>Where is your display?</h2>' +
            '<p class="setup-intro">Used for the weather widgets on your display. ' +
            'You can skip this step.</p>' +
            '<form id="setup-location-form" class="setup-search-row">' +
            '<input type="text" id="setup-location-input" class="prop-input" ' +
            'placeholder="City or town&hellip;" autocomplete="off" inputmode="search">' +
            '<button type="submit" class="btn-primary" id="setup-location-search-btn">Search</button>' +
            '</form>' +
            '<div class="setup-location-results" id="setup-location-results"></div>' +
            '<div class="setup-location-chip-row" id="setup-location-chip-row"></div>' +
            '<p class="setup-hint">If you skip this step, weather widgets keep their default ' +
            'location (Berlin) &mdash; you can change it later in the designer.</p>' +
            '<div class="setup-wizard-actions">' +
            '<button type="button" class="btn-secondary setup-back" id="setup-step2-back">Back</button>' +
            '<button type="button" class="btn-secondary" id="setup-step2-skip">Skip this step</button>' +
            '<button type="button" class="btn-primary setup-continue" id="setup-step2-continue" disabled>Continue</button>' +
            '</div>';

        var continueBtn = document.getElementById('setup-step2-continue');

        function renderChip() {
            var row = document.getElementById('setup-location-chip-row');
            row.innerHTML = '';
            if (!self.state.location) {
                continueBtn.disabled = true;
                return;
            }
            var chip = document.createElement('span');
            chip.className = 'setup-location-chip';
            chip.id = 'setup-location-chip';
            var label = document.createElement('span');
            label.textContent = self.state.location.short +
                ' (' + self.state.location.lat + ', ' + self.state.location.lon + ')';
            var clear = document.createElement('button');
            clear.type = 'button';
            clear.className = 'setup-chip-clear';
            clear.setAttribute('aria-label', 'Remove location');
            clear.textContent = '×';
            clear.addEventListener('click', function () {
                self.state.location = null;
                self._saveProgress();
                renderChip();
            });
            chip.appendChild(label);
            chip.appendChild(clear);
            row.appendChild(chip);
            continueBtn.disabled = false;
        }
        renderChip();

        document.getElementById('setup-location-form')
            .addEventListener('submit', function (event) {
                event.preventDefault();
                var q = document.getElementById('setup-location-input').value.trim();
                if (q) self._searchLocation(q, renderChip);
            });
        document.getElementById('setup-step2-back')
            .addEventListener('click', function () { self.showStep(1); });
        document.getElementById('setup-step2-skip')
            .addEventListener('click', function () {
                self.state.location = null;
                self.state.locationSkipped = true;
                self.showStep(3);
            });
        continueBtn.addEventListener('click', function () {
            if (!self.state.location) return;
            self.state.locationSkipped = false;
            self.showStep(3);
        });
    },

    // One GET /location_search per explicit search (click/Enter), at most
    // one per second (Nominatim policy) and never two in flight.
    async _searchLocation(query, onSelect) {
        if (this._searchBusy) return;
        var now = Date.now();
        if (now - this._lastSearchAt < 1000) return;
        this._lastSearchAt = now;
        this._searchBusy = true;

        var resultsEl = document.getElementById('setup-location-results');
        var searchBtn = document.getElementById('setup-location-search-btn');
        if (searchBtn) searchBtn.disabled = true;
        if (resultsEl) resultsEl.textContent = 'Searching…';

        var self = this;
        try {
            var resp = await fetch('/location_search?q=' + encodeURIComponent(query));
            if (!resp.ok) throw new Error('location search failed: ' + resp.status);
            var results = await resp.json();
            if (!resultsEl) return;
            resultsEl.innerHTML = '';
            if (!results || results.length === 0) {
                resultsEl.textContent = 'No results found — try a different search term, ' +
                    'or skip this step.';
                return;
            }
            results.forEach(function (r) {
                var item = document.createElement('button');
                item.type = 'button';
                item.className = 'setup-location-result';
                item.textContent = r.display_name || '';
                item.addEventListener('click', function () {
                    self.state.location = {
                        name: r.display_name || '',
                        short: (r.display_name || '').split(',')[0].trim(),
                        lat: self._trimCoord(r.lat),
                        lon: self._trimCoord(r.lon)
                    };
                    self._saveProgress();
                    resultsEl.innerHTML = '';
                    onSelect();
                });
                resultsEl.appendChild(item);
            });
        } catch (e) {
            if (resultsEl) {
                resultsEl.textContent = 'Location search is unavailable right now — ' +
                    'you can skip this step and set the location later in the designer.';
            }
        } finally {
            this._searchBusy = false;
            if (searchBtn) searchBtn.disabled = false;
        }
    },

    // Coordinates travel as strings (widget properties are strings),
    // trimmed to 4 decimal places (Richtung 5).
    _trimCoord(value) {
        var n = parseFloat(value);
        return isFinite(n) ? n.toFixed(4) : String(value || '');
    },

    // --- step 3: refresh interval + advanced (Richtung 7) ------------------
    _renderStep3(content) {
        var self = this;
        var cardsHtml = '';
        this.INTERVALS.forEach(function (it) {
            cardsHtml +=
                '<button type="button" class="setup-interval-card" data-seconds="' + it.seconds + '">' +
                '<span class="setup-interval-value">' + it.label + '</span>' +
                '<span class="setup-interval-note">' + it.note + '</span>' +
                '</button>';
        });

        content.innerHTML =
            '<h2>How often should the panel refresh?</h2>' +
            '<div class="setup-interval-cards" id="setup-interval-cards">' + cardsHtml + '</div>' +
            '<p class="setup-hint">Every refresh briefly flashes the panel and takes ~25 s on ' +
            'the 6-color display &mdash; longer intervals extend panel life.</p>' +
            '<button type="button" class="setup-advanced-toggle" id="setup-advanced-toggle" ' +
            'aria-expanded="false">Advanced &#9662;</button>' +
            '<div class="setup-advanced" id="setup-advanced" style="display:none;">' +
            '<div class="property-group">' +
            '<label for="setup-render-quality">Render quality</label>' +
            '<select id="setup-render-quality" class="prop-input">' +
            '<option value="high">High (2&times; supersampling)</option>' +
            '<option value="medium">Medium (1.5&times; supersampling)</option>' +
            '<option value="fast">Fast (no supersampling)</option>' +
            '</select>' +
            '<p class="setup-hint">High looks best but takes the longest to render on a ' +
            'Raspberry Pi Zero.</p>' +
            '</div>' +
            '</div>' +
            '<p class="setup-error-text" id="setup-step3-error" style="display:none;"></p>' +
            '<div class="setup-wizard-actions">' +
            '<button type="button" class="btn-secondary setup-back" id="setup-step3-back">Back</button>' +
            '<button type="button" class="btn-primary setup-continue" id="setup-step3-continue">Continue</button>' +
            '</div>';

        var cards = content.querySelectorAll('.setup-interval-card');
        function applySelection() {
            cards.forEach(function (card) {
                card.classList.toggle('selected',
                    parseInt(card.dataset.seconds, 10) === self.state.refreshInterval);
            });
        }
        cards.forEach(function (card) {
            card.addEventListener('click', function () {
                self.state.refreshInterval = parseInt(card.dataset.seconds, 10);
                self._saveProgress();
                applySelection();
            });
        });
        applySelection();

        var qualitySelect = document.getElementById('setup-render-quality');
        qualitySelect.value = this.state.renderQuality || 'high';
        qualitySelect.addEventListener('input', function () {
            self.state.renderQuality = qualitySelect.value;
            self._saveProgress();
        });

        var advToggle = document.getElementById('setup-advanced-toggle');
        advToggle.addEventListener('click', function () {
            var panel = document.getElementById('setup-advanced');
            var opening = panel.style.display === 'none';
            panel.style.display = opening ? '' : 'none';
            advToggle.setAttribute('aria-expanded', opening ? 'true' : 'false');
            advToggle.innerHTML = opening ? 'Advanced &#9652;' : 'Advanced &#9662;';
        });

        document.getElementById('setup-step3-back')
            .addEventListener('click', function () { self.showStep(2); });
        document.getElementById('setup-step3-continue')
            .addEventListener('click', function () { self._saveSettings(); });
    },

    // The ONE settings write of the whole wizard (Richtung 7): display_type
    // from step 1, refresh_interval + render_quality from step 3 — saved
    // BEFORE step 5 so the gallery previews render panel-true.
    async _saveSettings() {
        var btn = document.getElementById('setup-step3-continue');
        var errEl = document.getElementById('setup-step3-error');
        if (btn) btn.disabled = true;
        if (errEl) errEl.style.display = 'none';
        try {
            var resp = await fetch('/update_settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    display_type: this.state.displayType,
                    refresh_interval: this.state.refreshInterval,
                    render_quality: this.state.renderQuality || 'high'
                })
            });
            if (!resp.ok) throw new Error('update_settings failed: ' + resp.status);
            // Display changed since the previews last rendered? Drop the
            // cache so step 5 renders against the new panel (Richtung 7).
            if (this._previewBaseline && this._previewBaseline !== this.state.displayType &&
                window.TemplateGallery) {
                TemplateGallery.invalidatePreviews();
            }
            this._previewBaseline = this.state.displayType;
            this.showStep(4);
        } catch (e) {
            if (errEl) {
                errEl.textContent = 'Saving the settings failed — please try again.';
                errEl.style.display = '';
            }
        } finally {
            if (btn) btn.disabled = false;
        }
    },

    // --- step 4: admin password (Richtung 8, mandatory) --------------------
    _renderStep4(content) {
        var self = this;
        if (this.state.passwordDone || (this.status && this.status.password_set)) {
            content.innerHTML =
                '<h2>Admin password</h2>' +
                '<div class="setup-password-done">' +
                '<span class="setup-check">&#10003;</span> Password already configured' +
                '</div>' +
                '<div class="setup-wizard-actions">' +
                '<button type="button" class="btn-secondary setup-back" id="setup-step4-back">Back</button>' +
                '<button type="button" class="btn-primary setup-continue" id="setup-step4-continue">Continue</button>' +
                '</div>';
            document.getElementById('setup-step4-back')
                .addEventListener('click', function () { self.showStep(3); });
            document.getElementById('setup-step4-continue')
                .addEventListener('click', function () { self.showStep(5); });
            return;
        }

        content.innerHTML =
            '<h2>Set an admin password</h2>' +
            '<p class="setup-intro">Without a password, anyone on your network can control ' +
            'this device.</p>' +
            '<form id="setup-password-form">' +
            '<div class="property-group">' +
            '<label for="setup-wizard-password">Password</label>' +
            '<input type="password" id="setup-wizard-password" class="prop-input" ' +
            'autocomplete="new-password">' +
            '</div>' +
            '<div class="property-group">' +
            '<label for="setup-wizard-password-repeat">Repeat password</label>' +
            '<input type="password" id="setup-wizard-password-repeat" class="prop-input" ' +
            'autocomplete="new-password">' +
            '</div>' +
            '<p class="setup-error-text" id="setup-step4-error" style="display:none;"></p>' +
            '<p id="setup-step4-login-hint" style="display:none;">' +
            '<a href="/login">Go to sign-in</a></p>' +
            '<div class="setup-wizard-actions">' +
            '<button type="button" class="btn-secondary setup-back" id="setup-step4-back">Back</button>' +
            '<button type="submit" class="btn-primary setup-continue" id="setup-step4-submit">' +
            'Set password &amp; continue</button>' +
            '</div>' +
            '</form>';

        document.getElementById('setup-step4-back')
            .addEventListener('click', function () { self.showStep(3); });
        document.getElementById('setup-password-form')
            .addEventListener('submit', function (event) {
                event.preventDefault();
                self._submitPassword();
            });
    },

    _showPasswordError(message, showLoginLink) {
        var errEl = document.getElementById('setup-step4-error');
        if (errEl) {
            errEl.textContent = message;
            errEl.style.display = message ? '' : 'none';
        }
        var hint = document.getElementById('setup-step4-login-hint');
        if (hint) hint.style.display = showLoginLink ? '' : 'none';
    },

    // Same sequence as the auth.js banner dialog (E5.1): POST /api/auth/setup
    // sets no session, so a 200 is followed IMMEDIATELY by POST /api/auth/login
    // — otherwise the global 401 interceptor would tear the wizard down on
    // the next API call. Login failure after a successful setup falls back
    // to the /login page (documented recovery path).
    async _submitPassword() {
        var password = document.getElementById('setup-wizard-password').value;
        var repeat = document.getElementById('setup-wizard-password-repeat').value;
        this._showPasswordError('', false);
        if (password.length < 1) {
            this._showPasswordError('Password must not be empty.', false);
            return;
        }
        if (password !== repeat) {
            this._showPasswordError('Passwords do not match.', false);
            return;
        }

        var submitBtn = document.getElementById('setup-step4-submit');
        if (submitBtn) submitBtn.disabled = true;
        try {
            var setupResp = await fetch('/api/auth/setup', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ password: password })
            });
            if (setupResp.status === 200) {
                var loginResp = await fetch('/api/auth/login', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ password: password })
                });
                if (loginResp.status === 200) {
                    this.state.passwordDone = true;
                    if (this.status) this.status.password_set = true;
                    this.showStep(5);
                    return;
                }
                // Password is set but no session (e.g. rate limit hit) —
                // the login page is the recovery path.
                window.location = '/login';
                return;
            }
            if (setupResp.status === 403) {
                // Two-browser race: someone else finished the setup first.
                this._showPasswordError('A password is already set — sign in instead.', true);
                return;
            }
            if (setupResp.status === 429) {
                var retryAfter = setupResp.headers.get('Retry-After');
                this._showPasswordError('Too many attempts. Try again in ' +
                    (retryAfter ? retryAfter + ' s.' : 'a minute.'), false);
                return;
            }
            this._showPasswordError('Setting the password failed.', false);
        } catch (e) {
            this._showPasswordError('Server not reachable.', false);
        } finally {
            if (submitBtn) submitBtn.disabled = false;
        }
    },

    // --- step 5: starter design from the template gallery (Richtung 9) -----
    _renderStep5(content) {
        var self = this;
        content.innerHTML =
            '<h2>Pick your first design</h2>' +
            '<p class="setup-intro">Every card is rendered exactly as your panel will show it. ' +
            'Tap one to put it on the display.</p>' +
            '<p class="setup-error-text" id="setup-step5-error" style="display:none;"></p>' +
            '<div class="designs-grid setup-wizard-grid" id="setup-wizard-templates-grid"></div>' +
            '<div class="setup-blank-row">' +
            '<button type="button" class="setup-blank-link" id="setup-blank-canvas">' +
            'Start with a blank canvas &rarr;</button>' +
            '</div>' +
            '<div class="setup-wizard-actions">' +
            '<button type="button" class="btn-secondary setup-back" id="setup-step5-back">Back</button>' +
            '</div>';

        document.getElementById('setup-step5-back')
            .addEventListener('click', function () { self.showStep(4); });
        document.getElementById('setup-blank-canvas')
            .addEventListener('click', function () {
                if (self._choosing) return;
                // Blank canvas: no create/activate/trigger — the default
                // design stays active (Richtung 9).
                self._created = null;
                self.state.completed = { blank: true };
                self.showStep(6);
            });

        if (window.TemplateGallery) {
            TemplateGallery.ensureLoaded(this._galleryHost());
        }
    },

    // Wizard host for the shared gallery: own grid, pause while the wizard
    // is not sitting on step 5, wizard select flow instead of useTemplate.
    _galleryHost() {
        return {
            gridId: 'setup-wizard-templates-grid',
            isPaused: function () {
                return !(SetupWizard._isOpen && SetupWizard.state &&
                    SetupWizard.state.step === 5);
            },
            onSelect: function (t) { SetupWizard._chooseTemplate(t); }
        };
    },

    // Wizard use flow: E3.5 instantiation pass (tokens + photo slot) plus
    // the wizard-only location pass, then create -> activate -> trigger.
    // Activation here is deliberate: the wizard click IS the decision.
    async _chooseTemplate(t) {
        if (this._choosing) return;
        this._choosing = true;
        var errEl = document.getElementById('setup-step5-error');
        if (errEl) errEl.style.display = 'none';
        try {
            var tpl = await TemplateGallery._getTemplateDesign(t);
            var design = JSON.parse(JSON.stringify(tpl));
            TemplateGallery._substituteTokens(design);
            await TemplateGallery._applyPhotoSlot(design);
            this._applyLocation(design);
            var name = TemplateGallery._uniqueName(t.name);

            var createResp = await fetch('/api/designs', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: name,
                    elements: design.elements,
                    canvas: design.canvas
                })
            });
            if (!createResp.ok) throw new Error('create failed: ' + createResp.status);
            var created = await createResp.json();
            TemplateGallery._createdNames.push(name);

            var activateResp = await fetch('/api/designs/' +
                encodeURIComponent(created.id) + '/activate', { method: 'POST' });
            if (!activateResp.ok) throw new Error('activate failed: ' + activateResp.status);

            // Trigger failure is not fatal: the interval refresh picks the
            // design up anyway.
            await fetch('/api/trigger_refresh', { method: 'POST' });

            this._created = { id: created.id, name: created.name, templateId: t.id, design: design };
            this.state.completed = { id: created.id, name: created.name, templateId: t.id };
            this.showStep(6);
        } catch (e) {
            if (errEl) {
                errEl.textContent = 'Creating the design failed — please try again.';
                errEl.style.display = '';
            }
        } finally {
            this._choosing = false;
        }
    },

    // Wizard-only location pass (Richtung 9): weather/forecast widgets get
    // the chosen coordinates, the location label (templateSlot:"location")
    // gets the short name in caps. Without a chosen location the pass is
    // skipped entirely — templates keep their Berlin defaults. The dashboard
    // use flow deliberately has no such pass (AC7).
    _applyLocation(design) {
        var loc = this.state.location;
        if (!loc) return;
        (design.elements || []).forEach(function (el) {
            if (el.type === 'widget_weather' || el.type === 'widget_forecast') {
                if (!el.properties) el.properties = {};
                el.properties.latitude = loc.lat;
                el.properties.longitude = loc.lon;
            }
            if (el.type === 'text' && el.properties &&
                el.properties.templateSlot === 'location') {
                el.properties.text = loc.short.toUpperCase();
                delete el.properties.templateSlot;
            }
        });
    },

    // --- success page + finish (Richtung 9) --------------------------------
    _renderSuccess(content) {
        var self = this;
        var completed = this.state.completed || {};
        var profile = null;
        for (var i = 0; i < this._profiles.length; i++) {
            if (this._profiles[i].type === this.state.displayType) profile = this._profiles[i];
        }
        var intervalLabel = Math.round(this.state.refreshInterval / 60) + ' min';
        var locationLabel = this.state.location
            ? this.state.location.short
            : 'Default (Berlin)';
        var designLabel = completed.blank ? 'Blank canvas' : (completed.name || '—');

        content.innerHTML =
            '<div class="setup-success">' +
            '<div class="setup-success-icon">&#10003;</div>' +
            '<h2>Your display is set up</h2>' +
            '<p class="setup-intro">The panel updates within about a minute.</p>' +
            '<div class="setup-success-preview" id="setup-success-preview"></div>' +
            '<dl class="setup-summary">' +
            '<div><dt>Display</dt><dd id="setup-summary-display"></dd></div>' +
            '<div><dt>Location</dt><dd id="setup-summary-location"></dd></div>' +
            '<div><dt>Refresh</dt><dd id="setup-summary-interval"></dd></div>' +
            '<div><dt>Design</dt><dd id="setup-summary-design"></dd></div>' +
            '</dl>' +
            '<p class="setup-error-text" id="setup-finish-error" style="display:none;"></p>' +
            '<div class="setup-wizard-actions setup-success-actions">' +
            '<button type="button" class="btn-primary setup-continue" id="setup-finish-btn">' +
            'Open Designer</button>' +
            '</div>' +
            '</div>';

        document.getElementById('setup-summary-display').textContent =
            profile ? profile.name : (this.state.displayType || '—');
        document.getElementById('setup-summary-location').textContent = locationLabel;
        document.getElementById('setup-summary-interval').textContent = 'Every ' + intervalLabel;
        document.getElementById('setup-summary-design').textContent = designLabel;

        this._loadSuccessPreview(document.getElementById('setup-success-preview'));

        document.getElementById('setup-finish-btn')
            .addEventListener('click', function () { self._finish(); });
    },

    // Exactly one panel-true render of the CREATED design (with location);
    // 503/busy falls back to the cached template preview or a placeholder —
    // no retry (Richtung 9 / AC6d).
    async _loadSuccessPreview(container) {
        if (!container) return;
        var placeholderText = 'Preview unavailable — the panel will still update.';
        if (!this._created) {
            if (this.state.completed && this.state.completed.blank) {
                container.textContent = 'A blank canvas is waiting in the designer.';
            } else {
                container.textContent = placeholderText;
            }
            return;
        }
        container.textContent = 'Rendering preview…';
        var url = null;
        try {
            var resp = await fetch('/api/preview_live', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: this._created.name,
                    elements: this._created.design.elements,
                    canvas: this._created.design.canvas
                })
            });
            if (!resp.ok) throw new Error('preview render failed: ' + resp.status);
            var blob = await resp.blob();
            url = URL.createObjectURL(blob);
        } catch (e) {
            url = (window.TemplateGallery &&
                TemplateGallery._previewUrls[this._created.templateId]) || null;
        }
        if (url) {
            container.textContent = '';
            var img = document.createElement('img');
            img.src = url;
            img.alt = 'Preview of ' + this._created.name;
            container.appendChild(img);
        } else {
            container.textContent = placeholderText;
        }
    },

    // Finish = persist the one-way latch, clear resume state, reload: the
    // stock page (banner/logout logic in auth.js) takes over.
    async _finish() {
        if (this._finishing) return;
        this._finishing = true;
        var btn = document.getElementById('setup-finish-btn');
        var errEl = document.getElementById('setup-finish-error');
        if (btn) btn.disabled = true;
        if (errEl) errEl.style.display = 'none';
        try {
            var resp = await fetch('/update_settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ setup_completed: true })
            });
            if (!resp.ok) throw new Error('update_settings failed: ' + resp.status);
            this._clearProgress();
            window.location.reload();
        } catch (e) {
            if (errEl) {
                errEl.textContent = 'Finishing the setup failed — please try again.';
                errEl.style.display = '';
            }
            if (btn) btn.disabled = false;
            this._finishing = false;
        }
    },

    // "Skip setup" (Richtung 6): same latch as finishing — after the reload
    // exactly the stock behavior remains (banner visible, no wizard, ever).
    async _skipSetup() {
        if (this._finishing) return;
        this._finishing = true;
        var btn = document.getElementById('setup-wizard-skip-btn');
        if (btn) btn.disabled = true;
        try {
            var resp = await fetch('/update_settings', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ setup_completed: true })
            });
            if (!resp.ok) throw new Error('update_settings failed: ' + resp.status);
            this._clearProgress();
            window.location.reload();
        } catch (e) {
            if (btn) btn.disabled = false;
            this._finishing = false;
        }
    },

    escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str == null ? '' : String(str);
        return div.innerHTML;
    }
};

(function () {
    'use strict';
    function onReady(fn) {
        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', fn);
        } else {
            fn();
        }
    }
    onReady(function () { SetupWizard.boot(); });
})();
