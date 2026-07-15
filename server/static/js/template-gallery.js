// Template Gallery - ships 8 ready-made designs as static JSONs
// (server/static/templates/, embedded via go:embed) and shows them as
// cards in the design dashboard.
//
// Preview contract (spec E3.5, Richtung 3): one POST /api/preview_live per
// template (panel mode, no raw param), strictly sequential (never more than
// one request in flight, E5.6 semaphore), object URLs cached per page
// session. The queue pauses while the host is hidden and resumes on the
// next ensureLoaded(); a failed card falls back to the template name and is
// NOT retried within the session.
//
// Host refactor (spec E2.3, Richtung 7): the gallery renders into whichever
// host is passed to ensureLoaded() — grid element id, pause criterion and
// select behavior are host-provided. Without a host argument the design
// dashboard defaults apply (behavior identical to pre-refactor). Preview
// cache and queue are shared across hosts: globally never more than one
// render in flight, no matter who is showing the cards.
var TemplateGallery = {
    manifest: null,
    _manifestLoading: false,
    _queueRunning: false,
    _using: false,
    _templateCache: {},   // id -> design JSON (as shipped, un-substituted)
    _previewUrls: {},     // id -> object URL (page-session cache)
    _previewFailed: {},   // id -> true (no auto-retry this session)
    _cards: {},           // id -> card DOM node (current host's grid)
    _createdNames: [],    // names created this session (collision suffix)
    _host: null,          // active host (see _dashboardHost for the shape)
    _renderedGrid: null,  // grid element the cards are currently rendered in

    // Default host: the design dashboard (pre-refactor behavior).
    _dashboardHost: {
        gridId: 'templates-grid',
        isPaused: function () { return !DesignDashboard.isOpen; },
        onSelect: function (t) { TemplateGallery.useTemplate(t); }
    },

    // Called from DesignDashboard.open() (no argument) or from an alternate
    // host such as the setup wizard ({gridId, isPaused, onSelect}). Loads
    // the manifest once per page session, renders the cards into the host's
    // grid, then (re)starts the shared preview queue.
    async ensureLoaded(host) {
        this._host = host || this._dashboardHost;
        if (!this.manifest) {
            if (this._manifestLoading) return;
            this._manifestLoading = true;
            try {
                var resp = await fetch('/static/templates/index.json');
                if (!resp.ok) throw new Error('manifest fetch failed: ' + resp.status);
                var data = await resp.json();
                this.manifest = data.templates || [];
            } catch (e) {
                console.error('Failed to load template manifest:', e);
                return;
            } finally {
                this._manifestLoading = false;
            }
        }
        var grid = document.getElementById(this._host.gridId);
        if (grid && grid !== this._renderedGrid) {
            this.renderCards();
        }
        this._runQueue();
    },

    renderCards() {
        var grid = document.getElementById(this._host.gridId);
        if (!grid) return;
        grid.innerHTML = '';
        this._cards = {};
        var self = this;
        this.manifest.forEach(function(t) {
            var card = document.createElement('div');
            card.className = 'design-card template-card';
            card.dataset.templateId = t.id;
            card.title = 'Use this template';

            var badge = '';
            if (t.setup && t.setup.length > 0) {
                badge = '<span class="design-card-badge setup-badge" title="' +
                    self.escapeHtml('Needs: ' + t.setup.join(', ')) + '">SETUP</span>';
            }

            card.innerHTML =
                '<div class="design-card-preview">' +
                '<div class="no-preview">Rendering…</div>' +
                badge +
                '</div>' +
                '<div class="design-card-body">' +
                '<div class="design-card-name">' + self.escapeHtml(t.name) + '</div>' +
                '<div class="design-card-date">' + self.escapeHtml(t.description) + '</div>' +
                '</div>';

            card.addEventListener('click', function() {
                self._host.onSelect(t);
            });

            self._cards[t.id] = card;
            grid.appendChild(card);

            // Re-renders (host switch) restore the page-session cache state
            // immediately instead of waiting for the queue.
            if (self._previewUrls[t.id]) {
                self._attachPreview(card, self._previewUrls[t.id], t.name);
            } else if (self._previewFailed[t.id]) {
                var ph = card.querySelector('.no-preview');
                if (ph) ph.textContent = t.name;
            }
        });
        this._renderedGrid = grid;
    },

    // Strictly sequential preview queue: at most one preview_live request in
    // flight; pauses when the host reports itself hidden; cached cards cost
    // nothing.
    async _runQueue() {
        if (this._queueRunning || !this.manifest) return;
        this._queueRunning = true;
        try {
            for (var i = 0; i < this.manifest.length; i++) {
                if (this._host.isPaused()) break; // pause; next ensureLoaded resumes
                var t = this.manifest[i];
                if (this._previewUrls[t.id] || this._previewFailed[t.id]) continue;
                await this._loadPreview(t);
            }
        } finally {
            this._queueRunning = false;
        }
    },

    // Drops the whole preview cache (spec E2.3, Richtung 7): revokes the
    // object URLs, forgets failures (a new display profile deserves a fresh
    // attempt) and forces a card re-render on the next ensureLoaded(). The
    // setup wizard calls this when display_type changed since the previews
    // were rendered, so the cards become panel-true again.
    invalidatePreviews() {
        var self = this;
        Object.keys(this._previewUrls).forEach(function(id) {
            URL.revokeObjectURL(self._previewUrls[id]);
        });
        this._previewUrls = {};
        this._previewFailed = {};
        this._renderedGrid = null;
    },

    _attachPreview(card, url, alt) {
        var preview = card.querySelector('.design-card-preview');
        if (!preview) return;
        var placeholder = preview.querySelector('.no-preview');
        if (placeholder) preview.removeChild(placeholder);
        var img = document.createElement('img');
        img.src = url;
        img.alt = alt;
        preview.insertBefore(img, preview.firstChild);
    },

    async _loadPreview(t) {
        var card = this._cards[t.id];
        try {
            var tpl = await this._getTemplateDesign(t);
            var design = JSON.parse(JSON.stringify(tpl));
            this._substituteTokens(design);
            // Panel mode (no raw param): the card shows what the display
            // will actually render, including quantization.
            var resp = await fetch('/api/preview_live', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(design),
            });
            if (!resp.ok) throw new Error('preview render failed: ' + resp.status);
            var blob = await resp.blob();
            this._previewUrls[t.id] = URL.createObjectURL(blob);
            if (card) {
                this._attachPreview(card, this._previewUrls[t.id], t.name);
            }
        } catch (e) {
            // 503 (renderer busy) or network error: show the template name
            // as fallback, no auto-retry (fresh page session retries).
            console.error('Template preview failed for ' + t.id + ':', e);
            this._previewFailed[t.id] = true;
            if (card) {
                var ph = card.querySelector('.no-preview');
                if (ph) ph.textContent = t.name;
            }
        }
    },

    async _getTemplateDesign(t) {
        if (this._templateCache[t.id]) return this._templateCache[t.id];
        var resp = await fetch('/static/templates/' + t.file);
        if (!resp.ok) throw new Error('template fetch failed: ' + resp.status);
        var design = await resp.json();
        this._templateCache[t.id] = design;
        return design;
    },

    // Substitution 1 (Richtung 4b): __NEXT_NEW_YEAR__ -> Jan 1st of next
    // year, local time, in the renderer's date format.
    _substituteTokens(design) {
        var replacement = (new Date().getFullYear() + 1) + '-01-01 00:00:00';
        (design.elements || []).forEach(function(el) {
            var props = el.properties;
            if (!props) return;
            Object.keys(props).forEach(function(key) {
                if (typeof props[key] === 'string' && props[key].indexOf('__NEXT_NEW_YEAR__') !== -1) {
                    props[key] = props[key].split('__NEXT_NEW_YEAR__').join(replacement);
                }
            });
        });
    },

    // Substitution 2 (Richtung 4c): photo slots get the newest media-library
    // image; without any upload the slot element is removed (the designed
    // placeholder beneath it stays).
    async _applyPhotoSlot(design) {
        var hasSlot = (design.elements || []).some(function(el) {
            return el.properties && el.properties.templateSlot === 'photo';
        });
        if (!hasSlot) return;

        var filename = null;
        try {
            var resp = await fetch('/api/media/images?page=1&limit=1');
            if (resp.ok) {
                var data = await resp.json();
                if (data.images && data.images.length > 0) {
                    filename = data.images[0].filename;
                }
            }
        } catch (e) {
            console.error('Media library lookup failed:', e);
        }

        design.elements = design.elements.filter(function(el) {
            if (!el.properties || el.properties.templateSlot !== 'photo') return true;
            if (!filename) return false;
            el.properties.image = filename;
            delete el.properties.templateSlot;
            return true;
        });
    },

    // Collision-safe name (Richtung 4d): the server does not deduplicate and
    // the legacy endpoints are name-based, so suffix " 2", " 3", ... against
    // the loaded dashboard list plus names created this session.
    _uniqueName(base) {
        var taken = {};
        (DesignDashboard.designs || []).forEach(function(d) { taken[d.name] = true; });
        this._createdNames.forEach(function(n) { taken[n] = true; });
        var name = base;
        var i = 2;
        while (taken[name]) {
            name = base + ' ' + i;
            i++;
        }
        return name;
    },

    // Use flow (Richtung 4): copy, substitute, create as INACTIVE design
    // (activation stays a deliberate user click), open in the designer.
    // This is the dashboard's select behavior; the setup wizard brings its
    // own (create + activate + trigger, spec E2.3 Richtung 9).
    async useTemplate(t) {
        if (this._using) return;
        this._using = true;
        try {
            var tpl = await this._getTemplateDesign(t);
            var design = JSON.parse(JSON.stringify(tpl));
            this._substituteTokens(design);
            await this._applyPhotoSlot(design);
            var name = this._uniqueName(t.name);

            var resp = await fetch('/api/designs', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    name: name,
                    elements: design.elements,
                    canvas: design.canvas,
                }),
            });
            if (!resp.ok) throw new Error('create failed: ' + resp.status);
            var created = await resp.json();
            this._createdNames.push(name);
            showNotification('Template added as "' + created.name + '"', 'success');
            DesignDashboard.openDesign(created.id, created.name);
        } catch (e) {
            console.error('Failed to use template ' + t.id + ':', e);
            showNotification('Failed to use template', 'error');
        } finally {
            this._using = false;
        }
    },

    escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }
};
