// Top bar and left panel actions
var Toolbar = {
    init() {
        var self = this;

        // Widget click-to-add
        document.querySelectorAll('.widget-item').forEach(function(item) {
            item.addEventListener('click', function() {
                var type = item.dataset.type;
                self.addElement(type);
            });
        });

        // Layer actions
        var layerUp = document.getElementById('layer-up');
        var layerDown = document.getElementById('layer-down');
        var layerFront = document.getElementById('layer-front');
        var layerBack = document.getElementById('layer-back');
        var duplicateBtn = document.getElementById('duplicate-btn');
        var deleteBtn = document.getElementById('delete-btn');
        var lockBtn = document.getElementById('lock-btn');
        var previewBtn = document.getElementById('preview-btn');
        var settingsBtn = document.getElementById('settings-btn');

        if (layerUp) layerUp.addEventListener('click', function() { self.moveLayer('up'); });
        if (layerDown) layerDown.addEventListener('click', function() { self.moveLayer('down'); });
        if (layerFront) layerFront.addEventListener('click', function() { self.moveLayer('front'); });
        if (layerBack) layerBack.addEventListener('click', function() { self.moveLayer('back'); });
        if (duplicateBtn) duplicateBtn.addEventListener('click', function() { self.duplicateSelected(); });
        if (deleteBtn) deleteBtn.addEventListener('click', function() { self.deleteSelected(); });
        if (lockBtn) lockBtn.addEventListener('click', function() { self.toggleLock(); });

        // Preview
        if (previewBtn) previewBtn.addEventListener('click', function() { self.preview(); });

        // Settings
        if (settingsBtn) settingsBtn.addEventListener('click', function() {
            document.getElementById('settings-modal').style.display = 'flex';
        });
    },

    addElement(type) {
        var canvas = CanvasManager.getCanvas();
        var obj;

        switch (type) {
            case 'text':
                obj = ElementFactory.createText();
                break;
            case 'image':
                obj = ElementFactory.createImage();
                break;
            case 'shape':
                obj = ElementFactory.createShape();
                break;
            default:
                obj = ElementFactory.createWidget(type);
        }

        if (obj) {
            canvas.add(obj);
            canvas.setActiveObject(obj);
            canvas.renderAll();
            HistoryManager.saveState();
            LayersPanel.refresh();
        }
    },

    moveLayer(direction) {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;

        switch (direction) {
            case 'up': canvas.bringForward(obj); break;
            case 'down': canvas.sendBackwards(obj); break;
            case 'front': canvas.bringToFront(obj); break;
            case 'back': canvas.sendToBack(obj); break;
        }
        canvas.renderAll();
        LayersPanel.refresh();
        HistoryManager.saveState();
    },

    deleteSelected() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;
        canvas.remove(obj);
        canvas.discardActiveObject();
        canvas.renderAll();
        LayersPanel.refresh();
        HistoryManager.saveState();
    },

    duplicateSelected() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;

        obj.clone(function(cloned) {
            cloned.set({
                left: obj.left + 20,
                top: obj.top + 20,
            });
            cloned.set('elementId', ElementFactory.generateId());
            cloned.set('elementType', obj.get('elementType'));
            cloned.set('elementData', JSON.parse(JSON.stringify(obj.get('elementData') || {})));
            canvas.add(cloned);
            canvas.setActiveObject(cloned);
            canvas.renderAll();
            LayersPanel.refresh();
            HistoryManager.saveState();
        });
    },

    toggleLock() {
        var canvas = CanvasManager.getCanvas();
        var obj = canvas.getActiveObject();
        if (!obj) return;
        var locked = !obj.lockMovementX;
        obj.set({
            lockMovementX: locked,
            lockMovementY: locked,
            lockScalingX: locked,
            lockScalingY: locked,
            lockRotation: locked,
            hasControls: !locked,
        });
        canvas.renderAll();
        LayersPanel.refresh();
    },

    preview() {
        var designName = document.getElementById('design-select') ? document.getElementById('design-select').value : '';
        var url = designName ? '/preview?name=' + encodeURIComponent(designName) : '/preview';
        document.getElementById('preview-image').src = url + (url.indexOf('?') >= 0 ? '&' : '?') + 't=' + Date.now();
        document.getElementById('preview-modal').style.display = 'flex';
    }
};
