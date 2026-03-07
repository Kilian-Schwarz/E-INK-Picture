// Design Dashboard - Canva-like design management
var DesignDashboard = {
    designs: [],
    contextMenuDesignId: null,
    isOpen: false,

    async open() {
        this.isOpen = true;
        var overlay = document.getElementById('dashboard-overlay');
        if (overlay) overlay.classList.add('visible');
        await this.loadDesigns();
    },

    close() {
        this.isOpen = false;
        var overlay = document.getElementById('dashboard-overlay');
        if (overlay) overlay.classList.remove('visible');
        this.hideContextMenu();
    },

    async loadDesigns() {
        try {
            var resp = await fetch('/api/designs');
            var data = await resp.json();
            this.designs = data.designs || [];
            this.render();
        } catch (e) {
            console.error('Failed to load designs:', e);
        }
    },

    render() {
        this.renderRecent();
        this.renderAll();
    },

    renderRecent() {
        var container = document.getElementById('recent-designs-scroll');
        if (!container) return;
        container.innerHTML = '';

        // Show last 5 most recently edited
        var recent = this.designs.slice(0, 5);
        var self = this;
        recent.forEach(function(d) {
            container.appendChild(self.createCard(d, true));
        });
    },

    renderAll() {
        var container = document.getElementById('designs-grid');
        if (!container) return;
        container.innerHTML = '';

        // Create new design card
        var newCard = document.createElement('div');
        newCard.className = 'design-card';
        newCard.innerHTML =
            '<div class="design-card-preview" style="cursor:pointer;">' +
            '<div class="no-preview" style="font-size:32px;color:var(--accent);">+</div>' +
            '</div>' +
            '<div class="design-card-body">' +
            '<div class="design-card-name" style="color:var(--accent);">New Design</div>' +
            '<div class="design-card-date">Create a blank design</div>' +
            '</div>';
        newCard.addEventListener('click', function() {
            DesignDashboard.createNewDesign();
        });
        container.appendChild(newCard);

        var self = this;
        this.designs.forEach(function(d) {
            container.appendChild(self.createCard(d, false));
        });
    },

    createCard(design, isRecent) {
        var card = document.createElement('div');
        card.className = 'design-card' + (design.active ? ' active' : '');
        card.dataset.id = design.id;

        var dateStr = this.formatRelativeDate(design.updated_at);
        var createdStr = this.formatRelativeDate(design.created_at);

        card.innerHTML =
            '<div class="design-card-preview">' +
            '<div class="no-preview">' + (design.element_count || 0) + ' elements</div>' +
            (design.active ? '<span class="design-card-badge active-badge">AKTIV</span>' : '') +
            '</div>' +
            '<div class="design-card-body">' +
            '<div class="design-card-name" title="' + this.escapeHtml(design.name) + '">' + this.escapeHtml(design.name) + '</div>' +
            '<div class="design-card-date">Edited ' + dateStr + '</div>' +
            '</div>' +
            '<button class="design-card-menu-btn" title="Menu">&middot;&middot;&middot;</button>';

        var self = this;

        // Click to open design
        card.addEventListener('click', function(e) {
            if (e.target.classList.contains('design-card-menu-btn')) return;
            self.openDesign(design.id, design.name);
        });

        // Double-click on name to rename
        var nameEl = card.querySelector('.design-card-name');
        if (nameEl) {
            nameEl.addEventListener('dblclick', function(e) {
                e.stopPropagation();
                self.startRename(design.id, design.name, nameEl);
            });
        }

        // Menu button
        var menuBtn = card.querySelector('.design-card-menu-btn');
        if (menuBtn) {
            menuBtn.addEventListener('click', function(e) {
                e.stopPropagation();
                self.showContextMenu(e, design);
            });
        }

        return card;
    },

    async openDesign(id, name) {
        this.close();
        try {
            // Load the design by ID via new API, then load to canvas
            var resp = await fetch('/api/designs/' + encodeURIComponent(id));
            var design = await resp.json();
            Storage.currentDesignName = design.name;
            Storage.currentDesignId = design.id;
            Storage.loadDesignToCanvas(design);
            DesignDashboard.updateNameDisplay(design.name, design.active);
            setTimeout(function() { WidgetPreview.refreshAllWidgets(); }, 500);
            showNotification('Design loaded: ' + design.name);
        } catch (e) {
            showNotification('Failed to load design', 'error');
        }
    },

    async createNewDesign() {
        var name = prompt('Design name:');
        if (!name) return;

        try {
            var resp = await fetch('/api/designs', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ name: name })
            });
            var design = await resp.json();
            showNotification('Design created!', 'success');
            this.openDesign(design.id, design.name);
        } catch (e) {
            showNotification('Failed to create design', 'error');
        }
    },

    showContextMenu(event, design) {
        this.contextMenuDesignId = design.id;
        var menu = document.getElementById('design-context-menu');
        if (!menu) return;

        menu.innerHTML =
            '<button class="context-menu-item" data-action="rename">Rename</button>' +
            '<button class="context-menu-item" data-action="duplicate">Duplicate</button>' +
            '<button class="context-menu-item" data-action="activate">' +
            (design.active ? 'Active on Display' : 'Set as Active') + '</button>' +
            '<button class="context-menu-item" data-action="history">Show History</button>' +
            '<div class="context-menu-divider"></div>' +
            '<button class="context-menu-item danger" data-action="delete">Delete</button>';

        if (design.active) {
            menu.querySelector('[data-action="activate"]').disabled = true;
            menu.querySelector('[data-action="activate"]').style.opacity = '0.5';
        }

        // Position
        var rect = event.target.getBoundingClientRect();
        menu.style.top = rect.bottom + 4 + 'px';
        menu.style.left = Math.min(rect.left, window.innerWidth - 200) + 'px';
        menu.classList.add('visible');

        var self = this;
        menu.querySelectorAll('.context-menu-item').forEach(function(item) {
            item.addEventListener('click', function() {
                self.handleContextAction(item.dataset.action, design);
                self.hideContextMenu();
            });
        });
    },

    hideContextMenu() {
        var menu = document.getElementById('design-context-menu');
        if (menu) menu.classList.remove('visible');
    },

    async handleContextAction(action, design) {
        switch (action) {
            case 'rename':
                var newName = prompt('New name:', design.name);
                if (!newName) return;
                try {
                    await fetch('/api/designs/' + encodeURIComponent(design.id), {
                        method: 'PATCH',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ name: newName })
                    });
                    showNotification('Design renamed', 'success');
                    if (Storage.currentDesignId === design.id) {
                        Storage.currentDesignName = newName;
                        this.updateNameDisplay(newName, design.active);
                    }
                    this.loadDesigns();
                } catch (e) {
                    showNotification('Rename failed', 'error');
                }
                break;

            case 'duplicate':
                try {
                    await fetch('/api/designs/' + encodeURIComponent(design.id) + '/duplicate', {
                        method: 'POST'
                    });
                    showNotification('Design duplicated', 'success');
                    this.loadDesigns();
                } catch (e) {
                    showNotification('Duplicate failed', 'error');
                }
                break;

            case 'activate':
                try {
                    await fetch('/api/designs/' + encodeURIComponent(design.id) + '/activate', {
                        method: 'POST'
                    });
                    showNotification('Design set as active', 'success');
                    this.loadDesigns();
                } catch (e) {
                    showNotification('Activation failed', 'error');
                }
                break;

            case 'history':
                this.close();
                DesignHistory.open(design.id, design.name);
                break;

            case 'delete':
                if (!confirm('Delete "' + design.name + '"?')) return;
                try {
                    await fetch('/api/designs/' + encodeURIComponent(design.id), {
                        method: 'DELETE'
                    });
                    showNotification('Design deleted', 'success');
                    this.loadDesigns();
                } catch (e) {
                    showNotification('Delete failed', 'error');
                }
                break;
        }
    },

    startRename(id, currentName, el) {
        var input = document.createElement('input');
        input.type = 'text';
        input.value = currentName;
        input.className = 'design-name-input';
        input.style.display = 'block';
        input.style.width = el.offsetWidth + 40 + 'px';

        var parent = el.parentNode;
        parent.replaceChild(input, el);
        input.focus();
        input.select();

        var self = this;

        function finishRename() {
            var newName = input.value.trim();
            if (newName && newName !== currentName) {
                fetch('/api/designs/' + encodeURIComponent(id), {
                    method: 'PATCH',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name: newName })
                }).then(function() {
                    showNotification('Renamed', 'success');
                    self.loadDesigns();
                });
            } else {
                self.loadDesigns();
            }
        }

        input.addEventListener('blur', finishRename);
        input.addEventListener('keydown', function(e) {
            if (e.key === 'Enter') { input.blur(); }
            if (e.key === 'Escape') {
                input.value = currentName;
                input.blur();
            }
        });
    },

    updateNameDisplay(name, isActive) {
        var nameDisplay = document.getElementById('design-name-display');
        if (nameDisplay) {
            nameDisplay.textContent = name;
            nameDisplay.title = name;
        }
        var indicator = document.getElementById('active-indicator');
        if (indicator) {
            indicator.style.display = isActive ? 'inline-flex' : 'none';
        }
    },

    formatRelativeDate(isoStr) {
        if (!isoStr) return 'unknown';
        var d = new Date(isoStr);
        var now = new Date();
        var diffMs = now - d;
        var diffMin = Math.floor(diffMs / 60000);
        var diffHours = Math.floor(diffMin / 60);
        var diffDays = Math.floor(diffHours / 24);

        if (diffMin < 1) return 'just now';
        if (diffMin < 60) return diffMin + ' min ago';
        if (diffHours < 24) return diffHours + 'h ago';
        if (diffDays < 7) return diffDays + 'd ago';
        return d.toLocaleDateString();
    },

    escapeHtml(str) {
        var div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    },

    init() {
        var self = this;

        // Close button
        var closeBtn = document.getElementById('dashboard-close-btn');
        if (closeBtn) {
            closeBtn.addEventListener('click', function() {
                self.close();
            });
        }

        // Click outside context menu to close
        document.addEventListener('click', function(e) {
            if (!e.target.closest('.context-menu') && !e.target.classList.contains('design-card-menu-btn')) {
                self.hideContextMenu();
            }
        });

        // Design name click-to-edit in topbar
        var nameDisplay = document.getElementById('design-name-display');
        var nameInput = document.getElementById('design-name-input');
        if (nameDisplay && nameInput) {
            nameDisplay.addEventListener('dblclick', function() {
                nameDisplay.style.display = 'none';
                nameInput.style.display = 'block';
                nameInput.value = Storage.currentDesignName || '';
                nameInput.focus();
                nameInput.select();
            });

            nameInput.addEventListener('blur', function() {
                nameInput.style.display = 'none';
                nameDisplay.style.display = 'block';
                var newName = nameInput.value.trim();
                if (newName && newName !== Storage.currentDesignName && Storage.currentDesignId) {
                    fetch('/api/designs/' + encodeURIComponent(Storage.currentDesignId), {
                        method: 'PATCH',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ name: newName })
                    }).then(function() {
                        Storage.currentDesignName = newName;
                        nameDisplay.textContent = newName;
                        showNotification('Design renamed', 'success');
                    });
                }
            });

            nameInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter') nameInput.blur();
                if (e.key === 'Escape') {
                    nameInput.value = Storage.currentDesignName || '';
                    nameInput.blur();
                }
            });
        }
    }
};

