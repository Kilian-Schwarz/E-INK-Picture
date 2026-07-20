// Right panel - Properties editor
// Shows context-dependent properties based on selected element type
var PropertiesPanel = {
    currentObject: null,
    displayColors: ['#000000', '#FFFFFF'],
    _layoutCache: {},

    // Location search (weather/forecast widgets, B1b). Politeness state lives on
    // the instance so debounce/rate-limit survive panel re-renders.
    _locTimer: null,
    _locBusy: false,
    _locLastAt: 0,
    _locReqSeq: 0,
    _locOutsideBound: false,

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
        var type = obj.get('elementType');
        var x = document.getElementById('prop-x');
        var y = document.getElementById('prop-y');
        var w = document.getElementById('prop-w');
        var h = document.getElementById('prop-h');
        var rot = document.getElementById('prop-rotation');

        if (x) x.value = Math.round(obj.left || 0);
        if (y) y.value = Math.round(obj.top || 0);
        if (w) w.value = Math.round(obj.getScaledWidth ? obj.getScaledWidth() : (obj.width || 0));
        // For text elements, show the clip height (bounding box) not the auto-calculated text height
        if (h) {
            if (type === 'text' && obj.get('_clipH')) {
                h.value = Math.round(obj.get('_clipH'));
            } else {
                h.value = Math.round(obj.getScaledHeight ? obj.getScaledHeight() : (obj.height || 0));
            }
        }
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
                var type = obj.get('elementType');

                if (id === 'prop-x') obj.set('left', parseFloat(el.value));
                if (id === 'prop-y') obj.set('top', parseFloat(el.value));
                if (id === 'prop-w') {
                    var newW = parseFloat(el.value);
                    if (type === 'text') {
                        obj.set('width', newW);
                        if (obj.clipPath) {
                            obj.clipPath.set({ width: newW, left: -newW / 2 });
                        }
                    } else {
                        var scaleX = newW / obj.width;
                        obj.set('scaleX', scaleX);
                    }
                }
                if (id === 'prop-h') {
                    var newH = parseFloat(el.value);
                    if (type === 'text') {
                        obj.set('_clipH', newH);
                        if (obj.clipPath) {
                            obj.clipPath.set({ height: newH, top: -newH / 2 });
                        }
                    } else {
                        var scaleY = newH / obj.height;
                        obj.set('scaleY', scaleY);
                    }
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

        // Crop button — visual crop with draggable selection
        var cropBtn = document.getElementById('open-crop-btn');
        if (cropBtn) {
            cropBtn.addEventListener('click', function() {
                var data = obj.get('elementData') || {};
                var p = data.properties || {};
                var imgName = p.image;
                var cropModal = document.getElementById('crop-modal');
                var previewImg = document.getElementById('crop-preview-img');
                var selection = document.getElementById('crop-selection');

                document.getElementById('crop-x').value = p.cropX || 0;
                document.getElementById('crop-y').value = p.cropY || 0;
                document.getElementById('crop-w').value = p.cropW || 0;
                document.getElementById('crop-h').value = p.cropH || 0;

                // Load image for visual crop
                if (imgName) {
                    previewImg.src = '/image/' + imgName;
                    previewImg.onload = function() {
                        var scale = previewImg.width / previewImg.naturalWidth;
                        var cx = (p.cropX || 0) * scale;
                        var cy = (p.cropY || 0) * scale;
                        var cw = (p.cropW || previewImg.naturalWidth) * scale;
                        var ch = (p.cropH || previewImg.naturalHeight) * scale;
                        selection.style.display = 'block';
                        selection.style.left = cx + 'px';
                        selection.style.top = cy + 'px';
                        selection.style.width = cw + 'px';
                        selection.style.height = ch + 'px';

                        // Draggable crop selection
                        var dragging = false, startX = 0, startY = 0, origL = 0, origT = 0;
                        var drawing = false, drawStartX = 0, drawStartY = 0;
                        var container = document.getElementById('crop-preview-container');

                        var moveHandler = function(ev) {
                            if (dragging) {
                                var dx = ev.clientX - startX;
                                var dy = ev.clientY - startY;
                                var newL = Math.max(0, Math.min(origL + dx, previewImg.width - selection.offsetWidth));
                                var newT = Math.max(0, Math.min(origT + dy, previewImg.height - selection.offsetHeight));
                                selection.style.left = newL + 'px';
                                selection.style.top = newT + 'px';
                                updateCropInputs(scale);
                            }
                            if (drawing) {
                                var rect = container.getBoundingClientRect();
                                var curX = Math.max(0, Math.min(ev.clientX - rect.left, previewImg.width));
                                var curY = Math.max(0, Math.min(ev.clientY - rect.top, previewImg.height));
                                var x1 = Math.min(drawStartX, curX), y1 = Math.min(drawStartY, curY);
                                var x2 = Math.max(drawStartX, curX), y2 = Math.max(drawStartY, curY);
                                selection.style.left = x1 + 'px';
                                selection.style.top = y1 + 'px';
                                selection.style.width = (x2 - x1) + 'px';
                                selection.style.height = (y2 - y1) + 'px';
                                selection.style.display = 'block';
                                updateCropInputs(scale);
                            }
                        };
                        // pointerup and pointercancel (browser aborts the
                        // gesture: notification, edge swipe, palm rejection)
                        // both end the drag cleanly.
                        var upHandler = function() {
                            dragging = false;
                            drawing = false;
                        };
                        var selectionDownHandler = function(ev) {
                            if (!ev.isPrimary) return;
                            ev.stopPropagation();
                            dragging = true;
                            startX = ev.clientX;
                            startY = ev.clientY;
                            origL = selection.offsetLeft;
                            origT = selection.offsetTop;
                            try {
                                selection.setPointerCapture(ev.pointerId);
                            } catch (e) {
                                // Synthetic event or pointer already released
                            }
                        };
                        var containerDownHandler = function(ev) {
                            if (!ev.isPrimary) return;
                            if (ev.target === selection) return;
                            drawing = true;
                            var rect = container.getBoundingClientRect();
                            drawStartX = ev.clientX - rect.left;
                            drawStartY = ev.clientY - rect.top;
                            try {
                                container.setPointerCapture(ev.pointerId);
                            } catch (e) {
                                // Synthetic event or pointer already released
                            }
                        };
                        // move/up/cancel listeners live on the capturing
                        // elements (not document): real events are routed
                        // there via pointer capture, synthetic test events
                        // reach them via direct dispatch.
                        selection.addEventListener('pointerdown', selectionDownHandler);
                        container.addEventListener('pointerdown', containerDownHandler);
                        [selection, container].forEach(function(el) {
                            el.addEventListener('pointermove', moveHandler);
                            el.addEventListener('pointerup', upHandler);
                            el.addEventListener('pointercancel', upHandler);
                        });

                        // Store cleanup for later
                        cropModal._cleanupDrag = function() {
                            selection.removeEventListener('pointerdown', selectionDownHandler);
                            container.removeEventListener('pointerdown', containerDownHandler);
                            [selection, container].forEach(function(el) {
                                el.removeEventListener('pointermove', moveHandler);
                                el.removeEventListener('pointerup', upHandler);
                                el.removeEventListener('pointercancel', upHandler);
                            });
                        };
                    };
                } else {
                    previewImg.src = '';
                    selection.style.display = 'none';
                }

                function updateCropInputs(scale) {
                    document.getElementById('crop-x').value = Math.round(selection.offsetLeft / scale);
                    document.getElementById('crop-y').value = Math.round(selection.offsetTop / scale);
                    document.getElementById('crop-w').value = Math.round(selection.offsetWidth / scale);
                    document.getElementById('crop-h').value = Math.round(selection.offsetHeight / scale);
                }

                cropModal.style.display = 'flex';

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
                    closeCropModal();
                    HistoryManager.saveState();
                };
                var resetHandler = function() {
                    document.getElementById('crop-x').value = 0;
                    document.getElementById('crop-y').value = 0;
                    document.getElementById('crop-w').value = 0;
                    document.getElementById('crop-h').value = 0;
                    selection.style.display = 'none';
                };
                var cancelHandler = function() {
                    closeCropModal();
                };
                var closeCropModal = function() {
                    cropModal.style.display = 'none';
                    if (cropModal._cleanupDrag) cropModal._cleanupDrag();
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

        // Weather + forecast get a primary "Ort oder PLZ" location search on top;
        // their raw lat/lon inputs are demoted into an "Erweitert" section (B1b).
        var isLocationWidget = (type === 'widget_weather' || type === 'widget_forecast');
        if (isLocationWidget) {
            html += this.renderLocationSearchHTML();
            var advBody = '';
            ['latitude', 'longitude'].forEach(function(k) {
                var d = propDefs[k];
                if (!d) return;
                var v = props[k] !== undefined ? props[k] : d.default;
                advBody += self.renderPropDef(k, d, v);
            });
            html += '<details class="location-advanced">' +
                '<summary>Erweitert / manuelle Koordinaten</summary>' +
                '<div class="location-advanced-body">' + advBody + '</div></details>';
        }

        for (var i = 0; i < keys.length; i++) {
            var key = keys[i];
            if (isLocationWidget && (key === 'latitude' || key === 'longitude')) continue;
            var def = propDefs[key];
            var val = props[key] !== undefined ? props[key] : def.default;
            html += this.renderPropDef(key, def, val);
        }

        // Color for widgets
        html += '<div class="property-group"><label>Color</label><div class="color-swatches" id="widget-color-swatches"></div></div>';

        // Text alignment for widgets (horizontal)
        var currentAlign = props.textAlign || 'center';
        html += '<div class="property-group"><label>Horizontal</label><div class="button-group">' +
            '<button class="align-btn widget-align' + (currentAlign === 'left' ? ' active' : '') + '" data-align="left">L</button>' +
            '<button class="align-btn widget-align' + (currentAlign === 'center' ? ' active' : '') + '" data-align="center">C</button>' +
            '<button class="align-btn widget-align' + (currentAlign === 'right' ? ' active' : '') + '" data-align="right">R</button>' +
            '</div></div>';

        // Vertical alignment for widgets
        var currentVAlign = props.verticalAlign || 'top';
        html += '<div class="property-group"><label>Vertical</label><div class="button-group">' +
            '<button class="align-btn widget-valign' + (currentVAlign === 'top' ? ' active' : '') + '" data-valign="top">T</button>' +
            '<button class="align-btn widget-valign' + (currentVAlign === 'middle' ? ' active' : '') + '" data-valign="middle">M</button>' +
            '<button class="align-btn widget-valign' + (currentVAlign === 'bottom' ? ' active' : '') + '" data-valign="bottom">B</button>' +
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
                // Debounced: fires POST /api/widget_content, coalesce keystrokes.
                WidgetPreview.updatePreviewDebounced(obj);
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

        // Widget horizontal alignment buttons
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

        // Widget vertical alignment buttons
        container.querySelectorAll('.widget-valign').forEach(function(btn) {
            btn.addEventListener('click', function() {
                container.querySelectorAll('.widget-valign').forEach(function(b) { b.classList.remove('active'); });
                btn.classList.add('active');
                var data = obj.get('elementData') || {};
                data.properties = data.properties || {};
                data.properties.verticalAlign = btn.dataset.valign;
                obj.set('elementData', data);
                WidgetPreview.updatePreview(obj);
                HistoryManager.saveState();
            });
        });

        // Listen for widget property changes
        var urlProps = ['icalUrl', 'feedUrl', 'url'];
        container.querySelectorAll('.widget-prop').forEach(function(input) {
            input.addEventListener('change', function() {
                var propKey = input.dataset.key;
                var value;
                if (input.type === 'checkbox') {
                    value = input.checked;
                } else if (input.type === 'number') {
                    value = parseFloat(input.value);
                } else {
                    value = input.value;
                }
                self.writeWidgetProp(obj, propKey, value);
                // Invalidate cache when URL properties change
                if (urlProps.indexOf(propKey) >= 0) {
                    WidgetPreview.invalidateCache(type);
                }
                // Debounced: fires POST /api/widget_content, coalesce edits.
                WidgetPreview.updatePreviewDebounced(obj);
                HistoryManager.saveState();
            });
        });

        // Location autocomplete for weather/forecast (B1b) — selection reuses the
        // same writeWidgetProp path as the manual fields above (no fork).
        if (isLocationWidget) {
            this.setupLocationSearch(container, type, obj);
        }
    },

    // Single source of truth for writing a widget property into elementData.
    // Both the manual .widget-prop change handler and the location-search
    // selection go through here so the two never drift apart.
    writeWidgetProp(obj, key, value) {
        var data = obj.get('elementData') || {};
        data.properties = data.properties || {};
        data.properties[key] = value;
        obj.set('elementData', data);
    },

    // Renders a single widget property group to HTML (extracted so the location
    // widgets can pull latitude/longitude into their "Erweitert" section while
    // keeping identical markup for every other prop).
    renderPropDef(key, def, val) {
        if (def.type === 'text') {
            return '<div class="property-group"><label>' + def.label + '</label>' +
                '<input type="text" class="prop-input widget-prop" data-key="' + key + '" value="' + (val || '') + '"></div>';
        } else if (def.type === 'number') {
            return '<div class="property-group"><label>' + def.label + '</label>' +
                '<input type="number" class="prop-input widget-prop" data-key="' + key + '" value="' + (val || 0) + '" min="' + (def.min || 0) + '" max="' + (def.max || 999) + '"></div>';
        } else if (def.type === 'select') {
            // Options are either plain strings (value == label) or
            // {value, label} pairs when the wire value is not presentable
            // (e.g. widget_progress period ids year/month/week/day).
            var options = '';
            for (var j = 0; j < def.options.length; j++) {
                var o = def.options[j];
                var oVal = (o && typeof o === 'object') ? o.value : o;
                var oLabel = (o && typeof o === 'object') ? o.label : o;
                options += '<option value="' + oVal + '"' + (val === oVal ? ' selected' : '') + '>' + oLabel + '</option>';
            }
            return '<div class="property-group"><label>' + def.label + '</label>' +
                '<select class="prop-input widget-prop" data-key="' + key + '">' + options + '</select></div>';
        } else if (def.type === 'checkbox') {
            return '<div class="property-group"><label class="checkbox-label">' +
                '<input type="checkbox" class="widget-prop" data-key="' + key + '"' + (val ? ' checked' : '') + '> ' + def.label + '</label></div>';
        }
        return '';
    },

    // Static markup for the "Ort oder PLZ" combobox. The suggestion list is an
    // in-flow <ul> (not absolutely positioned) so it can never be clipped by the
    // mobile bottom-sheet's overflow:hidden — it scrolls with the panel body.
    renderLocationSearchHTML() {
        return '<div class="property-group location-search-group">' +
            '<label for="location-search-input">Ort oder PLZ</label>' +
            '<input type="text" id="location-search-input" class="prop-input location-search-input" ' +
            'placeholder="Ort oder PLZ suchen&hellip;" inputmode="search" autocomplete="off" ' +
            'autocapitalize="off" spellcheck="false" ' +
            'role="combobox" aria-autocomplete="list" aria-expanded="false" ' +
            'aria-controls="location-search-listbox" aria-activedescendant="">' +
            '<ul id="location-search-listbox" class="location-search-results" role="listbox" ' +
            'aria-label="Standortvorschl&auml;ge" hidden></ul>' +
            '</div>';
    },

    // Coordinates travel as strings trimmed to 4 decimals (same convention the
    // setup wizard uses). GetPropString on the server accepts string or number.
    _trimCoord(value) {
        var n = parseFloat(value);
        return isFinite(n) ? n.toFixed(4) : String(value || '');
    },

    // Primary dropdown line: the concise place name.
    locationPrimary(r) {
        if (r.name) return r.name;
        if (r.display_name) return r.display_name.split(',')[0].trim();
        return r.label || '';
    },

    // Secondary dropdown line: region/country (+ PLZ) for disambiguation
    // (Hannover, Niedersachsen, Deutschland vs. Hanover, ..., USA).
    locationSecondary(r, primary) {
        var seen = {};
        if (primary) seen[primary.toLowerCase()] = true;
        var parts = [];
        function add(v) {
            if (!v) return;
            var k = String(v).toLowerCase();
            if (seen[k]) return;
            seen[k] = true;
            parts.push(v);
        }
        add(r.city);
        add(r.region);
        add(r.country);
        var line = parts.join(', ');
        if (r.type !== 'postcode' && r.postcode) {
            line = line ? (r.postcode + ' · ' + line) : r.postcode;
        }
        if (!line && r.label && r.label !== primary) line = r.label;
        return line;
    },

    // Wires the "Ort oder PLZ" combobox: debounced fetch, client-side politeness
    // (>= 1s spacing, never two in flight), keyboard + touch selection, ARIA and
    // clean empty/no-results/error states.
    setupLocationSearch(container, type, obj) {
        var self = this;
        var input = container.querySelector('#location-search-input');
        var box = container.querySelector('#location-search-listbox');
        if (!input || !box) return;
        var latInput = container.querySelector('.widget-prop[data-key="latitude"]');
        var lonInput = container.querySelector('.widget-prop[data-key="longitude"]');

        var DEBOUNCE_MS = 400;
        var MIN_LEN = 2;
        var results = [];
        var activeIndex = -1;

        // Prefill with the currently resolved location (informational — shows the
        // user what the widget is set to without opening "Erweitert").
        var props = (obj.get('elementData') || {}).properties || {};
        if (props.locationName) input.value = props.locationName;

        function closeList() {
            box.hidden = true;
            box.textContent = '';
            results = [];
            activeIndex = -1;
            input.setAttribute('aria-expanded', 'false');
            input.removeAttribute('aria-activedescendant');
        }

        function showStatus(text) {
            box.textContent = '';
            var li = document.createElement('li');
            li.className = 'location-search-status';
            li.setAttribute('role', 'presentation');
            li.textContent = text;
            box.appendChild(li);
            box.hidden = false;
            results = [];
            activeIndex = -1;
            input.setAttribute('aria-expanded', 'true');
            input.removeAttribute('aria-activedescendant');
        }

        function setActive(idx) {
            var items = box.querySelectorAll('.location-search-result');
            if (!items.length) return;
            if (idx < 0) idx = items.length - 1;
            if (idx >= items.length) idx = 0;
            for (var i = 0; i < items.length; i++) {
                var on = (i === idx);
                items[i].classList.toggle('is-active', on);
                items[i].setAttribute('aria-selected', on ? 'true' : 'false');
            }
            activeIndex = idx;
            var active = items[idx];
            input.setAttribute('aria-activedescendant', active.id);
            var top = active.offsetTop;
            var bottom = top + active.offsetHeight;
            if (top < box.scrollTop) box.scrollTop = top;
            else if (bottom > box.scrollTop + box.clientHeight) box.scrollTop = bottom - box.clientHeight;
        }

        function renderResults(list) {
            results = list;
            box.textContent = '';
            activeIndex = -1;
            if (!list.length) {
                showStatus('Keine Treffer');
                return;
            }
            list.forEach(function(r, i) {
                var li = document.createElement('li');
                li.className = 'location-search-result';
                li.id = 'location-search-opt-' + i;
                li.setAttribute('role', 'option');
                li.setAttribute('aria-selected', 'false');
                var primary = self.locationPrimary(r);
                var pEl = document.createElement('div');
                pEl.className = 'location-result-primary';
                pEl.textContent = primary; // textContent: Nominatim free text, never innerHTML
                li.appendChild(pEl);
                var secondary = self.locationSecondary(r, primary);
                if (secondary) {
                    var sEl = document.createElement('div');
                    sEl.className = 'location-result-secondary';
                    sEl.textContent = secondary; // textContent
                    li.appendChild(sEl);
                }
                li.addEventListener('click', function() { selectResult(r); });
                li.addEventListener('pointermove', function() {
                    if (activeIndex !== i) setActive(i);
                });
                box.appendChild(li);
            });
            box.hidden = false;
            input.setAttribute('aria-expanded', 'true');
            input.removeAttribute('aria-activedescendant');
        }

        function selectResult(r) {
            var lat = self._trimCoord(r.lat);
            var lon = self._trimCoord(r.lon);
            var locName = (r.label && String(r.label).trim()) || r.display_name || self.locationPrimary(r) || '';
            // Same write path as the manual fields — one source, then update once.
            self.writeWidgetProp(obj, 'latitude', lat);
            self.writeWidgetProp(obj, 'longitude', lon);
            self.writeWidgetProp(obj, 'locationName', locName);
            if (latInput) latInput.value = lat;
            if (lonInput) lonInput.value = lon;
            input.value = locName;
            closeList();
            WidgetPreview.updatePreview(obj); // discrete action -> instant re-render
            HistoryManager.saveState();       // single undo step
        }

        async function doFetch(query) {
            var reqId = self._locReqSeq;
            self._locBusy = true;
            self._locLastAt = Date.now();
            showStatus('Suche…');
            try {
                var resp = await fetch('/location_search?q=' + encodeURIComponent(query));
                if (reqId !== self._locReqSeq || !input.isConnected) return;
                // Expired session: auth.js already navigates to /login. Clear the
                // spinner so nothing broken is left behind, then bail.
                if (resp.status === 401) { closeList(); return; }
                if (!resp.ok) throw new Error('location search failed: ' + resp.status);
                var data = await resp.json();
                if (reqId !== self._locReqSeq || !input.isConnected) return;
                renderResults(Array.isArray(data) ? data : []);
            } catch (e) {
                if (reqId === self._locReqSeq && input.isConnected) {
                    showStatus('Standortsuche gerade nicht verfügbar');
                }
            } finally {
                self._locBusy = false;
            }
        }

        // Enforce >= 1 req/s and never two in flight; reschedule instead of
        // dropping so the last keystroke is always searched.
        function maybeSearch(query) {
            if (!input.isConnected) return;
            if (input.value.trim() !== query) return; // superseded
            var wait = 1000 - (Date.now() - (self._locLastAt || 0));
            if (self._locBusy || wait > 0) {
                clearTimeout(self._locTimer);
                self._locTimer = setTimeout(function() { maybeSearch(query); }, self._locBusy ? 300 : wait);
                return;
            }
            doFetch(query);
        }

        input.addEventListener('input', function() {
            var query = input.value.trim();
            clearTimeout(self._locTimer);
            self._locReqSeq++; // invalidate any in-flight/pending render
            if (query.length < MIN_LEN) { closeList(); return; }
            self._locTimer = setTimeout(function() { maybeSearch(query); }, DEBOUNCE_MS);
        });

        input.addEventListener('keydown', function(e) {
            if (e.key === 'ArrowDown') {
                if (box.hidden || !box.querySelector('.location-search-result')) return;
                e.preventDefault();
                setActive(activeIndex + 1);
            } else if (e.key === 'ArrowUp') {
                if (box.hidden || !box.querySelector('.location-search-result')) return;
                e.preventDefault();
                setActive(activeIndex - 1);
            } else if (e.key === 'Enter') {
                if (!box.hidden && activeIndex >= 0 && results[activeIndex]) {
                    e.preventDefault();
                    selectResult(results[activeIndex]);
                }
            } else if (e.key === 'Escape') {
                if (!box.hidden) { e.preventDefault(); closeList(); }
            }
        });

        // Close on interaction outside the search group. Bound once and id-based
        // so it keeps working across panel re-renders without stacking listeners.
        if (!self._locOutsideBound) {
            self._locOutsideBound = true;
            document.addEventListener('pointerdown', function(e) {
                var b = document.getElementById('location-search-listbox');
                if (!b || b.hidden) return;
                if (e.target.closest && e.target.closest('.location-search-group')) return;
                var inp = document.getElementById('location-search-input');
                b.hidden = true;
                b.textContent = '';
                if (inp) {
                    inp.setAttribute('aria-expanded', 'false');
                    inp.removeAttribute('aria-activedescendant');
                }
            });
        }
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
                fontSize: { label: 'Font Size', type: 'number', default: 18, min: 8, max: 200 },
            },
            widget_forecast: {
                latitude: { label: 'Latitude', type: 'number', default: 52.3759, min: -90, max: 90 },
                longitude: { label: 'Longitude', type: 'number', default: 9.7320, min: -180, max: 180 },
                days: { label: 'Days', type: 'number', default: 3, min: 1, max: 7 },
                fontSize: { label: 'Font Size', type: 'number', default: 13, min: 8, max: 200 },
            },
            widget_calendar: {
                icalUrl: { label: 'iCal URL', type: 'text', default: '' },
                maxEvents: { label: 'Max Events', type: 'number', default: 5, min: 1, max: 20 },
                showTime: { label: 'Show Time', type: 'checkbox', default: true },
                daysAhead: { label: 'Days Ahead', type: 'number', default: 7, min: 1, max: 30 },
                title: { label: 'Title', type: 'text', default: 'Events' },
                fontSize: { label: 'Font Size', type: 'number', default: 13, min: 8, max: 200 },
            },
            widget_news: {
                feedUrl: { label: 'RSS Feed URL', type: 'text', default: '' },
                maxItems: { label: 'Max Items', type: 'number', default: 3, min: 1, max: 10 },
                showDescription: { label: 'Show Description', type: 'checkbox', default: false },
                title: { label: 'Title', type: 'text', default: 'News' },
                fontSize: { label: 'Font Size', type: 'number', default: 13, min: 8, max: 200 },
            },
            widget_timer: {
                targetDate: { label: 'Target Date', type: 'text', default: '2026-12-25T00:00:00' },
                label: { label: 'Label', type: 'text', default: 'Countdown' },
                finishedText: { label: 'Finished Text', type: 'text', default: "Time's up!" },
                fontSize: { label: 'Font Size', type: 'number', default: 24, min: 8, max: 200 },
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
                fontSize: { label: 'Font Size', type: 'number', default: 12, min: 8, max: 200 },
            },
            // Home Assistant widget: NO token / NO base-URL field. The HA
            // connection secret is admin config (Settings > Home Assistant),
            // stored server-side and never exposed as a widget property.
            widget_hass: {
                hassMode: { label: 'Mode', type: 'select', options: ['temperature', 'alarm', 'presence'], default: 'temperature' },
                entityId: { label: 'Entity ID(s)', type: 'text', default: '' },
                label: { label: 'Label', type: 'text', default: '' },
                fontSize: { label: 'Font Size', type: 'number', default: 18, min: 8, max: 200 },
            },
            // F7 progress widget. Defaults and the barWidth range mirror
            // widget_progress.go (progressMinBarWidth/progressMaxBarWidth);
            // the server clamps anyway, min/max here just stops the spinner
            // from offering values that would be silently rewritten.
            widget_progress: {
                period: {
                    label: 'Period', type: 'select', default: 'year',
                    options: [
                        { value: 'year', label: 'Year' },
                        { value: 'month', label: 'Month' },
                        { value: 'week', label: 'Week' },
                        { value: 'day', label: 'Day' },
                    ],
                },
                barWidth: { label: 'Bar Width', type: 'number', default: 20, min: 5, max: 60 },
                timezone: { label: 'Timezone (empty = server)', type: 'text', default: '' },
                fontSize: { label: 'Font Size', type: 'number', default: 18, min: 8, max: 200 },
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
