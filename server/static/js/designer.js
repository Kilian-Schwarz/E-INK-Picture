// Main Designer Entry Point
var MediaModal = {
    callback: null,
    selectedMedia: null,

    open(callback) {
        this.callback = callback;
        this.selectedMedia = null;
        this.loadMedia();
        document.getElementById('media-modal').style.display = 'flex';
    },

    close() {
        document.getElementById('media-modal').style.display = 'none';
    },

    async loadMedia() {
        // Load images
        try {
            var resp = await fetch('/images_all');
            var images = await resp.json();
            var container = document.getElementById('media-library');
            container.innerHTML = '';
            var self = this;
            (images || []).forEach(function(img) {
                var item = document.createElement('div');
                item.className = 'media-item';
                item.innerHTML = '<img src="/image/' + img.name + '" alt="' + img.name + '"><span class="media-name">' + img.name + '</span>';
                item.addEventListener('click', function() {
                    container.querySelectorAll('.media-item').forEach(function(i) { i.classList.remove('selected'); });
                    item.classList.add('selected');
                    self.selectedMedia = img.name;
                });
                container.appendChild(item);
            });
        } catch (e) {
            console.error('Failed to load images:', e);
        }

        // Load fonts
        try {
            var fresp = await fetch('/fonts_all');
            var fonts = await fresp.json();
            var fcontainer = document.getElementById('fonts-library');
            fcontainer.innerHTML = '';
            (fonts || []).forEach(function(f) {
                var item = document.createElement('div');
                item.className = 'media-item font-item';
                item.textContent = f.name;
                fcontainer.appendChild(item);
            });
        } catch (e) {
            console.error('Failed to load fonts:', e);
        }
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
            Storage.loadDesignToCanvas(active);
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
                Storage.loadDesignToCanvas(design);
                showNotification('Design loaded: ' + name);
            } catch (err) {
                showNotification('Failed to load design', 'error');
            }
        });
    }

    // Save button
    var saveBtn = document.getElementById('save-btn');
    if (saveBtn) {
        saveBtn.addEventListener('click', async function() {
            var name = Storage.currentDesignName || 'Unnamed Design';
            var data = Storage.canvasToDesignJSON();
            try {
                await Storage.saveDesign(name, data, false);
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

    // Grid toggle
    var gridToggle = document.getElementById('grid-toggle');
    if (gridToggle) {
        gridToggle.addEventListener('change', function() {
            CanvasManager.gridEnabled = gridToggle.checked;
            CanvasManager.renderGrid();
        });
    }

    // Modal close buttons
    document.querySelectorAll('.modal-close, .modal-overlay').forEach(function(el) {
        el.addEventListener('click', function(e) {
            var modal = e.target.closest('.modal');
            if (modal) modal.style.display = 'none';
        });
    });

    // Media modal OK
    var mediaOkBtn = document.getElementById('media-ok-btn');
    if (mediaOkBtn) {
        mediaOkBtn.addEventListener('click', function() {
            if (MediaModal.selectedMedia && MediaModal.callback) {
                MediaModal.callback(MediaModal.selectedMedia);
            }
            MediaModal.close();
        });
    }

    // Media modal Cancel
    var mediaCancelBtn = document.getElementById('media-cancel-btn');
    if (mediaCancelBtn) {
        mediaCancelBtn.addEventListener('click', function() { MediaModal.close(); });
    }

    // Media upload
    var mediaUploadBtn = document.getElementById('media-upload-btn');
    if (mediaUploadBtn) {
        mediaUploadBtn.addEventListener('click', async function() {
            var fileInput = document.getElementById('media-upload-file');
            if (!fileInput.files.length) return;
            var formData = new FormData();
            formData.append('file', fileInput.files[0]);
            try {
                await fetch('/upload_image', { method: 'POST', body: formData });
                showNotification('File uploaded!', 'success');
                MediaModal.loadMedia();
            } catch (err) {
                showNotification('Upload failed', 'error');
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
            refreshNowBtn.textContent = 'Refreshing...';
            try {
                var res = await fetch('/api/trigger_refresh', { method: 'POST' });
                if (res.ok) {
                    refreshNowBtn.textContent = 'Triggered!';
                    showNotification('Display refresh triggered', 'success');
                } else {
                    refreshNowBtn.textContent = 'Error';
                    showNotification('Failed to trigger refresh', 'error');
                }
            } catch (e) {
                refreshNowBtn.textContent = 'Error';
                showNotification('Failed to trigger refresh', 'error');
            }
            setTimeout(function() {
                refreshNowBtn.textContent = 'Refresh Display';
                refreshNowBtn.disabled = false;
            }, 3000);
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

    // Client status polling
    async function updateClientStatus() {
        try {
            var res = await fetch('/api/refresh_status');
            var data = await res.json();
            var statusEl = document.getElementById('client-status');
            var statusBarEl = document.getElementById('status-client');
            if (data.last_client_refresh) {
                var ago = Math.round((Date.now() - new Date(data.last_client_refresh).getTime()) / 60000);
                var text = ago < 1 ? 'Display: just updated' : 'Display: updated ' + ago + 'min ago';
                if (statusEl) { statusEl.textContent = text; statusEl.className = 'status-info status-ok'; }
                if (statusBarEl) statusBarEl.textContent = text;
            } else {
                if (statusEl) { statusEl.textContent = 'Display: no updates yet'; statusEl.className = 'status-info status-warning'; }
                if (statusBarEl) statusBarEl.textContent = 'Display: waiting';
            }
        } catch (e) {
            var statusEl2 = document.getElementById('client-status');
            var statusBarEl2 = document.getElementById('status-client');
            if (statusEl2) { statusEl2.textContent = 'Client: offline'; statusEl2.className = 'status-info status-error'; }
            if (statusBarEl2) statusBarEl2.textContent = '';
        }
    }
    updateClientStatus();
    setInterval(updateClientStatus, 10000);

    showNotification('Designer loaded!', 'success');
});