// Design Version History
var DesignHistory = {
    designId: null,
    designName: null,

    async open(designId, designName) {
        this.designId = designId || Storage.currentDesignId;
        this.designName = designName || Storage.currentDesignName;

        if (!this.designId) {
            showNotification('No design selected', 'warning');
            return;
        }

        var sidebar = document.getElementById('history-sidebar');
        if (sidebar) {
            sidebar.classList.add('visible');
        }

        var title = document.getElementById('history-design-name');
        if (title) title.textContent = this.designName || 'Design';

        await this.loadHistory();
    },

    close() {
        var sidebar = document.getElementById('history-sidebar');
        if (sidebar) sidebar.classList.remove('visible');
    },

    async loadHistory() {
        var list = document.getElementById('history-list');
        if (!list) return;

        list.innerHTML = '<div class="history-empty">Loading...</div>';

        try {
            var resp = await fetch('/api/designs/' + encodeURIComponent(this.designId) + '/history');
            var entries = await resp.json();

            if (!entries || entries.length === 0) {
                list.innerHTML = '<div class="history-empty">No history entries yet.<br>History is created when you save the design.</div>';
                return;
            }

            list.innerHTML = '';
            var self = this;
            entries.forEach(function(entry, idx) {
                var item = document.createElement('div');
                item.className = 'history-item' + (idx === 0 ? ' current' : '');

                var timeStr = self.formatTimestamp(entry.timestamp);

                item.innerHTML =
                    '<div class="history-item-time">' + timeStr + '</div>' +
                    '<div class="history-item-desc">' + (entry.description || 'Snapshot') + '</div>' +
                    '<div class="history-item-actions">' +
                    '<button class="btn-primary" data-ts="' + entry.timestamp + '">Restore</button>' +
                    '<button class="btn-secondary" data-ts="' + entry.timestamp + '" data-action="preview">Preview</button>' +
                    '</div>';

                item.querySelector('.btn-primary').addEventListener('click', function(e) {
                    e.stopPropagation();
                    self.restoreSnapshot(entry.timestamp);
                });

                var previewBtn = item.querySelector('[data-action="preview"]');
                if (previewBtn) {
                    previewBtn.addEventListener('click', function(e) {
                        e.stopPropagation();
                        self.previewSnapshot(entry.timestamp);
                    });
                }

                list.appendChild(item);
            });
        } catch (e) {
            list.innerHTML = '<div class="history-empty">Failed to load history</div>';
        }
    },

    async restoreSnapshot(timestamp) {
        if (!confirm('Restore this version? The current state will be saved as a new snapshot.')) return;

        try {
            var resp = await fetch('/api/designs/' + encodeURIComponent(this.designId) +
                '/history/' + encodeURIComponent(timestamp) + '/restore', { method: 'POST' });
            var design = await resp.json();
            Storage.loadDesignToCanvas(design);
            showNotification('Design restored!', 'success');
            this.loadHistory();
        } catch (e) {
            showNotification('Restore failed', 'error');
        }
    },

    async previewSnapshot(timestamp) {
        try {
            var resp = await fetch('/api/designs/' + encodeURIComponent(this.designId) +
                '/history/' + encodeURIComponent(timestamp));
            var snap = await resp.json();
            if (snap && snap.design) {
                // Load to canvas without saving (preview mode)
                Storage.loadDesignToCanvas(snap.design);
                showNotification('Preview loaded. Save or click Restore to keep this version.', 'info');
            }
        } catch (e) {
            showNotification('Preview failed', 'error');
        }
    },

    formatTimestamp(ts) {
        if (!ts) return 'Unknown';
        // Format: 2006-01-02T15-04-05
        var parts = ts.split('T');
        if (parts.length === 2) {
            var date = parts[0];
            var time = parts[1].replace(/-/g, ':');
            var d = new Date(date + 'T' + time + 'Z');
            if (!isNaN(d.getTime())) {
                var now = new Date();
                var diffMs = now - d;
                var diffMin = Math.floor(diffMs / 60000);
                var diffHours = Math.floor(diffMin / 60);
                if (diffMin < 1) return 'Just now';
                if (diffMin < 60) return diffMin + ' min ago';
                if (diffHours < 24) return diffHours + 'h ago (' + time.substring(0, 5) + ')';
                return date + ' ' + time.substring(0, 5);
            }
        }
        return ts;
    },

    init() {
        var self = this;
        var closeBtn = document.getElementById('history-close-btn');
        if (closeBtn) {
            closeBtn.addEventListener('click', function() {
                self.close();
            });
        }
    }
};
