// Fabric.js Canvas initialization and management
// Canvas size: 800x480 (E-Ink display dimensions)
// Features: Zoom (25%-400%), Pan, Grid, Snap
const CanvasManager = {
    canvas: null,
    zoom: 1,
    gridEnabled: false,
    GRID_SIZE: 10,
    displayConfig: { width: 800, height: 480 },
    isPanning: false,
    lastPanX: 0,
    lastPanY: 0,

    init() {
        this.canvas = new fabric.Canvas('design-canvas', {
            width: 800,
            height: 480,
            backgroundColor: '#FFFFFF',
            selection: true,
            preserveObjectStacking: true,
        });

        // Custom control styling
        fabric.Object.prototype.set({
            borderColor: '#89b4fa',
            cornerColor: '#89b4fa',
            cornerSize: 8,
            cornerStyle: 'circle',
            transparentCorners: false,
            borderScaleFactor: 2,
        });

        this.setupZoom();
        this.setupSnap();
        this.setupSelectionEvents();
        this.setupGrid();
        this.centerCanvas();
    },

    centerCanvas() {
        const wrapper = document.getElementById('canvas-wrapper');
        const area = document.getElementById('canvas-area');
        if (!wrapper || !area) return;

        // Center the canvas in the viewport
        const areaRect = area.getBoundingClientRect();
        const canvasW = 800 * this.zoom;
        const canvasH = 480 * this.zoom;
        const padX = Math.max(40, (areaRect.width - canvasW) / 2);
        const padY = Math.max(40, (areaRect.height - canvasH) / 2);
        wrapper.style.padding = padY + 'px ' + padX + 'px';
    },

    setupZoom() {
        const canvasArea = document.getElementById('canvas-area');
        if (!canvasArea) return;

        canvasArea.addEventListener('wheel', (e) => {
            if (!e.ctrlKey) return;
            e.preventDefault();
            e.stopPropagation();

            const delta = e.deltaY;
            let newZoom = this.zoom * (0.999 ** delta);
            newZoom = Math.min(Math.max(newZoom, 0.25), 4);
            this.setZoom(newZoom);
        }, { passive: false });
    },

    setupSnap() {
        this.canvas.on('object:moving', (e) => {
            if (!this.gridEnabled) return;
            const obj = e.target;
            obj.set({
                left: Math.round(obj.left / this.GRID_SIZE) * this.GRID_SIZE,
                top: Math.round(obj.top / this.GRID_SIZE) * this.GRID_SIZE,
            });
        });
    },

    setupSelectionEvents() {
        this.canvas.on('selection:created', (e) => {
            PropertiesPanel.show(e.selected[0]);
            LayersPanel.refresh();
        });
        this.canvas.on('selection:updated', (e) => {
            PropertiesPanel.show(e.selected[0]);
            LayersPanel.refresh();
        });
        this.canvas.on('selection:cleared', () => {
            PropertiesPanel.hide();
            LayersPanel.refresh();
        });
        this.canvas.on('object:modified', (e) => {
            var obj = e.target;
            if (obj) {
                var type = obj.get('elementType');

                // For widget groups: absorb scale into dimensions, keep text fontSize unchanged
                if (type && type.startsWith('widget_') && obj.type === 'group') {
                    var sx = obj.scaleX || 1;
                    var sy = obj.scaleY || 1;
                    if (sx !== 1 || sy !== 1) {
                        var bgObj = obj._objects[0];
                        var labelObj = obj._objects[1];
                        var newW = Math.round((bgObj ? bgObj.width : obj.width) * sx);
                        var newH = Math.round((bgObj ? bgObj.height : obj.height) * sy);

                        if (bgObj) {
                            bgObj.set({ width: newW, height: newH, scaleX: 1, scaleY: 1 });
                        }
                        if (labelObj) {
                            // Keep fontSize unchanged — only resize the text box
                            labelObj.set({
                                width: newW - 16,
                                left: -newW / 2 + 8,
                                top: -newH / 2 + 4,
                                scaleX: 1,
                                scaleY: 1,
                            });
                        }
                        // Update clip path
                        if (obj.clipPath) {
                            obj.clipPath.set({
                                width: newW,
                                height: newH,
                                left: -newW / 2,
                                top: -newH / 2,
                            });
                        }
                        obj.set({ scaleX: 1, scaleY: 1 });
                        obj.addWithUpdate();
                        WidgetPreview.updatePreview(obj);
                    }
                }

                // For text: absorb scale into width/height, never distort font
                if (type === 'text' && (obj.type === 'textbox' || obj.type === 'i-text' || obj.type === 'text')) {
                    var tSx = obj.scaleX || 1;
                    var tSy = obj.scaleY || 1;
                    if (tSx !== 1 || tSy !== 1) {
                        var newWidth = Math.round(obj.width * tSx);
                        var newClipH = Math.round((obj.get('_clipH') || obj.height || 60) * tSy);
                        // Do NOT scale fontSize — keep it independent of widget size
                        obj.set({
                            width: newWidth,
                            scaleX: 1,
                            scaleY: 1,
                        });
                        obj.set('_clipH', newClipH);
                        // Update clipPath for overflow clipping
                        if (obj.clipPath) {
                            obj.clipPath.set({
                                width: newWidth,
                                height: newClipH,
                                left: -newWidth / 2,
                                top: -newClipH / 2,
                            });
                        }
                        obj.setCoords();
                    }
                }

                // For images: handle aspect ratio lock
                if (type === 'image' && obj.type === 'image') {
                    var data = obj.get('elementData') || {};
                    var props = data.properties || {};
                    if (props.resizeMode !== 'free') {
                        // Maintain aspect ratio from the latest scale
                        var imgSx = obj.scaleX || 1;
                        var imgSy = obj.scaleY || 1;
                        if (Math.abs(imgSx - imgSy) > 0.001) {
                            var avgScale = Math.max(imgSx, imgSy);
                            obj.set({ scaleX: avgScale, scaleY: avgScale });
                            obj.setCoords();
                        }
                    }
                }
            }

            HistoryManager.saveState();
            PropertiesPanel.updateFromCanvas();
            LayersPanel.refresh();
        });
        this.canvas.on('object:moving', () => {
            PropertiesPanel.updateFromCanvas();
        });
        this.canvas.on('object:scaling', () => {
            PropertiesPanel.updateFromCanvas();
        });
        this.canvas.on('object:rotating', () => {
            PropertiesPanel.updateFromCanvas();
        });

        // Live preview: debounced update on any canvas change
        var livePreviewTimeout = null;
        var triggerLivePreview = () => {
            if (livePreviewTimeout) clearTimeout(livePreviewTimeout);
            livePreviewTimeout = setTimeout(() => {
                if (typeof LivePreview !== 'undefined' && LivePreview.enabled) {
                    LivePreview.update();
                }
            }, 300);
        };
        this.canvas.on('object:modified', triggerLivePreview);
        this.canvas.on('object:added', triggerLivePreview);
        this.canvas.on('object:removed', triggerLivePreview);
    },

    setupGrid() {
        // Restore grid state from localStorage
        var savedGrid = localStorage.getItem('eink_grid_enabled');
        var savedGridSize = localStorage.getItem('eink_grid_size');
        if (savedGrid === 'true') this.gridEnabled = true;
        if (savedGridSize) this.GRID_SIZE = parseInt(savedGridSize) || 10;

        const toggle = document.getElementById('grid-toggle');
        if (toggle) {
            toggle.checked = this.gridEnabled;
            toggle.addEventListener('change', () => {
                this.gridEnabled = toggle.checked;
                localStorage.setItem('eink_grid_enabled', this.gridEnabled);
                this.renderGrid();
            });
        }

        // Update toolbar grid button state
        var gridBtn = document.getElementById('grid-toggle-btn');
        if (gridBtn && this.gridEnabled) gridBtn.classList.add('active');

        if (this.gridEnabled) this.renderGrid();
    },

    setGridSize(size) {
        this.GRID_SIZE = Math.max(5, Math.min(100, size));
        localStorage.setItem('eink_grid_size', this.GRID_SIZE);
        if (this.gridEnabled) this.renderGrid();
    },

    renderGrid() {
        // Remove existing grid lines
        const objects = this.canvas.getObjects();
        for (let i = objects.length - 1; i >= 0; i--) {
            if (objects[i].isGridLine) {
                this.canvas.remove(objects[i]);
            }
        }

        if (!this.gridEnabled) {
            this.canvas.renderAll();
            return;
        }

        const gridSize = this.GRID_SIZE;
        const width = this.displayConfig.width;
        const height = this.displayConfig.height;

        for (let x = 0; x <= width; x += gridSize) {
            const line = new fabric.Line([x, 0, x, height], {
                stroke: '#e0e0e0',
                strokeWidth: 0.5,
                selectable: false,
                evented: false,
                excludeFromExport: true,
            });
            line.isGridLine = true;
            this.canvas.add(line);
            this.canvas.sendToBack(line);
        }

        for (let y = 0; y <= height; y += gridSize) {
            const line = new fabric.Line([0, y, width, y], {
                stroke: '#e0e0e0',
                strokeWidth: 0.5,
                selectable: false,
                evented: false,
                excludeFromExport: true,
            });
            line.isGridLine = true;
            this.canvas.add(line);
            this.canvas.sendToBack(line);
        }

        this.canvas.renderAll();
    },

    setZoom(level) {
        this.zoom = Math.min(Math.max(level, 0.25), 4);
        this.canvas.setZoom(this.zoom);
        this.canvas.setDimensions({
            width: this.displayConfig.width * this.zoom,
            height: this.displayConfig.height * this.zoom
        });
        this.updateZoomDisplay();
        this.centerCanvas();
    },

    updateZoomDisplay() {
        const el = document.getElementById('zoom-level');
        if (el) el.textContent = Math.round(this.zoom * 100) + '%';
    },

    setDisplayConfig(config) {
        this.displayConfig = config;
        this.canvas.setDimensions({
            width: config.width * this.zoom,
            height: config.height * this.zoom,
        });
        this.centerCanvas();
    },

    getCanvas() {
        return this.canvas;
    }
};
