// Creates Fabric.js objects for each element type
// Each element gets custom data properties stored on the Fabric object
const ElementFactory = {
    idCounter: 0,

    generateId() {
        return 'elem_' + (++this.idCounter) + '_' + Date.now();
    },

    createText(options) {
        options = options || {};
        const id = this.generateId();
        const text = new fabric.Textbox(options.text || 'Text', {
            left: options.x || 50,
            top: options.y || 50,
            width: options.width || 200,
            fontSize: options.fontSize || 24,
            fontFamily: options.fontFamily || 'Arial',
            fontWeight: options.fontWeight || 'normal',
            fontStyle: options.fontStyle || 'normal',
            fill: options.color || '#000000',
            textAlign: options.textAlign || 'left',
        });
        text.set('elementId', id);
        text.set('elementType', 'text');
        text.set('elementData', { type: 'text', properties: options.properties || {} });
        return text;
    },

    createImage(options) {
        options = options || {};
        const id = this.generateId();
        const rect = new fabric.Rect({
            left: options.x || 50,
            top: options.y || 50,
            width: options.width || 200,
            height: options.height || 150,
            fill: '#e0e0e0',
            stroke: '#999999',
            strokeWidth: 1,
        });
        rect.set('elementId', id);
        rect.set('elementType', 'image');
        rect.set('elementData', { type: 'image', properties: options.properties || {} });

        // If image is specified, load it
        if (options.properties && options.properties.image) {
            const imgName = options.properties.image;
            const targetRect = rect;
            fabric.Image.fromURL('/image/' + imgName, function(img) {
                if (!img) return;
                const canvas = CanvasManager.getCanvas();
                const left = targetRect.left;
                const top = targetRect.top;
                const w = targetRect.width;
                const h = targetRect.height;

                canvas.remove(targetRect);
                img.set({
                    left: left,
                    top: top,
                    scaleX: w / img.width,
                    scaleY: h / img.height,
                });
                img.set('elementId', targetRect.get('elementId'));
                img.set('elementType', 'image');
                img.set('elementData', targetRect.get('elementData'));
                canvas.add(img);
                canvas.renderAll();
            });
        }

        return rect;
    },

    createShape(options) {
        options = options || {};
        const id = this.generateId();
        const shape = new fabric.Rect({
            left: options.x || 50,
            top: options.y || 50,
            width: options.width || 100,
            height: options.height || 100,
            fill: options.fill || '#000000',
            stroke: options.stroke || '#000000',
            strokeWidth: options.strokeWidth !== undefined ? options.strokeWidth : 1,
            rx: options.rx || 0,
            ry: options.ry || 0,
        });
        shape.set('elementId', id);
        shape.set('elementType', 'shape');
        shape.set('elementData', { type: 'shape', properties: options.properties || {} });
        return shape;
    },

    createWidget(type, options) {
        options = options || {};
        const id = this.generateId();
        var defaultSizes = {
            widget_clock: { w: 200, h: 60 },
            widget_weather: { w: 300, h: 200 },
            widget_forecast: { w: 400, h: 150 },
            widget_calendar: { w: 300, h: 250 },
            widget_news: { w: 300, h: 200 },
            widget_timer: { w: 250, h: 60 },
            widget_custom: { w: 200, h: 60 },
            widget_system: { w: 300, h: 100 },
        };

        var size = defaultSizes[type] || { w: 200, h: 100 };
        var w = options.width || size.w;
        var h = options.height || size.h;

        var bg = new fabric.Rect({
            width: w,
            height: h,
            fill: '#f8f8f8',
            stroke: '#cccccc',
            strokeWidth: 1,
            strokeDashArray: [5, 3],
            rx: 4,
            ry: 4,
            originX: 'center',
            originY: 'center',
        });

        var widgetProps = options.properties || ElementFactory.getDefaultProperties(type);
        var previewText = WidgetPreview.getPreviewContent(type, widgetProps);
        var previewFontSize = WidgetPreview.getPreviewFontSize(type, widgetProps);

        var label = new fabric.Text(previewText, {
            fontSize: previewFontSize,
            fill: widgetProps.color || '#333333',
            fontFamily: widgetProps.fontFamily || 'monospace',
            originX: 'center',
            originY: 'center',
            textAlign: 'center',
        });

        var group = new fabric.Group([bg, label], {
            left: options.x || 50,
            top: options.y || 50,
        });

        group.set('elementId', id);
        group.set('elementType', type);
        group.set('elementData', {
            type: type,
            properties: options.properties || ElementFactory.getDefaultProperties(type)
        });
        return group;
    },

    getDefaultProperties(type) {
        var defaults = {
            widget_clock: {
                format: 'HH:mm',
                timezone: 'Europe/Berlin',
                fontFamily: 'Arial',
                fontSize: 48,
                color: '#000000',
                textAlign: 'center'
            },
            widget_weather: {
                latitude: 52.3759,
                longitude: 9.7320,
                units: 'metric',
                style: 'compact',
                showTemperature: true,
                showCondition: true
            },
            widget_forecast: {
                latitude: 52.3759,
                longitude: 9.7320,
                days: 3,
                layout: 'horizontal',
                showHighLow: true,
                showIcons: true
            },
            widget_calendar: {
                icalUrl: '',
                maxEvents: 5,
                showTime: true,
                daysAhead: 7,
                title: 'Events'
            },
            widget_news: {
                feedUrl: '',
                maxItems: 3,
                showDescription: true,
                layout: 'list',
                title: 'News'
            },
            widget_timer: {
                targetDate: '2026-12-25T00:00:00',
                label: 'Countdown',
                format: 'days',
                finishedText: "Time's up!"
            },
            widget_custom: {
                url: '',
                method: 'GET',
                jsonPath: '',
                prefix: '',
                suffix: '',
                fontFamily: 'Arial',
                fontSize: 24
            },
            widget_system: {
                metrics: ['cpu', 'memory', 'temperature'],
                layout: 'horizontal',
                showLabels: true
            },
        };
        return defaults[type] || {};
    },

    // Create element from Design JSON (loading a saved design)
    fromElement(elem) {
        switch (elem.type) {
            case 'text':
                return this.createText({
                    x: elem.x,
                    y: elem.y,
                    width: elem.width,
                    height: elem.height,
                    text: elem.properties ? elem.properties.text : 'Text',
                    fontSize: elem.properties ? elem.properties.fontSize : 24,
                    fontFamily: elem.properties ? elem.properties.fontFamily : 'Arial',
                    fontWeight: elem.properties ? elem.properties.fontWeight : 'normal',
                    fontStyle: elem.properties ? elem.properties.fontStyle : 'normal',
                    color: elem.properties ? elem.properties.color : '#000000',
                    textAlign: elem.properties ? elem.properties.textAlign : 'left',
                    properties: elem.properties,
                });
            case 'image':
                return this.createImage({
                    x: elem.x,
                    y: elem.y,
                    width: elem.width,
                    height: elem.height,
                    properties: elem.properties,
                });
            case 'shape':
                return this.createShape({
                    x: elem.x,
                    y: elem.y,
                    width: elem.width,
                    height: elem.height,
                    fill: elem.properties ? elem.properties.fill : '#000000',
                    stroke: elem.properties ? elem.properties.stroke : '#000000',
                    strokeWidth: elem.properties ? elem.properties.strokeWidth : 1,
                    rx: elem.properties ? elem.properties.rx : 0,
                    ry: elem.properties ? elem.properties.ry : 0,
                    properties: elem.properties,
                });
            default:
                if (elem.type && elem.type.startsWith('widget_')) {
                    return this.createWidget(elem.type, {
                        x: elem.x,
                        y: elem.y,
                        width: elem.width,
                        height: elem.height,
                        properties: elem.properties,
                    });
                }
                return null;
        }
    }
};
