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
        var w = options.width || 200;
        var h = options.height || 60;
        var text = new fabric.Textbox(options.text || 'Text', {
            left: options.x || 50,
            top: options.y || 50,
            width: w,
            fontSize: options.fontSize || 24,
            fontFamily: options.fontFamily || 'Arial',
            fontWeight: options.fontWeight || 'normal',
            fontStyle: options.fontStyle || 'normal',
            fill: options.color || '#000000',
            textAlign: options.textAlign || 'left',
            clipPath: new fabric.Rect({
                width: w,
                height: h,
                top: -h / 2,
                left: -w / 2,
                absolutePositioned: false,
            }),
        });
        text.set('elementId', id);
        text.set('elementType', 'text');
        text.set('elementData', { type: 'text', properties: options.properties || {} });
        text.set('_clipH', h);
        return text;
    },

    createImage(options) {
        options = options || {};
        const id = this.generateId();
        var resizeMode = (options.properties && options.properties.resizeMode) || 'proportional';
        const rect = new fabric.Rect({
            left: options.x || 50,
            top: options.y || 50,
            width: options.width || 200,
            height: options.height || 150,
            fill: '#e0e0e0',
            stroke: '#999999',
            strokeWidth: 1,
            lockUniScaling: resizeMode !== 'free',
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
                    lockUniScaling: resizeMode !== 'free',
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

        var widgetProps = options.properties || ElementFactory.getDefaultProperties(type);

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

        var previewText = WidgetPreview.getPreviewContent(type, widgetProps, null);
        var previewFontSize = WidgetPreview.getPreviewFontSize(type, widgetProps);

        var label = new fabric.Textbox(previewText, {
            width: w - 16,
            fontSize: previewFontSize,
            fill: widgetProps.color || '#333333',
            fontFamily: widgetProps.fontFamily || 'monospace',
            left: -w / 2 + 8,
            top: -h / 2 + 4,
            originX: 'left',
            originY: 'top',
            textAlign: widgetProps.textAlign || 'left',
            splitByGrapheme: false,
        });

        var clipRect = new fabric.Rect({
            width: w,
            height: h,
            left: -w / 2,
            top: -h / 2,
            absolutePositioned: false,
        });

        var group = new fabric.Group([bg, label], {
            left: options.x || 50,
            top: options.y || 50,
            lockUniScaling: false,
            clipPath: clipRect,
        });

        group.set('elementId', id);
        group.set('elementType', type);
        group.set('elementData', {
            type: type,
            properties: widgetProps
        });

        // Fetch live data asynchronously
        setTimeout(function() {
            WidgetPreview.updatePreview(group);
        }, 100);

        return group;
    },

    getDefaultProperties(type) {
        var defaults = {
            widget_clock: {
                layout: 'digital_large',
                timezone: 'Europe/Berlin',
                fontFamily: 'Arial',
                fontSize: 48,
                color: '#000000',
                textAlign: 'center'
            },
            widget_weather: {
                layout: 'compact',
                latitude: 52.3759,
                longitude: 9.7320,
                fontSize: 18,
                color: '#000000',
                textAlign: 'left'
            },
            widget_forecast: {
                layout: 'vertical',
                latitude: 52.3759,
                longitude: 9.7320,
                days: 3,
                fontSize: 13,
                color: '#000000',
                textAlign: 'left'
            },
            widget_calendar: {
                layout: 'list',
                icalUrl: '',
                maxEvents: 5,
                showTime: true,
                daysAhead: 7,
                title: 'Events',
                fontSize: 13,
                color: '#000000',
                textAlign: 'left'
            },
            widget_news: {
                layout: 'headlines',
                feedUrl: '',
                maxItems: 3,
                showDescription: false,
                title: 'News',
                fontSize: 13,
                color: '#000000',
                textAlign: 'left'
            },
            widget_timer: {
                layout: 'countdown_large',
                targetDate: '2026-12-25T00:00:00',
                label: 'Countdown',
                finishedText: "Time's up!",
                color: '#000000',
                textAlign: 'center'
            },
            widget_custom: {
                url: '',
                method: 'GET',
                jsonPath: '',
                prefix: '',
                suffix: '',
                fontFamily: 'Arial',
                fontSize: 24,
                color: '#000000'
            },
            widget_system: {
                layout: 'vertical',
                showLabels: true,
                fontSize: 12,
                color: '#000000',
                textAlign: 'left'
            },
        };
        return defaults[type] || {};
    },

    // Create element from Design JSON (loading a saved design)
    fromElement(elem) {
        switch (elem.type) {
            case 'text':
            case 'i-text':
            case 'textbox':
                return this.createText({
                    x: elem.x,
                    y: elem.y,
                    width: elem.width,
                    height: elem.height || 60,
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
