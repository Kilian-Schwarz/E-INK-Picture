// Widget preview rendering on canvas
// Provides realistic preview data for each widget type
var WidgetPreview = {
    _clockInterval: null,

    // Get realistic preview text for a widget type
    getPreviewContent(type, props) {
        props = props || {};
        switch (type) {
            case 'widget_clock': {
                var now = new Date();
                var format = props.format || 'HH:mm';
                if (format === 'HH:mm:ss') {
                    return now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', second:'2-digit', hour12:false});
                } else if (format === 'hh:mm A') {
                    return now.toLocaleTimeString('en-US', {hour:'2-digit', minute:'2-digit', hour12:true});
                } else if (format.indexOf('dddd') >= 0) {
                    return now.toLocaleDateString('de-DE', {weekday:'long', year:'numeric', month:'long', day:'numeric'});
                }
                return now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', hour12:false});
            }
            case 'widget_weather':
                return '22\u00B0C Sunny';
            case 'widget_forecast': {
                var days = props.days || 3;
                var lines = [];
                var dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
                var today = new Date().getDay();
                for (var i = 0; i < days; i++) {
                    var d = dayNames[(today + i) % 7];
                    var hi = 18 + Math.floor(Math.random() * 8);
                    var lo = hi - 5 - Math.floor(Math.random() * 5);
                    lines.push(d + ': ' + lo + '-' + hi + '\u00B0C');
                }
                return lines.join('\n');
            }
            case 'widget_calendar':
                return (props.title || 'Events') + '\n10:00 - Meeting\n14:30 - Review';
            case 'widget_news':
                return (props.title || 'News') + '\n- Headline 1\n- Headline 2';
            case 'widget_timer': {
                var label = props.label || 'Countdown';
                return label + ': 42 days';
            }
            case 'widget_custom':
                return props.prefix ? props.prefix + '42' + (props.suffix || '') : 'API: 42';
            case 'widget_system':
                return 'Load: 0.5 0.3 0.2\nMem: 27MB / 512MB\nTemp: 45\u00B0C';
            default:
                return type.replace('widget_', '').toUpperCase();
        }
    },

    // Get appropriate font size for widget type
    getPreviewFontSize(type, props) {
        props = props || {};
        switch (type) {
            case 'widget_clock':
                return Math.min(props.fontSize || 48, 48);
            case 'widget_weather':
                return 20;
            case 'widget_forecast':
                return 13;
            case 'widget_calendar':
                return 13;
            case 'widget_news':
                return 13;
            case 'widget_timer':
                return 18;
            case 'widget_custom':
                return Math.min(props.fontSize || 24, 24);
            case 'widget_system':
                return 12;
            default:
                return 14;
        }
    },

    // Update widget preview when properties change
    updatePreview(fabricObj) {
        if (!fabricObj) return;
        var type = fabricObj.get('elementType');
        if (!type || !type.startsWith('widget_')) return;

        var data = fabricObj.get('elementData') || {};
        var props = data.properties || {};
        var displayText = this.getPreviewContent(type, props);
        var fontSize = this.getPreviewFontSize(type, props);

        if (fabricObj.type === 'group' && fabricObj._objects && fabricObj._objects.length >= 2) {
            var label = fabricObj._objects[1];
            if (label && label.set) {
                label.set('text', displayText);
                label.set('fontSize', fontSize);
                label.set('fill', props.color || '#333333');
                label.set('fontFamily', props.fontFamily || 'monospace');
            }
        }

        CanvasManager.getCanvas().renderAll();
    },

    // Start live clock updates on canvas
    startClockUpdates() {
        if (this._clockInterval) return;
        var self = this;
        this._clockInterval = setInterval(function() {
            var canvas = CanvasManager.getCanvas();
            if (!canvas) return;
            var changed = false;
            canvas.getObjects().forEach(function(obj) {
                if (obj.get('elementType') !== 'widget_clock') return;
                var data = obj.get('elementData') || {};
                var props = data.properties || {};
                var timeStr = self.getPreviewContent('widget_clock', props);

                if (obj.type === 'group' && obj._objects && obj._objects.length >= 2) {
                    var label = obj._objects[1];
                    if (label && label.get('text') !== timeStr) {
                        label.set('text', timeStr);
                        changed = true;
                    }
                }
            });
            if (changed) canvas.renderAll();
        }, 60000);
    }
};
