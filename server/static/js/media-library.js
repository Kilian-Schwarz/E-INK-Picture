// Media Library with tabs, lazy loading, sorting, filtering
var MediaLibrary = {
    callback: null,
    selectedItems: [],
    images: [],
    fonts: [],
    page: 1,
    limit: 20,
    total: 0,
    sort: 'date_desc',
    search: '',
    loading: false,
    searchTimeout: null,
    activeTab: 'images',
    activeUpload: null,

    open(callback) {
        this.callback = callback;
        this.selectedItems = [];
        this.page = 1;
        this.images = [];
        this.activeTab = window.location.hash === '#media-fonts' ? 'fonts' : 'images';
        this.switchTab(this.activeTab);
        document.getElementById('media-modal').style.display = 'flex';
        this.loadImages(true);
        this.loadFonts();
    },

    close() {
        document.getElementById('media-modal').style.display = 'none';
        this.callback = null;
    },

    switchTab(tab) {
        this.activeTab = tab;
        window.location.hash = '#media-' + tab;
        document.querySelectorAll('#media-modal .tab-btn').forEach(function(btn) {
            btn.classList.toggle('active', btn.dataset.tab === tab);
        });
        document.querySelectorAll('#media-modal .tab-content').forEach(function(tc) {
            tc.classList.toggle('active', tc.id === 'media-tab-' + tab);
        });
    },

    async loadImages(reset) {
        if (this.loading) return;
        this.loading = true;

        if (reset) {
            this.page = 1;
            this.images = [];
            this.showSkeletons();
        }

        try {
            var params = new URLSearchParams({
                sort: this.sort,
                search: this.search,
                page: String(this.page),
                limit: String(this.limit)
            });
            var resp = await fetch('/api/media/images?' + params);
            var data = await resp.json();
            this.total = data.total || 0;

            if (reset) {
                this.images = data.images || [];
            } else {
                this.images = this.images.concat(data.images || []);
            }
            this.renderImages();
        } catch (e) {
            console.error('Failed to load images:', e);
        }
        this.loading = false;
    },

    async loadFonts() {
        try {
            var params = new URLSearchParams();
            var fontSearch = document.getElementById('media-font-search');
            if (fontSearch && fontSearch.value) {
                params.set('search', fontSearch.value);
            }
            var resp = await fetch('/api/media/fonts?' + params);
            var data = await resp.json();
            this.fonts = data.fonts || [];
            this.renderFonts();
        } catch (e) {
            console.error('Failed to load fonts:', e);
        }
    },

    showSkeletons() {
        var container = document.getElementById('media-images-grid');
        if (!container) return;
        container.innerHTML = '';
        for (var i = 0; i < 8; i++) {
            var skel = document.createElement('div');
            skel.className = 'skeleton skeleton-card';
            container.appendChild(skel);
        }
    },

    renderImages() {
        var container = document.getElementById('media-images-grid');
        if (!container) return;
        container.innerHTML = '';

        var self = this;
        this.images.forEach(function(img) {
            var card = document.createElement('div');
            card.className = 'media-card';
            if (self.selectedItems.indexOf(img.filename) !== -1) {
                card.classList.add('selected');
            }

            var thumbUrl = '/api/media/images/thumb/' + encodeURIComponent(img.filename);
            var fullUrl = '/image/' + encodeURIComponent(img.filename);

            card.innerHTML =
                '<input type="checkbox" class="media-card-checkbox" data-filename="' + img.filename + '"' +
                (self.selectedItems.indexOf(img.filename) !== -1 ? ' checked' : '') + '>' +
                '<div class="media-card-thumb"><img src="' + thumbUrl + '" alt="' + img.filename +
                '" loading="lazy" onerror="this.src=\'' + fullUrl + '\'"></div>' +
                '<div class="media-card-info">' +
                '<div class="media-card-name" title="' + img.filename + '">' + img.filename + '</div>' +
                '<div class="media-card-meta">' + self.formatSize(img.size) +
                (img.width ? ' &middot; ' + img.width + 'x' + img.height : '') + '</div>' +
                '</div>' +
                '<div class="media-card-overlay">' +
                '<button class="btn-primary media-select-btn" data-filename="' + img.filename + '">Select</button>' +
                '<button class="btn-danger media-delete-btn" data-filename="' + img.filename + '">Delete</button>' +
                '</div>';

            card.addEventListener('click', function(e) {
                if (e.target.classList.contains('media-card-checkbox')) return;
                if (e.target.classList.contains('media-delete-btn')) return;
                if (e.target.classList.contains('media-select-btn')) {
                    if (self.callback) {
                        self.callback(img.filename);
                    }
                    self.close();
                    return;
                }
                // Toggle selection
                var idx = self.selectedItems.indexOf(img.filename);
                if (e.shiftKey && self.selectedItems.length > 0) {
                    // Shift-click: select range
                } else {
                    if (idx !== -1) {
                        self.selectedItems.splice(idx, 1);
                    } else {
                        if (!e.ctrlKey && !e.metaKey) {
                            self.selectedItems = [];
                        }
                        self.selectedItems.push(img.filename);
                    }
                }
                self.renderImages();
                self.updateBulkActions();
            });

            // Checkbox handler
            var cb = card.querySelector('.media-card-checkbox');
            if (cb) {
                cb.addEventListener('click', function(e) {
                    e.stopPropagation();
                    var fn = e.target.dataset.filename;
                    var idx = self.selectedItems.indexOf(fn);
                    if (e.target.checked) {
                        if (idx === -1) self.selectedItems.push(fn);
                    } else {
                        if (idx !== -1) self.selectedItems.splice(idx, 1);
                    }
                    self.updateBulkActions();
                });
            }

            // Delete handler
            var delBtn = card.querySelector('.media-delete-btn');
            if (delBtn) {
                delBtn.addEventListener('click', function(e) {
                    e.stopPropagation();
                    self.deleteImage(img.filename);
                });
            }

            container.appendChild(card);
        });

        // Intersection Observer for lazy loading
        this.setupInfiniteScroll();
        this.updateBulkActions();
    },

    renderFonts() {
        var container = document.getElementById('media-fonts-list');
        if (!container) return;
        container.innerHTML = '';

        var self = this;
        if (this.fonts.length === 0) {
            container.innerHTML = '<div class="history-empty">No fonts uploaded yet</div>';
            return;
        }
        this.fonts.forEach(function(f) {
            var item = document.createElement('div');
            item.className = 'font-list-item';
            item.innerHTML =
                '<div><div class="font-name">' + f.filename + '</div>' +
                '<div class="font-meta">' + self.formatSize(f.size) +
                (f.uploaded_at ? ' &middot; ' + self.formatDate(f.uploaded_at) : '') + '</div></div>' +
                '<button class="btn-danger font-delete" data-filename="' + f.filename + '">Delete</button>';

            var delBtn = item.querySelector('.font-delete');
            if (delBtn) {
                delBtn.addEventListener('click', function() {
                    self.deleteFont(f.filename);
                });
            }
            container.appendChild(item);
        });
    },

    setupInfiniteScroll() {
        var container = document.getElementById('media-images-grid');
        if (!container) return;

        var sentinel = document.getElementById('media-scroll-sentinel');
        if (sentinel) sentinel.remove();

        if (this.images.length >= this.total) return;

        sentinel = document.createElement('div');
        sentinel.id = 'media-scroll-sentinel';
        sentinel.style.height = '1px';
        container.appendChild(sentinel);

        var self = this;
        var observer = new IntersectionObserver(function(entries) {
            if (entries[0].isIntersecting && !self.loading && self.images.length < self.total) {
                self.page++;
                self.loadImages(false);
            }
        }, { root: container.parentElement });
        observer.observe(sentinel);
    },

    updateBulkActions() {
        var bar = document.getElementById('media-bulk-actions');
        if (!bar) return;
        var count = this.selectedItems.length;
        if (count > 0) {
            bar.classList.add('visible');
            bar.querySelector('.bulk-count').textContent = count + ' selected';
        } else {
            bar.classList.remove('visible');
        }
    },

    async deleteImage(filename) {
        if (!confirm('Delete "' + filename + '"?')) return;
        try {
            await fetch('/api/media/images/' + encodeURIComponent(filename), { method: 'DELETE' });
            showNotification('Image deleted', 'success');
            this.loadImages(true);
        } catch (e) {
            showNotification('Delete failed', 'error');
        }
    },

    async deleteBulk() {
        if (!confirm('Delete ' + this.selectedItems.length + ' images?')) return;
        var self = this;
        for (var i = 0; i < this.selectedItems.length; i++) {
            try {
                await fetch('/api/media/images/' + encodeURIComponent(this.selectedItems[i]), { method: 'DELETE' });
            } catch (e) { /* continue */ }
        }
        self.selectedItems = [];
        showNotification('Images deleted', 'success');
        self.loadImages(true);
    },

    async deleteFont(filename) {
        if (!confirm('Delete font "' + filename + '"?')) return;
        try {
            await fetch('/api/media/fonts/' + encodeURIComponent(filename), { method: 'DELETE' });
            showNotification('Font deleted', 'success');
            this.loadFonts();
        } catch (e) {
            showNotification('Delete failed', 'error');
        }
    },

    // Uploads a single file via XHR. fetch() cannot report request body
    // progress, so XHR is the only way to get real byte counts.
    // Never rejects: the outcome is always reported through the result object.
    uploadFile(endpoint, file, onProgress) {
        var self = this;
        return new Promise(function(resolve) {
            var formData = new FormData();
            formData.append('file', file);

            var xhr = new XMLHttpRequest();
            self.activeUpload = xhr;
            xhr.open('POST', endpoint);

            xhr.upload.addEventListener('progress', function(e) {
                if (e.lengthComputable) onProgress(e.loaded);
            });

            xhr.addEventListener('load', function() {
                self.activeUpload = null;
                onProgress(file.size);
                if (xhr.status >= 200 && xhr.status < 300) {
                    resolve({ ok: true });
                    return;
                }
                var message = 'Unknown error';
                try {
                    var err = JSON.parse(xhr.responseText);
                    if (err && err.message) message = err.message;
                } catch (e) {
                    // Non-JSON error body (proxy error page, empty response)
                    message = 'HTTP ' + xhr.status;
                }
                resolve({ ok: false, message: message });
            });

            xhr.addEventListener('error', function() {
                self.activeUpload = null;
                resolve({ ok: false, message: file.name });
            });

            xhr.addEventListener('abort', function() {
                self.activeUpload = null;
                resolve({ ok: false, aborted: true });
            });

            xhr.send(formData);
        });
    },

    cancelUpload() {
        if (this.activeUpload) this.activeUpload.abort();
    },

    async uploadFiles(files, type) {
        var self = this;
        var progressEl = document.getElementById('media-upload-progress-' + type);
        var progressFill = progressEl ? progressEl.querySelector('.progress-fill') : null;
        var progressText = progressEl ? progressEl.querySelector('.progress-text') : null;

        var endpoint = type === 'fonts' ? '/api/media/fonts/upload' : '/api/media/images/upload';
        var total = files.length;
        var completed = 0;

        // Progress is tracked over the whole batch, so the bar keeps advancing
        // across files instead of restarting at 0% for each one.
        var totalBytes = 0;
        for (var b = 0; b < total; b++) totalBytes += files[b].size;
        var sentBytes = 0;

        // Only reveal the bar once the upload is still running after a moment.
        // Small files finish within a frame and would just make it flicker.
        var barShown = false;
        var revealTimer = setTimeout(function() {
            barShown = true;
            if (progressEl) progressEl.style.display = 'block';
        }, 150);

        function renderProgress(loaded, index) {
            var done = sentBytes + loaded;
            var percent = totalBytes > 0 ? Math.min(100, Math.round((done / totalBytes) * 100)) : 100;
            if (progressFill) progressFill.style.width = percent + '%';
            if (progressText) {
                progressText.textContent = total > 1
                    ? (index + 1) + ' / ' + total + ' — ' + percent + '%'
                    : percent + '%';
            }
        }

        renderProgress(0, 0);

        for (var i = 0; i < total; i++) {
            var file = files[i];
            var result = await this.uploadFile(endpoint, file, (function(index) {
                return function(loaded) { renderProgress(loaded, index); };
            })(i));

            if (result.aborted) break;
            if (result.ok) {
                completed++;
            } else {
                showNotification('Upload failed: ' + result.message, 'error');
            }

            // Count the file either way so the bar reflects work processed
            sentBytes += file.size;
            renderProgress(0, i);
        }

        clearTimeout(revealTimer);
        if (progressEl) {
            if (barShown) {
                setTimeout(function() {
                    progressEl.style.display = 'none';
                    if (progressFill) progressFill.style.width = '0%';
                }, 1000);
            } else {
                progressEl.style.display = 'none';
                if (progressFill) progressFill.style.width = '0%';
            }
        }

        if (completed > 0) {
            showNotification(completed + ' file(s) uploaded', 'success');
            if (type === 'fonts') {
                self.loadFonts();
            } else {
                self.loadImages(true);
            }
        }
    },

    formatSize(bytes) {
        if (!bytes) return '';
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    },

    formatDate(isoStr) {
        if (!isoStr) return '';
        var d = new Date(isoStr);
        return d.toLocaleDateString();
    },

    init() {
        var self = this;

        // Tab buttons
        document.querySelectorAll('#media-modal .tab-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                self.switchTab(btn.dataset.tab);
            });
        });

        // Sort dropdown
        var sortEl = document.getElementById('media-image-sort');
        if (sortEl) {
            sortEl.addEventListener('change', function() {
                self.sort = sortEl.value;
                self.loadImages(true);
            });
        }

        // Search input with debounce
        var searchEl = document.getElementById('media-image-search');
        if (searchEl) {
            searchEl.addEventListener('input', function() {
                clearTimeout(self.searchTimeout);
                self.searchTimeout = setTimeout(function() {
                    self.search = searchEl.value;
                    self.loadImages(true);
                }, 200);
            });
        }

        // Font search
        var fontSearch = document.getElementById('media-font-search');
        if (fontSearch) {
            fontSearch.addEventListener('input', function() {
                clearTimeout(self.searchTimeout);
                self.searchTimeout = setTimeout(function() {
                    self.loadFonts();
                }, 200);
            });
        }

        // Image upload zone
        this.setupUploadZone('media-upload-zone-images', 'images', '.png,.jpg,.jpeg');
        this.setupUploadZone('media-upload-zone-fonts', 'fonts', '.ttf,.otf');

        // Bulk delete
        var bulkDeleteBtn = document.getElementById('media-bulk-delete');
        if (bulkDeleteBtn) {
            bulkDeleteBtn.addEventListener('click', function() {
                self.deleteBulk();
            });
        }

        // Select button (for image picker mode)
        var selectBtn = document.getElementById('media-select-btn');
        if (selectBtn) {
            selectBtn.addEventListener('click', function() {
                if (self.selectedItems.length > 0 && self.callback) {
                    self.callback(self.selectedItems[0]);
                    self.close();
                }
            });
        }
    },

    setupUploadZone(id, type, accept) {
        var zone = document.getElementById(id);
        if (!zone) return;
        var self = this;

        var input = zone.querySelector('input[type="file"]');
        if (!input) {
            input = document.createElement('input');
            input.type = 'file';
            input.accept = accept;
            input.multiple = true;
            input.style.display = 'none';
            zone.appendChild(input);
        }

        zone.addEventListener('click', function() { input.click(); });

        zone.addEventListener('dragover', function(e) {
            e.preventDefault();
            zone.classList.add('dragover');
        });
        zone.addEventListener('dragleave', function() {
            zone.classList.remove('dragover');
        });
        zone.addEventListener('drop', function(e) {
            e.preventDefault();
            zone.classList.remove('dragover');
            if (e.dataTransfer.files.length > 0) {
                self.uploadFiles(e.dataTransfer.files, type);
            }
        });

        input.addEventListener('change', function() {
            if (input.files.length > 0) {
                self.uploadFiles(input.files, type);
                input.value = '';
            }
        });
    }
};
