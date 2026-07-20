// Widget preview rendering on canvas
// Provides live data and layout-based rendering for each widget type
var WidgetPreview = {
    _clockInterval: null,
    _dataRefreshInterval: null,
    _dataCache: {},
    _CACHE_TTL: 60000,
    // Debounce for property-edit driven content re-fetches so typing (e.g. the
    // custom-template input) does not POST /api/widget_content per keystroke.
    _UPDATE_DEBOUNCE_MS: 350,

    // Fetch the widget's text content from the server.
    //
    // Data widgets (weather, forecast, calendar, news, system, custom) resolve
    // their content via POST /api/widget_content with the FULL properties map.
    // The server returns the exact string the E-Ink panel draws, so the canvas
    // renders it verbatim — canvas == panel by construction. clock and timer
    // stay client-live (they tick per second/minute; a static server render
    // would freeze) and never hit the server.
    async fetchWidgetData(type, props) {
        props = props || {};

        if (type === 'widget_clock') return { time: new Date() };
        if (type === 'widget_timer') return { targetDate: props.targetDate };

        var cacheKey = type + JSON.stringify(props);
        var cached = this._dataCache[cacheKey];
        if (cached && Date.now() - cached.time < this._CACHE_TTL) {
            return cached.data;
        }

        try {
            var res = await fetch('/api/widget_content', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ type: type, properties: props }),
            });
            // 400 (unsupported type) / network error → null → neutral
            // placeholder in getPreviewContent, never a crash.
            if (!res.ok) return null;
            var data = await res.json();
            this._dataCache[cacheKey] = { data: data, time: Date.now() };
            return data;
        } catch (e) {
            console.warn('Widget content fetch failed:', type, e);
            return null;
        }
    },

    // Get preview content based on layout and live data.
    //
    // clock/timer format client-live. All other widgets are PASSTHROUGH: they
    // render the server's {content} verbatim, so canvas == panel by
    // construction and the formatting lives exactly once, in the Go dispatch
    // (WidgetTextContent -> fill*Content). Never reimplement a widget's
    // formatting or math here — that forks the single source and drifts
    // silently. Passthrough widgets whose content changes on its own (e.g.
    // widget_progress, worst case once per hour in the "day" period) stay
    // honest via the 60s data refresh.
    //
    // liveData.content is authoritative; it is set as canvas text via Fabric
    // (label.set('text', ...)) which draws to the 2D context — no innerHTML, so
    // server-side free text (Home Assistant friendly_name/zone/state, news
    // headlines) can never be HTML-injected (AC-SEC11). The type-label
    // placeholder only shows on a rare fetch failure (the server otherwise
    // always returns a content string, even "No data").
    getPreviewContent(type, props, liveData) {
        props = props || {};
        var layout = props.layout || this.getDefaultLayout(type);

        switch (type) {
            case 'widget_clock':
                return this.buildClockContent(layout, props);
            case 'widget_timer':
                return this.buildTimerContent(layout, props);
            case 'widget_weather':
            case 'widget_forecast':
            case 'widget_calendar':
            case 'widget_news':
            case 'widget_system':
            case 'widget_custom':
            case 'widget_hass':
            case 'widget_progress':
            case 'widget_holidays':
                return (liveData && typeof liveData.content === 'string')
                    ? liveData.content
                    : this._widgetTypeLabel(type);
            default:
                return this._widgetTypeLabel(type);
        }
    },

    _widgetTypeLabel(type) {
        return type.replace('widget_', '').toUpperCase();
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
            widget_progress: 'bar_percent',
            widget_holidays: 'next_countdown',
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

    // Get font size from widget properties, with sensible defaults
    getPreviewFontSize(type, props) {
        props = props || {};
        // Use the user-configured fontSize if available
        if (props.fontSize && props.fontSize > 0) {
            return props.fontSize;
        }
        // Fallback defaults per widget type
        var defaults = {
            widget_clock: 48,
            widget_weather: 18,
            widget_forecast: 13,
            widget_calendar: 13,
            widget_news: 13,
            widget_timer: 24,
            widget_custom: 24,
            widget_system: 12,
            widget_hass: 18,
            // Must match preview.go's defaultFontSize for widget_progress (18).
            widget_progress: 18,
            // Must match preview.go's widgetDefaultFontSizes for
            // widget_holidays (13); pinned by
            // TestWidgetDefaultFontSizesMatchFrontend.
            widget_holidays: 13,
        };
        return defaults[type] || 14;
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
            var bg = fabricObj._objects[0];
            var label = fabricObj._objects[1];
            var w = bg ? bg.width : fabricObj.width;
            var h = bg ? bg.height : fabricObj.height;

            if (label && label.set) {
                label.set('text', displayText);
                label.set('fontSize', fontSize);
                label.set('fill', props.color || '#333333');
                label.set('fontFamily', props.fontFamily || 'monospace');
                label.set('textAlign', props.textAlign || 'left');
                label.set('width', w - 16);
                label.set('left', -w / 2 + 8);
                label.set('top', -h / 2 + 4);
                label.set('originX', 'left');
                label.set('originY', 'top');
            }

            fabricObj.dirty = true;
        }

        CanvasManager.getCanvas().renderAll();
    },

    // Debounced updatePreview for property-panel edits. A prop change now
    // triggers a POST /api/widget_content, so rapid edits (e.g. typing the
    // custom template) are coalesced into a single trailing-edge fetch per
    // widget. The timer lives on the fabric object so concurrent widgets do not
    // cancel each other. Non-typed, discrete edits (color/align/layout) keep
    // calling updatePreview directly for instant feedback.
    updatePreviewDebounced(fabricObj) {
        if (!fabricObj) return;
        var self = this;
        if (fabricObj._widgetPreviewDebounce) {
            clearTimeout(fabricObj._widgetPreviewDebounce);
        }
        fabricObj._widgetPreviewDebounce = setTimeout(function() {
            fabricObj._widgetPreviewDebounce = null;
            self.updatePreview(fabricObj);
        }, this._UPDATE_DEBOUNCE_MS);
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

    // Invalidate cache for a specific widget type (e.g. on URL change)
    invalidateCache(type) {
        var keysToRemove = [];
        for (var k in this._dataCache) {
            if (k.indexOf(type) === 0) {
                keysToRemove.push(k);
            }
        }
        keysToRemove.forEach(function(k) { delete this._dataCache[k]; }.bind(this));
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
