package widgets

import (
	"context"
	"image"
	"image/color"
)

// RenderContext provides rendering context for widgets.
type RenderContext struct {
	Bounds    image.Rectangle
	FontFace  interface{} // font.Face
	TextColor color.RGBA
}

// WidgetRenderer is the interface all widget renderers implement.
type WidgetRenderer interface {
	Render(ctx context.Context, props map[string]any, bounds image.Rectangle, img *image.RGBA) error
}

// WidgetData holds the text output of a widget for preview rendering.
type WidgetData struct {
	Text string
	Err  error
}
