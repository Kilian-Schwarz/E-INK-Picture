// Right panel - Properties editor
// Shows context-dependent properties based on selected element type
var PropertiesPanel = {
    currentObject: null,
    displayColors: ['#000000', '#FFFFFF'],

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
        container.innerHTML = '' +
            '<div class="property-group">' +
            '    <label>Image</label>' +
            '    <button id="select-image-btn" class="btn-secondary">Select Image</button>' +
            '    <span id="current-image-name" class="text-muted" style="display:block;margin-top:4px;font-size:0.85em;">' + (props.image || 'None') + '</span>' +
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
                    canvas.add(img);
                    canvas.setActiveObject(img);
                    self.currentObject = img;
                    canvas.renderAll();
                    HistoryManager.saveState();
                });

                document.getElementById('current-image-name').textContent = imageName;
            });
        });
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
        var widgetType = type.replace('widget_', '');
        var html = '<div class="property-group"><label>Widget: ' + widgetType + '</label></div>';

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

        container.innerHTML = html;

        // Listen for changes
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
                WidgetPreview.updatePreview(obj);
                HistoryManager.saveState();
            });
        });
    },

    getWidgetPropertyDefs(type) {
        var defs = {
            widget_clock: {
                format: { label: 'Format', type: 'select', options: ['HH:mm', 'hh:mm A', 'HH:mm:ss', 'dddd, DD. MMMM YYYY'], default: 'HH:mm' },
                timezone: { label: 'Timezone', type: 'text', default: 'Europe/Berlin' },
                fontSize: { label: 'Font Size', type: 'number', default: 48, min: 8, max: 200 },
            },
            widget_weather: {
                latitude: { label: 'Latitude', type: 'number', default: 52.3759, min: -90, max: 90 },
                longitude: { label: 'Longitude', type: 'number', default: 9.7320, min: -180, max: 180 },
                style: { label: 'Style', type: 'select', options: ['compact', 'detailed', 'minimal', 'icon_only'], default: 'compact' },
                showTemperature: { label: 'Show Temperature', type: 'checkbox', default: true },
                showCondition: { label: 'Show Condition', type: 'checkbox', default: true },
            },
            widget_forecast: {
                latitude: { label: 'Latitude', type: 'number', default: 52.3759, min: -90, max: 90 },
                longitude: { label: 'Longitude', type: 'number', default: 9.7320, min: -180, max: 180 },
                days: { label: 'Days', type: 'number', default: 3, min: 1, max: 7 },
                layout: { label: 'Layout', type: 'select', options: ['horizontal', 'vertical', 'grid'], default: 'horizontal' },
                showHighLow: { label: 'Show High/Low', type: 'checkbox', default: true },
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
                showDescription: { label: 'Show Description', type: 'checkbox', default: true },
                layout: { label: 'Layout', type: 'select', options: ['list', 'headline_only', 'cards'], default: 'list' },
                title: { label: 'Title', type: 'text', default: 'News' },
            },
            widget_timer: {
                targetDate: { label: 'Target Date', type: 'text', default: '2026-12-25T00:00:00' },
                label: { label: 'Label', type: 'text', default: 'Countdown' },
                format: { label: 'Format', type: 'select', options: ['days', 'dhm', 'full'], default: 'days' },
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
                layout: { label: 'Layout', type: 'select', options: ['horizontal', 'vertical', 'compact'], default: 'horizontal' },
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
