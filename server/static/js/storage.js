// Design save/load via API
var Storage = {
    currentDesignName: null,
    currentDesignId: null,

    async loadDesigns() {
        var resp = await fetch('/designs');
        return await resp.json();
    },

    async loadDesign(name) {
        var resp = await fetch('/get_design_by_name?name=' + encodeURIComponent(name));
        return await resp.json();
    },

    async saveDesign(name, designData, saveAsNew) {
        var resp = await fetch('/update_design', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: name,
                version: designData.version,
                canvas: designData.canvas,
                elements: designData.elements,
                conditionalRules: designData.conditionalRules,
                save_as_new: saveAsNew || false,
            }),
        });
        return await resp.json();
    },

    async setActive(name) {
        await fetch('/set_active_design', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name }),
        });
    },

    async deleteDesign(name) {
        await fetch('/delete_design', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name }),
        });
    },

    async cloneDesign(name) {
        await fetch('/clone_design', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: name }),
        });
    },

    async loadSettings() {
        var resp = await fetch('/settings');
        return await resp.json();
    },

    async loadDisplayProfiles() {
        var resp = await fetch('/display_profiles');
        return await resp.json();
    },

    // Convert canvas to design JSON for saving
    canvasToDesignJSON() {
        var canvas = CanvasManager.getCanvas();
        var elements = [];

        var zIdx = 0;
        canvas.getObjects().forEach(function(obj) {
            // Skip grid lines
            if (obj.isGridLine) return;

            var data = obj.get('elementData') || {};
            var type = obj.get('elementType') || 'unknown';

            var w = Math.round(obj.getScaledWidth ? obj.getScaledWidth() : (obj.width || 0));
            var h = Math.round(obj.getScaledHeight ? obj.getScaledHeight() : (obj.height || 0));

            // For text elements, use the clipPath height if available for consistent bounding box
            if (type === 'text' && obj.get('_clipH')) {
                h = Math.round(obj.get('_clipH'));
            }

            elements.push({
                id: obj.get('elementId') || 'elem_' + zIdx,
                type: type,
                x: Math.round(obj.left || 0),
                y: Math.round(obj.top || 0),
                width: w,
                height: h,
                rotation: Math.round(obj.angle || 0),
                zIndex: zIdx,
                locked: obj.lockMovementX || false,
                visible: obj.visible !== false,
                groupId: null,
                properties: Storage.extractProperties(type, obj, data.properties || {}),
                conditions: [],
            });
            zIdx++;
        });

        return {
            version: 2,
            canvas: {
                width: CanvasManager.displayConfig.width,
                height: CanvasManager.displayConfig.height,
                background: canvas.backgroundColor || '#FFFFFF',
            },
            elements: elements,
            conditionalRules: [],
        };
    },

    extractProperties(type, obj, savedProps) {
        switch (type) {
            case 'text':
                return {
                    text: obj.text || '',
                    fontFamily: obj.fontFamily || 'Arial',
                    fontSize: obj.fontSize || 24,
                    fontWeight: obj.fontWeight || 'normal',
                    fontStyle: obj.fontStyle || 'normal',
                    color: obj.fill || '#000000',
                    textAlign: obj.textAlign || 'left',
                };
            case 'shape':
                return {
                    fill: obj.fill || '#000000',
                    stroke: obj.stroke || '#000000',
                    strokeWidth: obj.strokeWidth || 1,
                    rx: obj.rx || 0,
                    ry: obj.ry || 0,
                };
            case 'image':
                return {
                    image: savedProps.image || '',
                    opacity: obj.opacity || 1,
                    resizeMode: savedProps.resizeMode || 'proportional',
                    cropX: savedProps.cropX,
                    cropY: savedProps.cropY,
                    cropW: savedProps.cropW,
                    cropH: savedProps.cropH,
                };
            default: {
                // Widgets: keep properties as-is (fontSize is already independent)
                return Object.assign({}, savedProps);
            }
        }
    },

    // Load design data onto canvas
    loadDesignToCanvas(design) {
        var canvas = CanvasManager.getCanvas();
        canvas.clear();

        // Handle v2 format
        if (design.version === 2 && design.elements) {
            canvas.backgroundColor = design.canvas ? design.canvas.background : '#FFFFFF';

            // Sort by zIndex
            var sortedElements = design.elements.slice().sort(function(a, b) {
                return (a.zIndex || 0) - (b.zIndex || 0);
            });

            sortedElements.forEach(function(elem) {
                var obj = ElementFactory.fromElement(elem);
                if (obj) {
                    if (elem.rotation) obj.set('angle', elem.rotation);
                    if (elem.locked) {
                        obj.set({
                            lockMovementX: true, lockMovementY: true,
                            lockScalingX: true, lockScalingY: true,
                            lockRotation: true, hasControls: false,
                        });
                    }
                    if (elem.visible === false) obj.set('visible', false);
                    canvas.add(obj);
                }
            });
        }
        // Handle v1 format (modules)
        else if (design.modules) {
            canvas.backgroundColor = '#FFFFFF';
            design.modules.forEach(function(mod) {
                var elem = Storage.migrateV1Module(mod);
                var obj = ElementFactory.fromElement(elem);
                if (obj) canvas.add(obj);
            });
        }

        canvas.renderAll();
        HistoryManager.clear();
        HistoryManager.saveState();
        LayersPanel.refresh();
    },

    // Migrate v1 module format to v2 element format
    migrateV1Module(mod) {
        var typeMap = {
            text: 'text',
            image: 'image',
            weather: 'widget_weather',
            datetime: 'widget_clock',
            timer: 'widget_timer',
            calendar: 'widget_calendar',
            news: 'widget_news',
            line: 'shape',
        };

        return {
            id: ElementFactory.generateId(),
            type: typeMap[mod.type] || mod.type,
            x: (mod.position ? mod.position.x : 0) - 200,
            y: (mod.position ? mod.position.y : 0) - 160,
            width: mod.size ? mod.size.width : 100,
            height: mod.size ? mod.size.height : 50,
            rotation: 0,
            zIndex: 0,
            locked: false,
            visible: true,
            properties: Storage.migrateV1Properties(mod),
        };
    },

    migrateV1Properties(mod) {
        var sd = mod.styleData || {};
        switch (mod.type) {
            case 'text':
                return {
                    text: mod.content || 'Text',
                    fontFamily: sd.font || 'Arial',
                    fontSize: parseInt(sd.fontSize) || 18,
                    fontWeight: sd.fontBold === 'true' ? 'bold' : 'normal',
                    color: sd.textColor || '#000000',
                    textAlign: sd.textAlign || 'left',
                };
            case 'image':
                return { image: sd.image || '' };
            case 'line':
                return { fill: sd.textColor || '#000000', strokeWidth: 1 };
            default:
                return {};
        }
    }
};
