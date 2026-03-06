// Widget preview rendering on canvas
// Provides live data and layout-based rendering for each widget type
var WidgetPreview = {
    _clockInterval: null,
    _dataRefreshInterval: null,
    _dataCache: {},
    _CACHE_TTL: 60000,

    // Fetch live widget data from server API
    async fetchWidgetData(type, props) {
        props = props || {};
        var cacheKey = type + JSON.stringify(props);
        var cached = this._dataCache[cacheKey];
        if (cached && Date.now() - cached.time < this._CACHE_TTL) {
            return cached.data;
        }

        var url;
        switch (type) {
            case 'widget_weather':
                url = '/api/widgets/weather?lat=' + (props.latitude || '52.52') + '&lon=' + (props.longitude || '13.41');
                break;
            case 'widget_forecast':
                url = '/api/widgets/forecast?lat=' + (props.latitude || '52.52') + '&lon=' + (props.longitude || '13.41') + '&days=' + (props.days || 3);
                break;
            case 'widget_calendar':
                if (!props.icalUrl) return null;
                url = '/api/widgets/calendar?url=' + encodeURIComponent(props.icalUrl) + '&days=' + (props.daysAhead || 7);
                break;
            case 'widget_news':
                if (!props.feedUrl) return null;
                url = '/api/widgets/news?url=' + encodeURIComponent(props.feedUrl) + '&max=' + (props.maxItems || 5);
                break;
            case 'widget_system':
                url = '/api/widgets/system';
                break;
            case 'widget_clock':
                return { time: new Date() };
            case 'widget_timer':
                return { targetDate: props.targetDate };
            default:
                return null;
        }

        try {
            var res = await fetch(url);
            if (!res.ok) return null;
            var data = await res.json();
            this._dataCache[cacheKey] = { data: data, time: Date.now() };
            return data;
        } catch (e) {
            console.warn('Widget data fetch failed:', type, e);
            return null;
        }
    },

    // Get preview content based on layout and live data
    getPreviewContent(type, props, liveData) {
        props = props || {};
        var layout = props.layout || this.getDefaultLayout(type);

        switch (type) {
            case 'widget_clock':
                return this.buildClockContent(layout, props);
            case 'widget_weather':
                return this.buildWeatherContent(layout, props, liveData);
            case 'widget_forecast':
                return this.buildForecastContent(layout, props, liveData);
            case 'widget_calendar':
                return this.buildCalendarContent(layout, props, liveData);
            case 'widget_news':
                return this.buildNewsContent(layout, props, liveData);
            case 'widget_timer':
                return this.buildTimerContent(layout, props);
            case 'widget_system':
                return this.buildSystemContent(layout, props, liveData);
            case 'widget_custom':
                return props.prefix ? props.prefix + '42' + (props.suffix || '') : 'API: 42';
            default:
                return type.replace('widget_', '').toUpperCase();
        }
    },

    getDefaultLayout(type) {
        var defaults = {
            widget_clock: 'digital_large',
            widget_weather: 'compact',
            widget_forecast: 'vertical',
            widget_calendar: 'list',
            widget_news: 'headlines',
            widget_system: 'vertical',
            widget_timer: 'countdown_large',
        };
        return defaults[type] || 'default';
    },

    // --- Clock ---
    buildClockContent(layout, props) {
        var now = new Date();
        if (layout === 'custom') {
            var template = props.customTemplate || '%HH%:%MM%';
            return this.applyTimePlaceholders(template, now);
        }
        switch (layout) {
            case 'digital_with_seconds':
                return now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', second:'2-digit', hour12:false});
            case 'digital_with_date': {
                var time = now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', hour12:false});
                var date = now.toLocaleDateString('de-DE', {weekday:'long', day:'numeric', month:'long', year:'numeric'});
                return time + '\n' + date;
            }
            case 'date_only':
                return now.toLocaleDateString('de-DE', {day:'2-digit', month:'2-digit', year:'numeric'});
            case 'full': {
                var fullDate = now.toLocaleDateString('de-DE', {weekday:'long', day:'numeric', month:'long', year:'numeric'});
                var fullTime = now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', hour12:false});
                return fullDate + ' \u2014 ' + fullTime + ' Uhr';
            }
            default: // digital_large
                // Legacy format support
                var format = props.format || '';
                if (format === 'HH:mm:ss') {
                    return now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', second:'2-digit', hour12:false});
                } else if (format === 'hh:mm A') {
                    return now.toLocaleTimeString('en-US', {hour:'2-digit', minute:'2-digit', hour12:true});
                } else if (format && format.indexOf('dddd') >= 0) {
                    return now.toLocaleDateString('de-DE', {weekday:'long', year:'numeric', month:'long', day:'numeric'});
                }
                return now.toLocaleTimeString('de-DE', {hour:'2-digit', minute:'2-digit', hour12:false});
        }
    },

    // --- Weather ---
    buildWeatherContent(layout, props, data) {
        if (!data || (!data.CurrentTemp && data.CurrentTemp !== 0)) {
            // Fallback to placeholder
            return '22\u00B0C Sunny';
        }
        var temp = Math.round(data.CurrentTemp);
        var desc = data.CurrentDesc || '';
        var icon = data.CurrentIcon || '';
        var tempMin = '--', tempMax = '--';
        if (data.Daily && data.Daily.length > 0) {
            tempMin = Math.round(data.Daily[0].Min);
            tempMax = Math.round(data.Daily[0].Max);
        }
        if (layout === 'custom') {
            var template = props.customTemplate || '%temperature%\u00B0C %description%';
            return template
                .replace(/%temperature%/g, temp)
                .replace(/%description%/g, desc)
                .replace(/%icon%/g, icon)
                .replace(/%temp_min%/g, tempMin)
                .replace(/%temp_max%/g, tempMax)
                .replace(/%feels_like%/g, temp)
                .replace(/%humidity%/g, '--')
                .replace(/%wind_speed%/g, '--');
        }
        switch (layout) {
            case 'standard':
                return temp + '\u00B0C\n' + desc;
            case 'detailed':
                return temp + '\u00B0C ' + desc + '\nHumidity: --%\nWind: -- km/h';
            case 'minimal':
                return temp + '\u00B0';
            default: // compact
                return temp + '\u00B0C ' + desc;
        }
    },

    // --- Forecast ---
    buildForecastContent(layout, props, data) {
        var daily = (data && data.daily) ? data.daily : null;
        if (!daily || daily.length === 0) {
            // Fallback
            var days = props.days || 3;
            var lines = [];
            var dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
            var today = new Date().getDay();
            for (var i = 0; i < days; i++) {
                var d = dayNames[(today + i) % 7];
                lines.push(d + ': 12-20\u00B0C');
            }
            return lines.join('\n');
        }
        var lines = [];
        for (var i = 0; i < daily.length; i++) {
            var day = daily[i];
            switch (layout) {
                case 'compact_row':
                    lines.push((day.Weekday || '').substring(0, 3) + ' ' + Math.round(day.Min) + '/' + Math.round(day.Max) + '\u00B0');
                    break;
                case 'detailed_list':
                    lines.push((day.Weekday || '') + ': ' + Math.round(day.Min) + '\u00B0/' + Math.round(day.Max) + '\u00B0 ' + (day.Desc || ''));
                    break;
                case 'custom': {
                    var template = props.customTemplate || '%day_name%: %temp_min%-%temp_max%\u00B0C';
                    lines.push(template
                        .replace(/%day_name%/g, day.Weekday || '')
                        .replace(/%temp_min%/g, Math.round(day.Min))
                        .replace(/%temp_max%/g, Math.round(day.Max))
                        .replace(/%description%/g, day.Desc || ''));
                    break;
                }
                default: // vertical
                    lines.push((day.Weekday || '') + ': ' + Math.round(day.Min) + '-' + Math.round(day.Max) + '\u00B0C ' + (day.Desc || ''));
            }
        }
        return lines.join('\n');
    },

    // --- Calendar ---
    buildCalendarContent(layout, props, data) {
        if (!data || !data.content) {
            return (props.title || 'Events') + '\n10:00 - Meeting\n14:30 - Review';
        }
        return data.content;
    },

    // --- News ---
    buildNewsContent(layout, props, data) {
        if (!data || !data.content) {
            return (props.title || 'News') + '\n- Headline 1\n- Headline 2';
        }
        return data.content;
    },

    // --- Timer ---
    buildTimerContent(layout, props) {
        var target = props.targetDate || '2026-12-25T00:00:00';
        var label = props.label || '';
        var finishedText = props.finishedText || "Time's up!";
        var targetDate = new Date(target);
        if (isNaN(targetDate.getTime())) return 'Invalid date';

        var diff = targetDate - new Date();
        if (diff < 0) return finishedText;

        var totalSecs = Math.floor(diff / 1000);
        var days = Math.floor(totalSecs / 86400);
        var rem = totalSecs % 86400;
        var hours = Math.floor(rem / 3600);
        var minutes = Math.floor((rem % 3600) / 60);
        var seconds = rem % 60;
        var pad = function(n) { return String(n).padStart(2, '0'); };

        var display;
        if (layout === 'custom') {
            var template = props.customTemplate || '%days% days %hours%:%minutes%:%seconds%';
            display = template
                .replace(/%days%/g, days)
                .replace(/%hours%/g, pad(hours))
                .replace(/%minutes%/g, pad(minutes))
                .replace(/%seconds%/g, pad(seconds))
                .replace(/%total_hours%/g, Math.floor(totalSecs / 3600))
                .replace(/%label%/g, label);
            return display;
        }
        switch (layout) {
            case 'countdown_compact':
                display = days + 'd ' + hours + 'h ' + minutes + 'm';
                break;
            case 'label_above':
                display = days + ' days ' + pad(hours) + ':' + pad(minutes) + ':' + pad(seconds);
                if (label) display = label + '\n' + display;
                return display;
            case 'days_only':
                display = days + ' days';
                break;
            default: // countdown_large
                display = days + ' days ' + pad(hours) + ':' + pad(minutes) + ':' + pad(seconds);
        }
        if (label && layout !== 'label_above') display = label + ': ' + display;
        return display;
    },

    // --- System ---
    buildSystemContent(layout, props, data) {
        if (!data || !data.content) {
            return 'Load: 0.5 0.3 0.2\nMem: 27MB / 512MB\nTemp: 45\u00B0C';
        }
        if (layout === 'horizontal') {
            return data.content.replace(/\n/g, ' | ');
        }
        return data.content;
    },

    // --- Placeholder helpers ---
    applyTimePlaceholders(template, date) {
        var d = date || new Date();
        var h12 = d.getHours() % 12 || 12;
        return template
            .replace(/%HH%/g, String(d.getHours()).padStart(2, '0'))
            .replace(/%hh%/g, String(h12).padStart(2, '0'))
            .replace(/%MM%/g, String(d.getMinutes()).padStart(2, '0'))
            .replace(/%SS%/g, String(d.getSeconds()).padStart(2, '0'))
            .replace(/%dd%/g, String(d.getDate()).padStart(2, '0'))
            .replace(/%mm%/g, String(d.getMonth() + 1).padStart(2, '0'))
            .replace(/%yyyy%/g, String(d.getFullYear()))
            .replace(/%WEEKDAY%/g, d.toLocaleDateString('de-DE', {weekday:'long'}))
            .replace(/%WEEKDAY_SHORT%/g, d.toLocaleDateString('de-DE', {weekday:'short'}))
            .replace(/%MONTH_NAME%/g, d.toLocaleDateString('de-DE', {month:'long'}))
            .replace(/%AMPM%/g, d.getHours() >= 12 ? 'PM' : 'AM');
    },

    // Get appropriate font size for widget type
    getPreviewFontSize(type, props) {
        props = props || {};
        var layout = props.layout || this.getDefaultLayout(type);
        switch (type) {
            case 'widget_clock':
                if (layout === 'digital_large') return Math.min(props.fontSize || 48, 48);
                if (layout === 'full' || layout === 'digital_with_date') return 18;
                return Math.min(props.fontSize || 36, 36);
            case 'widget_weather':
                if (layout === 'minimal') return 36;
                return 18;
            case 'widget_forecast':
                return 13;
            case 'widget_calendar':
                return 13;
            case 'widget_news':
                if (layout === 'single') return 24;
                return 13;
            case 'widget_timer':
                if (layout === 'countdown_large') return 24;
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
    async updatePreview(fabricObj) {
        if (!fabricObj) return;
        var type = fabricObj.get('elementType');
        if (!type || !type.startsWith('widget_')) return;

        var data = fabricObj.get('elementData') || {};
        var props = data.properties || {};

        // Fetch live data from server
        var liveData = await this.fetchWidgetData(type, props);
        var displayText = this.getPreviewContent(type, props, liveData);
        var fontSize = this.getPreviewFontSize(type, props);

        if (fabricObj.type === 'group' && fabricObj._objects && fabricObj._objects.length >= 2) {
            var label = fabricObj._objects[1];
            if (label && label.set) {
                label.set('text', displayText);
                label.set('fontSize', fontSize);
                label.set('fill', props.color || '#333333');
                label.set('fontFamily', props.fontFamily || 'monospace');
                label.set('textAlign', props.textAlign || 'center');
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
                var type = obj.get('elementType');
                if (type !== 'widget_clock' && type !== 'widget_timer') return;
                var data = obj.get('elementData') || {};
                var props = data.properties || {};
                var text = self.getPreviewContent(type, props, null);

                if (obj.type === 'group' && obj._objects && obj._objects.length >= 2) {
                    var label = obj._objects[1];
                    if (label && label.get('text') !== text) {
                        label.set('text', text);
                        changed = true;
                    }
                }
            });
            if (changed) canvas.renderAll();
        }, 60000);
    },

    // Start periodic data refresh for all widgets
    startDataRefresh() {
        if (this._dataRefreshInterval) return;
        var self = this;
        this._dataRefreshInterval = setInterval(function() {
            // Clear cache
            self._dataCache = {};
            // Update all widgets
            var canvas = CanvasManager.getCanvas();
            if (!canvas) return;
            canvas.getObjects().forEach(function(obj) {
                var type = obj.get('elementType');
                if (type && type.startsWith('widget_')) {
                    self.updatePreview(obj);
                }
            });
        }, 60000);
    },

    // Refresh all widgets immediately (e.g. after design load)
    async refreshAllWidgets() {
        this._dataCache = {};
        var canvas = CanvasManager.getCanvas();
        if (!canvas) return;
        var self = this;
        var objects = canvas.getObjects();
        for (var i = 0; i < objects.length; i++) {
            var obj = objects[i];
            var type = obj.get('elementType');
            if (type && type.startsWith('widget_')) {
                await self.updatePreview(obj);
            }
        }
    }
};
