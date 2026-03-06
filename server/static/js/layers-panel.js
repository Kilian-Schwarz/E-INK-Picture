// Layer management panel
var LayersPanel = {
    refresh() {
        var canvas = CanvasManager.getCanvas();
        if (!canvas) return;
        var container = document.getElementById('layers-panel');
        if (!container) return;

        container.innerHTML = '';
        var objects = canvas.getObjects().slice().reverse();

        // Filter out grid lines
        objects = objects.filter(function(obj) { return !obj.isGridLine; });

        objects.forEach(function(obj, idx) {
            var type = obj.get('elementType') || 'unknown';
            var isSelected = canvas.getActiveObject() === obj;
            var isVisible = obj.visible !== false;
            var isLocked = obj.lockMovementX || false;

            var item = document.createElement('div');
            item.className = 'layer-item' + (isSelected ? ' selected' : '');
            item.innerHTML = '' +
                '<span class="layer-icon">' + LayersPanel.getIcon(type) + '</span>' +
                '<span class="layer-name">' + type + '</span>' +
                '<button class="layer-visibility" title="Toggle Visibility">' + (isVisible ? 'V' : 'H') + '</button>' +
                '<button class="layer-lock ' + (isLocked ? 'locked' : '') + '" title="Toggle Lock">' + (isLocked ? 'L' : 'U') + '</button>';

            item.addEventListener('click', function(e) {
                if (e.target.classList.contains('layer-visibility')) {
                    obj.visible = !obj.visible;
                    canvas.renderAll();
                    LayersPanel.refresh();
                    return;
                }
                if (e.target.classList.contains('layer-lock')) {
                    var locked = !obj.lockMovementX;
                    obj.set({
                        lockMovementX: locked,
                        lockMovementY: locked,
                        lockScalingX: locked,
                        lockScalingY: locked,
                        lockRotation: locked,
                        hasControls: !locked,
                        selectable: !locked,
                    });
                    canvas.renderAll();
                    LayersPanel.refresh();
                    return;
                }
                canvas.setActiveObject(obj);
                canvas.renderAll();
            });

            container.appendChild(item);
        });
    },

    getIcon(type) {
        var icons = {
            text: 'T',
            image: 'Img',
            shape: '[]',
            widget_clock: 'Clk',
            widget_weather: 'W',
            widget_forecast: 'Fc',
            widget_calendar: 'Cal',
            widget_news: 'RSS',
            widget_timer: 'Tmr',
            widget_custom: 'API',
            widget_system: 'Sys',
        };
        return icons[type] || '?';
    }
};
