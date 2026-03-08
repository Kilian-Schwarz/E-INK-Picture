// Main Designer Entry Point

// Live Preview: renders the current canvas state via the server
var LivePreview = {
    enabled: false,
    _timeout: null,

    update() {
        // Called by debounced canvas events — no-op unless mini preview is shown
    }
};

// MediaModal compatibility wrapper — delegates to new MediaLibrary
var MediaModal = {
    callback: null,
    selectedMedia: null,

    open(callback) {
        MediaLibrary.open(callback);
    },

    close() {
        MediaLibrary.close();
    }
};

function showNotification(message, type) {
    type = type || 'info';
    var container = document.getElementById('notification-container');
    var note = document.createElement('div');
    note.className = 'notification ' + type;
    note.textContent = message;
    container.appendChild(note);
    setTimeout(function() {
        note.style.opacity = '0';
        setTimeout(function() { note.remove(); }, 300);
    }, 3000);
}

// Initialize everything when DOM is ready
document.addEventListener('DOMContentLoaded', async function() {
    // Initialize theme
    ThemeManager.init();

    // Initialize canvas
    CanvasManager.init();

    // Initialize properties panel
    PropertiesPanel.init();

    // Initialize toolbar
    Toolbar.init();

    // Initialize shortcuts
    Shortcuts.init();

    // Load settings and display profiles
    try {
        var settings = await Storage.loadSettings();
        if (settings && settings.display && settings.display.colors) {
            PropertiesPanel.displayColors = settings.display.colors;
        }

        // Update canvas size if display config exists
        if (settings && settings.display) {
            document.getElementById('status-display').textContent =
                settings.display.width + 'x' + settings.display.height + ' | ' + settings.display.name;

            if (settings.display.width && settings.display.height) {
                CanvasManager.setDisplayConfig({
                    width: settings.display.width,
                    height: settings.display.height
                });
            }
        }

        // Load display profiles for settings modal
        var profiles = await Storage.loadDisplayProfiles();
        var displaySelect = document.getElementById('display-select');
        if (displaySelect && profiles) {
            profiles.forEach(function(p) {
                var opt = document.createElement('option');
                opt.value = p.type;
                opt.textContent = p.name;
                if (settings && p.type === settings.display_type) opt.selected = true;
                displaySelect.appendChild(opt);
            });
        }
    } catch (e) {
        console.error('Failed to load settings:', e);
    }

    // Initialize media library and dashboard
    MediaLibrary.init();
    DesignDashboard.init();
    DesignHistory.init();

    // Load designs
    try {
        var designs = await Storage.loadDesigns();
        var designSelect = document.getElementById('design-select');
        if (designSelect && designs) {
            designs.forEach(function(d) {
                var opt = document.createElement('option');
                opt.value = d.name;
                opt.textContent = d.name + (d.active ? ' *' : '');
                if (d.active) opt.selected = true;
                designSelect.appendChild(opt);
            });
        }

        // Load active design
        if (designs && designs.length > 0) {
            var active = designs.find(function(d) { return d.active; }) || designs[0];
            Storage.currentDesignName = active.name;
            Storage.currentDesignId = active.id || '';
            Storage.loadDesignToCanvas(active);
            DesignDashboard.updateNameDisplay(active.name, active.active);
        }
    } catch (e) {
        console.error('Failed to load designs:', e);
    }

    // Design selector change
    var designSelectEl = document.getElementById('design-select');
    if (designSelectEl) {
        designSelectEl.addEventListener('change', async function(e) {
            var name = e.target.value;
            try {
                var design = await Storage.loadDesign(name);
                Storage.currentDesignName = name;
                Storage.currentDesignId = design.id || '';
                Storage.loadDesignToCanvas(design);
                DesignDashboard.updateNameDisplay(design.name, design.active);
                setTimeout(function() { WidgetPreview.refreshAllWidgets(); }, 500);
                showNotification('Design loaded: ' + name);
            } catch (err) {
                showNotification('Failed to load design', 'error');
            }
        });
    }

    // Save button — use new API if we have an ID
    var saveBtn = document.getElementById('save-btn');
    if (saveBtn) {
        saveBtn.addEventListener('click', async function() {
            var name = Storage.currentDesignName;
            if (!name) {
                name = prompt('Please name your design:');
                if (!name) return;
            }
            var data = Storage.canvasToDesignJSON();

            try {
                if (Storage.currentDesignId) {
                    // Update via new API (creates history snapshot)
                    await fetch('/api/designs/' + encodeURIComponent(Storage.currentDesignId), {
                        method: 'PUT',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            name: name,
                            elements: data.elements,
                            canvas: data.canvas,
                            keep_alive: false
                        })
                    });
                } else {
                    // Fallback to legacy API
                    await Storage.saveDesign(name, data, false);
                }
                showNotification('Design saved!', 'success');
            } catch (err) {
                showNotification('Failed to save', 'error');
            }
        });
    }

    // New design
    var newDesignBtn = document.getElementById('new-design-btn');
    if (newDesignBtn) {
        newDesignBtn.addEventListener('click', async function() {
            var name = prompt('Design name:');
            if (!name) return;
            var data = Storage.canvasToDesignJSON();
            try {
                await Storage.saveDesign(name, data, true);
                Storage.currentDesignName = name;
                showNotification('New design created!', 'success');
                location.reload();
            } catch (err) {
                showNotification('Failed to create', 'error');
            }
        });
    }

    // Clone design
    var cloneDesignBtn = document.getElementById('clone-design-btn');
    if (cloneDesignBtn) {
        cloneDesignBtn.addEventListener('click', async function() {
            if (!Storage.currentDesignName) return;
            try {
                await Storage.cloneDesign(Storage.currentDesignName);
                showNotification('Design cloned!', 'success');
                location.reload();
            } catch (err) {
                showNotification('Failed to clone', 'error');
            }
        });
    }

    // Delete design
    var deleteDesignBtn = document.getElementById('delete-design-btn');
    if (deleteDesignBtn) {
        deleteDesignBtn.addEventListener('click', async function() {
            if (!Storage.currentDesignName) return;
            if (!confirm('Delete design "' + Storage.currentDesignName + '"?')) return;
            try {
                await Storage.deleteDesign(Storage.currentDesignName);
                showNotification('Design deleted!', 'success');
                location.reload();
            } catch (err) {
                showNotification('Failed to delete', 'error');
            }
        });
    }

    // Zoom controls
    var zoomInBtn = document.getElementById('zoom-in-btn');
    var zoomOutBtn = document.getElementById('zoom-out-btn');
    var zoomResetBtn = document.getElementById('zoom-reset-btn');
    if (zoomInBtn) zoomInBtn.addEventListener('click', function() { CanvasManager.setZoom(CanvasManager.zoom + 0.1); });
    if (zoomOutBtn) zoomOutBtn.addEventListener('click', function() { CanvasManager.setZoom(CanvasManager.zoom - 0.1); });
    if (zoomResetBtn) zoomResetBtn.addEventListener('click', function() { CanvasManager.setZoom(1); });

    // Undo/Redo buttons
    var undoBtn = document.getElementById('undo-btn');
    var redoBtn = document.getElementById('redo-btn');
    if (undoBtn) undoBtn.addEventListener('click', function() { HistoryManager.undo(); });
    if (redoBtn) redoBtn.addEventListener('click', function() { HistoryManager.redo(); });

    // Theme toggle
    var themeToggle = document.getElementById('theme-toggle');
    if (themeToggle) themeToggle.addEventListener('click', function() { ThemeManager.toggle(); });

    // Grid toggle (checkbox in right panel)
    var gridToggle = document.getElementById('grid-toggle');
    if (gridToggle) {
        gridToggle.addEventListener('change', function() {
            CanvasManager.gridEnabled = gridToggle.checked;
            CanvasManager.renderGrid();
            var gridBtn = document.getElementById('grid-toggle-btn');
            if (gridBtn) gridBtn.classList.toggle('active', gridToggle.checked);
        });
    }

    // Grid toggle button in toolbar
    var gridToggleBtn = document.getElementById('grid-toggle-btn');
    if (gridToggleBtn) {
        gridToggleBtn.addEventListener('click', function() {
            CanvasManager.gridEnabled = !CanvasManager.gridEnabled;
            localStorage.setItem('eink_grid_enabled', CanvasManager.gridEnabled);
            CanvasManager.renderGrid();
            gridToggleBtn.classList.toggle('active', CanvasManager.gridEnabled);
            if (gridToggle) gridToggle.checked = CanvasManager.gridEnabled;
        });
    }

    // Grid size input
    var gridSizeInput = document.getElementById('grid-size-input');
    if (gridSizeInput) {
        gridSizeInput.value = CanvasManager.GRID_SIZE;
        gridSizeInput.addEventListener('change', function() {
            CanvasManager.setGridSize(parseInt(gridSizeInput.value) || 10);
        });
    }

    // Modal close buttons
    document.querySelectorAll('.modal-close, .modal-overlay').forEach(function(el) {
        el.addEventListener('click', function(e) {
            var modal = e.target.closest('.modal');
            if (modal) modal.style.display = 'none';
        });
    });

    // Media modal Cancel
    var mediaCancelBtn = document.getElementById('media-cancel-btn');
    if (mediaCancelBtn) {
        mediaCancelBtn.addEventListener('click', function() { MediaLibrary.close(); });
    }

    // Logo click — open dashboard
    var logoBtn = document.getElementById('logo-btn');
    if (logoBtn) {
        logoBtn.addEventListener('click', function() { DesignDashboard.open(); });
    }

    // History button
    var historyBtn = document.getElementById('history-btn');
    if (historyBtn) {
        historyBtn.addEventListener('click', function() {
            DesignHistory.open(Storage.currentDesignId, Storage.currentDesignName);
        });
    }

    // Activate button
    var activateBtn = document.getElementById('activate-btn');
    if (activateBtn) {
        activateBtn.addEventListener('click', async function() {
            if (!Storage.currentDesignId) {
                showNotification('No design selected', 'warning');
                return;
            }
            try {
                await fetch('/api/designs/' + encodeURIComponent(Storage.currentDesignId) + '/activate', { method: 'POST' });
                showNotification('Design set as active!', 'success');
                DesignDashboard.updateNameDisplay(Storage.currentDesignName, true);
            } catch (e) {
                showNotification('Failed to activate', 'error');
            }
        });
    }

    // Settings - display select change
    var settingsDisplaySelect = document.querySelector('#settings-modal #display-select');
    if (settingsDisplaySelect) {
        settingsDisplaySelect.addEventListener('change', async function() {
            try {
                await fetch('/update_settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ display_type: settingsDisplaySelect.value }),
                });
                showNotification('Settings saved!', 'success');
                location.reload();
            } catch (err) {
                showNotification('Failed to save settings', 'error');
            }
        });
    }

    // Canvas object events for layer panel updates
    var canvas = CanvasManager.getCanvas();
    canvas.on('object:added', function() { LayersPanel.refresh(); });
    canvas.on('object:removed', function() { LayersPanel.refresh(); });

    // Initial history state
    HistoryManager.saveState();

    // Refresh Display button
    var refreshNowBtn = document.getElementById('refresh-now-btn');
    if (refreshNowBtn) {
        refreshNowBtn.addEventListener('click', async function() {
            refreshNowBtn.disabled = true;
            refreshNowBtn.textContent = 'Triggering...';
            try {
                var res = await fetch('/api/trigger_refresh', { method: 'POST' });
                if (res.ok) {
                    refreshNowBtn.textContent = 'Triggered (updating within 30s)';
                    showNotification('Display refresh triggered — client will update on next poll', 'success');
                    // Poll for client update
                    var triggerTime = Date.now();
                    var pollId = setInterval(async function() {
                        try {
                            var statusRes = await fetch('/api/refresh_status');
                            var statusData = await statusRes.json();
                            if (statusData.last_client_refresh) {
                                var lastRefresh = new Date(statusData.last_client_refresh).getTime();
                                if (lastRefresh > triggerTime) {
                                    clearInterval(pollId);
                                    refreshNowBtn.textContent = 'Updated!';
                                    showNotification('Display updated successfully', 'success');
                                    updateClientStatus();
                                    setTimeout(function() {
                                        refreshNowBtn.textContent = 'Refresh Display';
                                        refreshNowBtn.disabled = false;
                                    }, 2000);
                                    return;
                                }
                            }
                        } catch (ignored) {}
                        if (Date.now() - triggerTime > 120000) {
                            clearInterval(pollId);
                            refreshNowBtn.textContent = 'Refresh Display';
                            refreshNowBtn.disabled = false;
                        }
                    }, 5000);
                } else {
                    refreshNowBtn.textContent = 'Error';
                    showNotification('Failed to trigger refresh', 'error');
                    setTimeout(function() {
                        refreshNowBtn.textContent = 'Refresh Display';
                        refreshNowBtn.disabled = false;
                    }, 3000);
                }
            } catch (e) {
                refreshNowBtn.textContent = 'Error';
                showNotification('Failed to trigger refresh', 'error');
                setTimeout(function() {
                    refreshNowBtn.textContent = 'Refresh Display';
                    refreshNowBtn.disabled = false;
                }, 3000);
            }
        });
    }

    // Refresh interval selector
    var refreshIntervalSelect = document.getElementById('refresh-interval');
    if (refreshIntervalSelect) {
        // Set current value from settings
        try {
            var settingsResp = await fetch('/settings');
            var settingsData = await settingsResp.json();
            if (settingsData.refresh_interval) {
                refreshIntervalSelect.value = String(settingsData.refresh_interval);
            }
        } catch (e) {}

        refreshIntervalSelect.addEventListener('change', async function() {
            try {
                await fetch('/update_settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ refresh_interval: parseInt(refreshIntervalSelect.value) }),
                });
                showNotification('Refresh interval updated', 'success');
            } catch (e) {
                showNotification('Failed to update interval', 'error');
            }
        });
    }

    // Render quality selector
    var renderQualitySelect = document.getElementById('render-quality');
    if (renderQualitySelect) {
        try {
            var qResp = await fetch('/settings');
            var qData = await qResp.json();
            if (qData.render_quality) {
                renderQualitySelect.value = qData.render_quality;
            }
        } catch (e) {}

        renderQualitySelect.addEventListener('change', async function() {
            try {
                await fetch('/update_settings', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ render_quality: renderQualitySelect.value }),
                });
                showNotification('Render quality updated', 'success');
            } catch (e) {
                showNotification('Failed to update render quality', 'error');
            }
        });
    }

    // Client status polling
    async function updateClientStatus() {
        try {
            var res = await fetch('/api/refresh_status');
            var data = await res.json();
            var statusEl = document.getElementById('client-status');
            var statusBarEl = document.getElementById('status-client');
            if (data.last_client_refresh && data.last_client_refresh !== '0001-01-01T00:00:00Z' && data.last_client_refresh !== '') {
                var lastUpdate = new Date(data.last_client_refresh);
                var diffMs = Date.now() - lastUpdate.getTime();
                var diffMin = Math.floor(diffMs / 60000);
                var diffHours = Math.floor(diffMin / 60);
                var text, cls;
                if (diffMin < 1) {
                    text = 'Display: just updated';
                    cls = 'status-info status-ok';
                } else if (diffMin < 60) {
                    text = 'Display: updated ' + diffMin + 'min ago';
                    cls = 'status-info status-ok';
                } else if (diffHours < 24) {
                    text = 'Display: updated ' + diffHours + 'h ago';
                    cls = 'status-info status-warning';
                } else {
                    text = 'Display: updated ' + Math.floor(diffHours / 24) + 'd ago';
                    cls = 'status-info status-error';
                }
                if (statusEl) { statusEl.textContent = text; statusEl.className = cls; }
                if (statusBarEl) statusBarEl.textContent = text;
            } else {
                if (statusEl) { statusEl.textContent = 'Display: never updated'; statusEl.className = 'status-info status-warning'; }
                if (statusBarEl) statusBarEl.textContent = 'Display: waiting';
            }
        } catch (e) {
            var statusEl2 = document.getElementById('client-status');
            var statusBarEl2 = document.getElementById('status-client');
            if (statusEl2) { statusEl2.textContent = 'Server: offline'; statusEl2.className = 'status-info status-error'; }
            if (statusBarEl2) statusBarEl2.textContent = '';
        }
    }
    updateClientStatus();
    setInterval(updateClientStatus, 15000);

    // Start live clock updates and periodic data refresh
    WidgetPreview.startClockUpdates();
    WidgetPreview.startDataRefresh();

    // Refresh all widget data after design load
    setTimeout(function() {
        WidgetPreview.refreshAllWidgets();
    }, 500);

    showNotification('Designer loaded!', 'success');
});
