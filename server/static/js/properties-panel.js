// Right panel - Properties editor
// Shows context-dependent properties based on selected element type
var PropertiesPanel = {
    currentObject: null,
    displayColors: ['#000000', '#FFFFFF'],
    _layoutCache: {},

    init(colors) {
        if (colors) this.displayColors = colors;
        this.setupInputListeners();
    },

    show(obj) {
        this.currentObject = obj;
        document.getElementById('canvas-properties').style.display = 'none';
        document.getElementById('element-properties').style.display = 'block';
        this.updateFromCanvas();
        this.showTypeSpecificProperties(obj);
    },

    hide() {
        this.currentObject = null;
        document.getElementById('canvas-properties').style.display = 'block';
        document.getElementById('element-properties').style.display = 'none';
        document.getElementById('status-selection').textContent = 'No selection';
    },

    updateFromCanvas() {
        if (!this.currentObject) return;
        var obj = this.currentObject;
        var x = document.getElementById('prop-x');
        var y = document.getElementById('prop-y');
        var w = document.getElementById('prop-w');
        var h = document.getElementById('prop-h');
        var rot = document.getElementById('prop-rotation');

        if (x) x.value = Math.round(obj.left || 0);
        if (y) y.value = Math.round(obj.top || 0);
        if (w) w.value = Math.round(obj.getScaledWidth ? obj.getScaledWidth() : (obj.width || 0));
        if (h) h.value = Math.round(obj.getScaledHeight ? obj.getScaledHeight() : (obj.height || 0));
        if (rot) rot.value = Math.round(obj.angle || 0);

        // Update status bar
        var type = obj.get('elementType') || 'unknown';
        document.getElementById('status-selection').textContent =
            'Selected: ' + type + ' (' + Math.round(obj.left) + ', ' + Math.round(obj.top) + ')';
    },

    setupInputListeners() {
        var self = this;
        var ids = ['prop-x', 'prop-y', 'prop-w', 'prop-h', 'prop-rotation'];
        ids.forEach(function(id) {
            var el = document.getElementById(id);
            if (!el) return;
            el.addEventListener('change', function() {
                if (!self.currentObject) return;
                var canvas = CanvasManager.getCanvas();
                var obj = self.currentObject;

                if (id === 'prop-x') obj.set('left', parseFloat(el.value));
                if (id === 'prop-y') obj.set('top', parseFloat(el.value));
                if (id === 'prop-w') {
                    var scaleX = parseFloat(el.value) / obj.width;
                    obj.set('scaleX', scaleX);
                }
                if (id === 'prop-h') {
                    var scaleY = parseFloat(el.value) / obj.height;
                    obj.set('scaleY', scaleY);
                }
                if (id === 'prop-rotation') obj.set('angle', parseFloat(el.value));

                obj.setCoords();
                canvas.renderAll();
                HistoryManager.saveState();
            });
        });
    },

    showTypeSpecificProperties(obj) {
        var container = document.getElementById('type-specific-properties');
        container.innerHTML = '';

        var type = obj.get('elementType');
        var data = obj.get('elementData') || {};
        var props = data.properties || {};

        switch (type) {
            case 'text':
                this.renderTextProperties(container, obj, props);
                break;
            case 'image':
                this.renderImageProperties(container, obj, props);
                break;
            case 'shape':
                this.renderShapeProperties(container, obj, props);
                break;
            default:
                if (type && type.startsWith('widget_')) {
                    this.renderWidgetProperties(container, type, obj, props);
                }
        }

        // Lock aspect ratio for all types
        this.renderLockAspectRatio(container, obj, type);
    },

    renderLockAspectRatio(container, obj, type) {
        var defaultLocked = (type === 'image' || (type && type.startsWith('widget_')));
        var isLocked = obj.lockUniScaling !== undefined ? obj.lockUniScaling : defaultLocked;

        var html = '<div class="property-group" style="margin-top:8px;border-top:1px solid var(--border);padding-top:8px;">' +
            '<label class="checkbox-label"><input type="checkbox" id="prop-lock-aspect"' + (isLocked ? ' checked' : '') + '> Lock Aspect Ratio</label></div>';
        container.insertAdjacentHTML('beforeend', html);

        document.getElementById('prop-lock-aspect').addEventListener('change', function(e) {
            obj.set('lockUniScaling', e.target.checked);
            CanvasManager.getCanvas().renderAll();
        });
    },

    renderTextProperties(container, obj, props) {
        var self = this;
        var html = '' +
            '<div class="property-group">' +
            '    <label>Font</label>' +
            '    <select id="prop-font-family" class="prop-input">' +
            '        <option value="Arial">Arial</option>' +
            '        <option value="Times New Roman">Times New Roman</option>' +
            '        <option value="Courier New">Courier New</option>' +
            '        <option value="Georgia">Georgia</option>' +
            '        <option value="Verdana">Verdana</option>' +
            '    </select>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Size</label>' +
            '    <input type="number" id="prop-font-size" class="prop-input" value="' + (obj.fontSize || 24) + '" min="8" max="200">' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Style</label>' +
            '    <div class="button-group">' +
            '        <button id="prop-bold" class="style-btn ' + (obj.fontWeight === 'bold' ? 'active' : '') + '">B</button>' +
            '        <button id="prop-italic" class="style-btn ' + (obj.fontStyle === 'italic' ? 'active' : '') + '">I</button>' +
            '    </div>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Align</label>' +
            '    <div class="button-group">' +
            '        <button class="align-btn ' + (obj.textAlign === 'left' ? 'active' : '') + '" data-align="left">L</button>' +
            '        <button class="align-btn ' + (obj.textAlign === 'center' ? 'active' : '') + '" data-align="center">C</button>' +
            '        <button class="align-btn ' + (obj.textAlign === 'right' ? 'active' : '') + '" data-align="right">R</button>' +
            '    </div>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Color</label>' +
            '    <div class="color-swatches" id="text-color-swatches"></div>' +
            '</div>';
        container.innerHTML = html;

        // Render color swatches
        this.renderColorSwatches('text-color-swatches', obj.fill, function(color) {
            obj.set('fill', color);
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        // Font family
        var fontSelect = document.getElementById('prop-font-family');
        fontSelect.value = obj.fontFamily || 'Arial';
        fontSelect.addEventListener('change', function() {
            obj.set('fontFamily', fontSelect.value);
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        // Font size
        document.getElementById('prop-font-size').addEventListener('change', function(e) {
            obj.set('fontSize', parseInt(e.target.value));
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        // Bold toggle
        document.getElementById('prop-bold').addEventListener('click', function(e) {
            var isBold = obj.fontWeight === 'bold';
            obj.set('fontWeight', isBold ? 'normal' : 'bold');
            e.target.classList.toggle('active');
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        // Italic toggle
        document.getElementById('prop-italic').addEventListener('click', function(e) {
            var isItalic = obj.fontStyle === 'italic';
            obj.set('fontStyle', isItalic ? 'normal' : 'italic');
            e.target.classList.toggle('active');
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        // Align buttons
        container.querySelectorAll('.align-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                container.querySelectorAll('.align-btn').forEach(function(b) { b.classList.remove('active'); });
                btn.classList.add('active');
                obj.set('textAlign', btn.dataset.align);
                CanvasManager.getCanvas().renderAll();
                HistoryManager.saveState();
            });
        });
    },

    renderImageProperties(container, obj, props) {
        var self = this;
        var resizeMode = props.resizeMode || 'proportional';
        var hasCrop = props.cropX || props.cropY || props.cropW || props.cropH;
        container.innerHTML = '' +
            '<div class="property-group">' +
            '    <label>Image</label>' +
            '    <button id="select-image-btn" class="btn-secondary">Select Image</button>' +
            '    <span id="current-image-name" class="text-muted" style="display:block;margin-top:4px;font-size:0.85em;">' + (props.image || 'None') + '</span>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Resize Mode</label>' +
            '    <div class="button-group">' +
            '        <button class="style-btn resize-mode-btn' + (resizeMode === 'proportional' ? ' active' : '') + '" data-mode="proportional" title="Proportional">Prop</button>' +
            '        <button class="style-btn resize-mode-btn' + (resizeMode === 'free' ? ' active' : '') + '" data-mode="free" title="Free transform">Free</button>' +
            '        <button class="style-btn resize-mode-btn' + (resizeMode === 'crop' ? ' active' : '') + '" data-mode="crop" title="Crop">Crop</button>' +
            '    </div>' +
            '</div>' +
            '<div class="property-group">' +
            '    <button id="open-crop-btn" class="btn-secondary" style="width:100%;">Crop Settings' + (hasCrop ? ' (active)' : '') + '</button>' +
            '</div>';

        document.getElementById('select-image-btn').addEventListener('click', function() {
            MediaModal.open(function(imageName) {
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                data.properties.image = imageName;
                obj.set('elementData', data);

                // Load and display image on canvas
                fabric.Image.fromURL('/image/' + imageName, function(img) {
                    var canvas = CanvasManager.getCanvas();
                    var left = obj.left;
                    var top = obj.top;
                    var w = obj.getScaledWidth();
                    var h = obj.getScaledHeight();

                    canvas.remove(obj);
                    img.set({
                        left: left,
                        top: top,
                        scaleX: w / img.width,
                        scaleY: h / img.height,
                    });
                    img.set('elementId', obj.get('elementId'));
                    img.set('elementType', 'image');
                    img.set('elementData', data);
                    img.set('lockUniScaling', data.properties.resizeMode !== 'free');
                    canvas.add(img);
                    canvas.setActiveObject(img);
                    self.currentObject = img;
                    canvas.renderAll();
                    HistoryManager.saveState();
                });

                document.getElementById('current-image-name').textContent = imageName;
            });
        });

        // Resize mode buttons
        container.querySelectorAll('.resize-mode-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                container.querySelectorAll('.resize-mode-btn').forEach(function(b) { b.classList.remove('active'); });
                btn.classList.add('active');
                var mode = btn.dataset.mode;
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                data.properties.resizeMode = mode;
                obj.set('elementData', data);
                obj.set('lockUniScaling', mode === 'proportional');
                CanvasManager.getCanvas().renderAll();
                HistoryManager.saveState();
            });
        });

        // Crop button
        var cropBtn = document.getElementById('open-crop-btn');
        if (cropBtn) {
            cropBtn.addEventListener('click', function() {
                var data = obj.get('elementData') || {};
                var p = data.properties || {};
                document.getElementById('crop-x').value = p.cropX || 0;
                document.getElementById('crop-y').value = p.cropY || 0;
                document.getElementById('crop-w').value = p.cropW || 0;
                document.getElementById('crop-h').value = p.cropH || 0;
                document.getElementById('crop-modal').style.display = 'flex';

                var applyBtn = document.getElementById('crop-apply-btn');
                var resetBtn = document.getElementById('crop-reset-btn');
                var cancelBtn = document.getElementById('crop-cancel-btn');

                var applyHandler = function() {
                    var d = obj.get('elementData') || {};
                    d.properties = d.properties || {};
                    var cx = parseInt(document.getElementById('crop-x').value) || 0;
                    var cy = parseInt(document.getElementById('crop-y').value) || 0;
                    var cw = parseInt(document.getElementById('crop-w').value) || 0;
                    var ch = parseInt(document.getElementById('crop-h').value) || 0;
                    if (cw > 0 && ch > 0) {
                        d.properties.cropX = cx;
                        d.properties.cropY = cy;
                        d.properties.cropW = cw;
                        d.properties.cropH = ch;
                    } else {
                        delete d.properties.cropX;
                        delete d.properties.cropY;
                        delete d.properties.cropW;
                        delete d.properties.cropH;
                    }
                    obj.set('elementData', d);
                    document.getElementById('crop-modal').style.display = 'none';
                    HistoryManager.saveState();
                    cleanup();
                };
                var resetHandler = function() {
                    document.getElementById('crop-x').value = 0;
                    document.getElementById('crop-y').value = 0;
                    document.getElementById('crop-w').value = 0;
                    document.getElementById('crop-h').value = 0;
                };
                var cancelHandler = function() {
                    document.getElementById('crop-modal').style.display = 'none';
                    cleanup();
                };
                var cleanup = function() {
                    applyBtn.removeEventListener('click', applyHandler);
                    resetBtn.removeEventListener('click', resetHandler);
                    cancelBtn.removeEventListener('click', cancelHandler);
                };
                applyBtn.addEventListener('click', applyHandler);
                resetBtn.addEventListener('click', resetHandler);
                cancelBtn.addEventListener('click', cancelHandler);
            });
        }
    },

    renderShapeProperties(container, obj, props) {
        var self = this;
        container.innerHTML = '' +
            '<div class="property-group">' +
            '    <label>Fill Color</label>' +
            '    <div class="color-swatches" id="shape-fill-swatches"></div>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Stroke Color</label>' +
            '    <div class="color-swatches" id="shape-stroke-swatches"></div>' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Stroke Width</label>' +
            '    <input type="number" id="prop-stroke-width" class="prop-input" value="' + (obj.strokeWidth || 1) + '" min="0" max="20">' +
            '</div>' +
            '<div class="property-group">' +
            '    <label>Border Radius</label>' +
            '    <input type="number" id="prop-border-radius" class="prop-input" value="' + (obj.rx || 0) + '" min="0" max="100">' +
            '</div>';

        this.renderColorSwatches('shape-fill-swatches', obj.fill, function(color) {
            obj.set('fill', color);
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        this.renderColorSwatches('shape-stroke-swatches', obj.stroke, function(color) {
            obj.set('stroke', color);
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        document.getElementById('prop-stroke-width').addEventListener('change', function(e) {
            obj.set('strokeWidth', parseInt(e.target.value));
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });

        document.getElementById('prop-border-radius').addEventListener('change', function(e) {
            var r = parseInt(e.target.value);
            obj.set({ rx: r, ry: r });
            CanvasManager.getCanvas().renderAll();
            HistoryManager.saveState();
        });
    },

    renderWidgetProperties(container, type, obj, props) {
        var self = this;
        var widgetType = type.replace('widget_', '');
        var html = '<div class="property-group"><label>Widget: ' + widgetType + '</label></div>';

        // Layout selector
        var currentLayout = props.layout || WidgetPreview.getDefaultLayout(type);
        html += '<div class="property-group"><label>Layout</label>' +
            '<select class="prop-input" id="widget-layout-select"><option value="' + currentLayout + '">' + currentLayout + '</option></select></div>';

        // Custom template (shown when layout is "custom")
        html += '<div class="property-group" id="custom-template-section" style="display:' + (currentLayout === 'custom' ? 'block' : 'none') + ';">' +
            '<label>Custom Template</label>' +
            '<textarea id="custom-template-input" class="prop-input" rows="3" style="resize:vertical;font-family:monospace;font-size:12px;">' + (props.customTemplate || '') + '</textarea>' +
            '<div id="placeholder-list" class="placeholder-chips" style="margin-top:4px;display:flex;flex-wrap:wrap;gap:4px;"></div>' +
            '</div>';

        // Widget-specific properties
        var propDefs = this.getWidgetPropertyDefs(type);
        var keys = Object.keys(propDefs);
        for (var i = 0; i < keys.length; i++) {
            var key = keys[i];
            var def = propDefs[key];
            var val = props[key] !== undefined ? props[key] : def.default;

            if (def.type === 'text') {
                html += '<div class="property-group"><label>' + def.label + '</label>' +
                    '<input type="text" class="prop-input widget-prop" data-key="' + key + '" value="' + (val || '') + '"></div>';
            } else if (def.type === 'number') {
                html += '<div class="property-group"><label>' + def.label + '</label>' +
                    '<input type="number" class="prop-input widget-prop" data-key="' + key + '" value="' + (val || 0) + '" min="' + (def.min || 0) + '" max="' + (def.max || 999) + '"></div>';
            } else if (def.type === 'select') {
                var options = '';
                for (var j = 0; j < def.options.length; j++) {
                    var o = def.options[j];
                    options += '<option value="' + o + '"' + (val === o ? ' selected' : '') + '>' + o + '</option>';
                }
                html += '<div class="property-group"><label>' + def.label + '</label>' +
                    '<select class="prop-input widget-prop" data-key="' + key + '">' + options + '</select></div>';
            } else if (def.type === 'checkbox') {
                html += '<div class="property-group"><label class="checkbox-label">' +
                    '<input type="checkbox" class="widget-prop" data-key="' + key + '"' + (val ? ' checked' : '') + '> ' + def.label + '</label></div>';
            }
        }

        // Color for widgets
        html += '<div class="property-group"><label>Color</label><div class="color-swatches" id="widget-color-swatches"></div></div>';

        // Text alignment for widgets
        var currentAlign = props.textAlign || 'center';
        html += '<div class="property-group"><label>Align</label><div class="button-group">' +
            '<button class="align-btn widget-align' + (currentAlign === 'left' ? ' active' : '') + '" data-align="left">L</button>' +
            '<button class="align-btn widget-align' + (currentAlign === 'center' ? ' active' : '') + '" data-align="center">C</button>' +
            '<button class="align-btn widget-align' + (currentAlign === 'right' ? ' active' : '') + '" data-align="right">R</button>' +
            '</div></div>';

        container.innerHTML = html;

        // Load layouts from server and populate dropdown
        this.loadAndPopulateLayouts(type, currentLayout, obj);

        // Setup custom template input
        var templateInput = document.getElementById('custom-template-input');
        if (templateInput) {
            templateInput.addEventListener('input', function() {
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                data.properties.customTemplate = templateInput.value;
                obj.set('elementData', data);
                WidgetPreview.updatePreview(obj);
                HistoryManager.saveState();
            });
        }

        // Color swatches for widget
        this.renderColorSwatches('widget-color-swatches', props.color || '#000000', function(color) {
            var data = obj.get('elementData') || {};
            data.properties = data.properties || {};
            data.properties.color = color;
            obj.set('elementData', data);
            WidgetPreview.updatePreview(obj);
            HistoryManager.saveState();
        });

        // Widget alignment buttons
        container.querySelectorAll('.widget-align').forEach(function(btn) {
            btn.addEventListener('click', function() {
                container.querySelectorAll('.widget-align').forEach(function(b) { b.classList.remove('active'); });
                btn.classList.add('active');
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                data.properties.textAlign = btn.dataset.align;
                obj.set('elementData', data);
                WidgetPreview.updatePreview(obj);
                HistoryManager.saveState();
            });
        });

        // Listen for widget property changes
        var urlProps = ['icalUrl', 'feedUrl', 'url'];
        container.querySelectorAll('.widget-prop').forEach(function(input) {
            input.addEventListener('change', function() {
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                var propKey = input.dataset.key;
                if (input.type === 'checkbox') {
                    data.properties[propKey] = input.checked;
                } else if (input.type === 'number') {
                    data.properties[propKey] = parseFloat(input.value);
                } else {
                    data.properties[propKey] = input.value;
                }
                obj.set('elementData', data);
                // Invalidate cache when URL properties change
                if (urlProps.indexOf(propKey) >= 0) {
                    WidgetPreview.invalidateCache(type);
                }
                WidgetPreview.updatePreview(obj);
                HistoryManager.saveState();
            });
        });
    },

    async loadAndPopulateLayouts(type, currentLayout, obj) {
        var select = document.getElementById('widget-layout-select');
        if (!select) return;

        // Try cache first
        if (this._layoutCache[type]) {
            this.populateLayoutSelect(select, this._layoutCache[type], currentLayout, obj, type);
            return;
        }

        try {
            var res = await fetch('/api/widget_layouts/' + type);
            if (res.ok) {
                var data = await res.json();
                this._layoutCache[type] = data;
                this.populateLayoutSelect(select, data, currentLayout, obj, type);
            }
        } catch (e) {
            console.warn('Failed to load layouts for', type);
        }
    },

    populateLayoutSelect(select, data, currentLayout, obj, type) {
        select.innerHTML = '';
        var layouts = data.layouts || [];
        for (var i = 0; i < layouts.length; i++) {
            var opt = document.createElement('option');
            opt.value = layouts[i].id;
            opt.textContent = layouts[i].name;
            if (layouts[i].id === currentLayout) opt.selected = true;
            select.appendChild(opt);
        }

        var self = this;
        select.addEventListener('change', function() {
            var newLayout = select.value;
            var d = obj.get('elementData') || {};
            d.properties = d.properties || {};
            d.properties.layout = newLayout;
            obj.set('elementData', d);

            // Show/hide custom template
            var customSection = document.getElementById('custom-template-section');
            if (customSection) {
                customSection.style.display = newLayout === 'custom' ? 'block' : 'none';
            }

            // Populate placeholders if custom
            if (newLayout === 'custom' && data.placeholders) {
                self.populatePlaceholders(data.placeholders, type);
            }

            WidgetPreview.updatePreview(obj);
            HistoryManager.saveState();
        });

        // If currently custom, populate placeholders
        if (currentLayout === 'custom' && data.placeholders) {
            this.populatePlaceholders(data.placeholders, type);
        }
    },

    populatePlaceholders(placeholders, type) {
        var container = document.getElementById('placeholder-list');
        if (!container) return;
        container.innerHTML = '';
        for (var i = 0; i < placeholders.length; i++) {
            var chip = document.createElement('span');
            chip.textContent = placeholders[i];
            chip.style.cssText = 'background:var(--bg-tertiary,#333);color:var(--accent,#89b4fa);padding:2px 8px;border-radius:4px;font-size:11px;font-family:monospace;cursor:pointer;';
            chip.addEventListener('click', (function(p) {
                return function() {
                    var textarea = document.getElementById('custom-template-input');
                    if (!textarea) return;
                    var pos = textarea.selectionStart;
                    var text = textarea.value;
                    textarea.value = text.slice(0, pos) + p + text.slice(pos);
                    textarea.focus();
                    textarea.dispatchEvent(new Event('input'));
                };
            })(placeholders[i]));
            container.appendChild(chip);
        }
    },

    getWidgetPropertyDefs(type) {
        var defs = {
            widget_clock: {
                timezone: { label: 'Timezone', type: 'text', default: 'Europe/Berlin' },
                fontSize: { label: 'Font Size', type: 'number', default: 48, min: 8, max: 200 },
            },
            widget_weather: {
                latitude: { label: 'Latitude', type: 'number', default: 52.3759, min: -90, max: 90 },
                longitude: { label: 'Longitude', type: 'number', default: 9.7320, min: -180, max: 180 },
            },
            widget_forecast: {
                latitude: { label: 'Latitude', type: 'number', default: 52.3759, min: -90, max: 90 },
                longitude: { label: 'Longitude', type: 'number', default: 9.7320, min: -180, max: 180 },
                days: { label: 'Days', type: 'number', default: 3, min: 1, max: 7 },
            },
            widget_calendar: {
                icalUrl: { label: 'iCal URL', type: 'text', default: '' },
                maxEvents: { label: 'Max Events', type: 'number', default: 5, min: 1, max: 20 },
                showTime: { label: 'Show Time', type: 'checkbox', default: true },
                daysAhead: { label: 'Days Ahead', type: 'number', default: 7, min: 1, max: 30 },
                title: { label: 'Title', type: 'text', default: 'Events' },
            },
            widget_news: {
                feedUrl: { label: 'RSS Feed URL', type: 'text', default: '' },
                maxItems: { label: 'Max Items', type: 'number', default: 3, min: 1, max: 10 },
                showDescription: { label: 'Show Description', type: 'checkbox', default: false },
                title: { label: 'Title', type: 'text', default: 'News' },
            },
            widget_timer: {
                targetDate: { label: 'Target Date', type: 'text', default: '2026-12-25T00:00:00' },
                label: { label: 'Label', type: 'text', default: 'Countdown' },
                finishedText: { label: 'Finished Text', type: 'text', default: "Time's up!" },
            },
            widget_custom: {
                url: { label: 'API URL', type: 'text', default: '' },
                jsonPath: { label: 'JSON Path', type: 'text', default: '' },
                prefix: { label: 'Prefix', type: 'text', default: '' },
                suffix: { label: 'Suffix', type: 'text', default: '' },
                fontSize: { label: 'Font Size', type: 'number', default: 24, min: 8, max: 200 },
            },
            widget_system: {
                showLabels: { label: 'Show Labels', type: 'checkbox', default: true },
            },
        };
        return defs[type] || {};
    },

    renderColorSwatches(containerId, currentColor, onChange) {
        var container = document.getElementById(containerId);
        if (!container) return;
        container.innerHTML = '';

        this.displayColors.forEach(function(color) {
            var swatch = document.createElement('div');
            swatch.className = 'color-swatch' + (color === currentColor ? ' selected' : '');
            swatch.style.backgroundColor = color;
            swatch.style.border = color === '#FFFFFF' ? '2px solid var(--border)' : '2px solid transparent';
            swatch.addEventListener('click', function() {
                container.querySelectorAll('.color-swatch').forEach(function(s) { s.classList.remove('selected'); });
                swatch.classList.add('selected');
                onChange(color);
            });
            container.appendChild(swatch);
        });
    }
};
