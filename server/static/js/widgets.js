// Widget preview rendering on canvas
// Provides preview placeholder data for each widget type
var WidgetPreview = {
    // Update widget preview when properties change
    updatePreview(fabricObj) {
        if (!fabricObj) return;
        var type = fabricObj.get('elementType');
        if (!type || !type.startsWith('widget_')) return;

        var data = fabricObj.get('elementData') || {};
        var props = data.properties || {};

        // For group objects, update the label text
        if (fabricObj.type === 'group' && fabricObj._objects && fabricObj._objects.length >= 2) {
            var label = fabricObj._objects[1];
            var displayText = type.replace('widget_', '').toUpperCase();

            switch (type) {
                case 'widget_clock':
                    displayText = 'CLOCK: ' + (props.format || 'HH:mm');
                    break;
                case 'widget_weather':
                    displayText = 'WEATHER';
                    break;
                case 'widget_forecast':
                    displayText = 'FORECAST (' + (props.days || 3) + 'd)';
                    break;
                case 'widget_calendar':
                    displayText = props.title || 'CALENDAR';
                    break;
                case 'widget_news':
                    displayText = props.title || 'NEWS';
                    break;
                case 'widget_timer':
                    displayText = props.label || 'TIMER';
                    break;
                case 'widget_custom':
                    displayText = 'API: ' + (props.url ? props.url.substring(0, 20) : 'none');
                    break;
                case 'widget_system':
                    displayText = 'SYSTEM';
                    break;
            }

            if (label && label.set) {
                label.set('text', displayText);
            }
        }

        CanvasManager.getCanvas().renderAll();
    }
};
