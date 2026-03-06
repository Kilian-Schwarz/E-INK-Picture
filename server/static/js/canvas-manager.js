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
        this.canvas.on('object:modified', () => {
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
    },

    setupGrid() {
        const toggle = document.getElementById('grid-toggle');
        if (toggle) {
            toggle.addEventListener('change', () => {
                this.gridEnabled = toggle.checked;
                this.renderGrid();
            });
        }
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
