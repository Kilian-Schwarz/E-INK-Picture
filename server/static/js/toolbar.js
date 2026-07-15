// Top bar and left panel actions
var Toolbar = {
    init() {
        var self = this;

        // Widget click-to-add
        document.querySelectorAll('.widget-item').forEach(function(item) {
            item.addEventListener('click', function() {
                var type = item.dataset.type;
                self.addElement(type);
            });
        });

        // Layer actions
        var layerUp = document.getElementById('layer-up');
        var layerDown = document.getElementById('layer-down');
        var layerFront = document.getElementById('layer-front');
        var layerBack = document.getElementById('layer-back');
        var duplicateBtn = document.getElementById('duplicate-btn');
        var deleteBtn = document.getElementById('delete-btn');
        var lockBtn = document.getElementById('lock-btn');
        var previewBtn = document.getElementById('preview-btn');
        var settingsBtn = document.getElementById('settings-btn');

        if (layerUp) layerUp.addEventListener('click', function() { self.moveLayer('up'); });
        if (layerDown) layerDown.addEventListener('click', function() { self.moveLayer('down'); });
        if (layerFront) layerFront.addEventListener('click', function() { self.moveLayer('front'); });
        if (layerBack) layerBack.addEventListener('click', function() { self.moveLayer('back'); });
        if (duplicateBtn) duplicateBtn.addEventListener('click', function() { self.duplicateSelected(); });
        if (deleteBtn) deleteBtn.addEventListener('click', function() { self.deleteSelected(); });
        if (lockBtn) lockBtn.addEventListener('click', function() { self.toggleLock(); });

        // Preview
        if (previewBtn) previewBtn.addEventListener('click', function() { self.preview(); });
        this.initPreviewModal();

        // Settings
        if (settingsBtn) settingsBtn.addEventListener('click', function() {
            document.getElementById('settings-modal').style.display = 'flex';
        });
    },

    addElement(type) {
        var canvas = CanvasManager.getCanvas();
        var obj;

        switch (type) {
            case 'text':
                obj = ElementFactory.createText();
                break;
            case 'image':
                obj = ElementFactory.createImage();
                break;
            case 'shape':
                obj = ElementFactory.createShape();
                break;
            default:
                obj = ElementFactory.createWidget(type);
        }

        if (obj) {
            canvas.add(obj);
            canvas.setActiveObject(obj);
            canvas.renderAll();
            HistoryManager.saveState();
            LayersPanel.refresh();
        }
    },

    moveLayer(direction) {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;

        switch (direction) {
            case 'up': canvas.bringForward(obj); break;
            case 'down': canvas.sendBackwards(obj); break;
            case 'front': canvas.bringToFront(obj); break;
            case 'back': canvas.sendToBack(obj); break;
        }
        canvas.renderAll();
        LayersPanel.refresh();
        HistoryManager.saveState();
    },

    deleteSelected() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;
        canvas.remove(obj);
        canvas.discardActiveObject();
        canvas.renderAll();
        LayersPanel.refresh();
        HistoryManager.saveState();
    },

    duplicateSelected() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;

        obj.clone(function(cloned) {
            cloned.set({
                left: obj.left + 20,
                top: obj.top + 20,
            });
            cloned.set('elementId', ElementFactory.generateId());
            cloned.set('elementType', obj.get('elementType'));
            cloned.set('elementData', JSON.parse(JSON.stringify(obj.get('elementData') || {})));
            canvas.add(cloned);
            canvas.setActiveObject(cloned);
            canvas.renderAll();
            LayersPanel.refresh();
            HistoryManager.saveState();
        });
    },

    toggleLock() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;
        var locked = !obj.lockMovementX;
        obj.set({
            lockMovementX: locked,
            lockMovementY: locked,
            lockScalingX: locked,
            lockScalingY: locked,
            lockRotation: locked,
            hasControls: !locked,
        });
        canvas.renderAll();
        LayersPanel.refresh();
    },

    // --- Preview modal ---
    // "panel" = POST /api/preview_live without raw (quantized to the driver
    // palette, incl. calibration — what the display actually shows).
    // "raw" = same POST with the raw query param set (full-color composition).
    // Per modal session: design frozen once, max one fetch per mode (object
    // URLs cached), URLs revoked on close. Future mini-preview wiring contract
    // lives in specs/E3.6 — the LivePreview stub in designer.js stays a no-op.
    _previewSession: 0,
    _previewUrls: { panel: null, raw: null },
    _previewDesign: null,
    _previewLoading: false,

    initPreviewModal() {
        var self = this;
        var tabPanel = document.getElementById('preview-tab-panel');
        var tabRaw = document.getElementById('preview-tab-raw');
        if (tabPanel) tabPanel.addEventListener('click', function() { self.selectPreviewMode('panel'); });
        if (tabRaw) tabRaw.addEventListener('click', function() { self.selectPreviewMode('raw'); });

        // Additional observers next to the generic close handlers in
        // designer.js: revoke cached object URLs when the modal closes.
        var modal = document.getElementById('preview-modal');
        if (modal) {
            modal.querySelectorAll('.modal-close, .modal-overlay').forEach(function(el) {
                el.addEventListener('click', function() { self.closePreviewSession(); });
            });
        }
    },

    async preview() {
        // Fresh session: clear any stale state, freeze the design once.
        this.closePreviewSession();
        var designData = Storage.canvasToDesignJSON();
        designData.name = Storage.currentDesignName || 'preview';
        this._previewDesign = designData;

        document.getElementById('preview-image').src = '';
        this.setPreviewTabs('panel');
        this.setPreviewStatus('');
        document.getElementById('preview-modal').style.display = 'flex';
        await this.fetchPreviewMode('panel', true);
    },

    selectPreviewMode(mode) {
        if (this._previewLoading) return; // tabs are disabled while loading; belt and braces
        if (this._previewUrls[mode]) {
            // Cached: pure src swap, no request
            document.getElementById('preview-image').src = this._previewUrls[mode];
            this.setPreviewTabs(mode);
            this.setPreviewStatus('');
            return;
        }
        this.fetchPreviewMode(mode, false);
    },

    async fetchPreviewMode(mode, isInitial) {
        var session = this._previewSession;
        var previewImg = document.getElementById('preview-image');
        this._previewLoading = true;
        this.setPreviewTabsDisabled(true);
        this.setPreviewStatus('Rendering…');
        try {
            var url = mode === 'raw' ? '/api/preview_live?raw=true' : '/api/preview_live';
            var resp = await fetch(url, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(this._previewDesign),
            });
            if (!resp.ok) throw new Error('Preview failed: ' + resp.status);
            var blob = await resp.blob();
            var objectUrl = URL.createObjectURL(blob);
            if (session !== this._previewSession) {
                // Modal closed while rendering — drop the late response
                URL.revokeObjectURL(objectUrl);
                return;
            }
            this._previewUrls[mode] = objectUrl;
            previewImg.src = objectUrl;
            this.setPreviewTabs(mode);
            this.setPreviewStatus('');
        } catch (e) {
            if (session !== this._previewSession) return;
            if (isInitial) {
                // Initial open failed: fall back to the saved design
                // (quantized like the panel default; deliberately not cached)
                console.error('Live preview failed, falling back to saved:', e);
                var designName = document.getElementById('design-select') ? document.getElementById('design-select').value : '';
                var fallbackUrl = designName ? '/preview?name=' + encodeURIComponent(designName) : '/preview';
                previewImg.src = fallbackUrl + (fallbackUrl.indexOf('?') >= 0 ? '&' : '?') + 't=' + Date.now();
                this.setPreviewStatus('Live preview unavailable — showing saved design');
            } else {
                // Toggle fetch failed (e.g. 503 while the renderer is busy):
                // keep the old image and the old active tab, no auto-retry
                console.error('Preview render failed:', e);
                this.setPreviewStatus('Renderer busy — try again');
            }
        } finally {
            if (session === this._previewSession) {
                this._previewLoading = false;
                this.setPreviewTabsDisabled(false);
            }
        }
    },

    closePreviewSession() {
        this._previewSession++; // invalidates in-flight responses
        this._previewLoading = false;
        if (this._previewUrls.panel) URL.revokeObjectURL(this._previewUrls.panel);
        if (this._previewUrls.raw) URL.revokeObjectURL(this._previewUrls.raw);
        this._previewUrls = { panel: null, raw: null };
        this._previewDesign = null;
        this.setPreviewTabsDisabled(false);
    },

    setPreviewTabs(mode) {
        var tabPanel = document.getElementById('preview-tab-panel');
        var tabRaw = document.getElementById('preview-tab-raw');
        if (tabPanel) tabPanel.classList.toggle('active', mode === 'panel');
        if (tabRaw) tabRaw.classList.toggle('active', mode === 'raw');
    },

    setPreviewTabsDisabled(disabled) {
        var tabPanel = document.getElementById('preview-tab-panel');
        var tabRaw = document.getElementById('preview-tab-raw');
        if (tabPanel) tabPanel.disabled = disabled;
        if (tabRaw) tabRaw.disabled = disabled;
    },

    setPreviewStatus(text) {
        var status = document.getElementById('preview-status');
        if (!status) return;
        status.textContent = text;
        status.style.display = text ? 'block' : 'none';
    }
};
