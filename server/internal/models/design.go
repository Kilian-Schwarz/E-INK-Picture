package models

// Position represents an x/y coordinate within the design canvas.
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Size represents width and height of a module.
type Size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// StyleData holds all optional styling and configuration fields for a module.
type StyleData struct {
	Font              *string  `json:"font,omitempty"`
	FontSize          *string  `json:"fontSize,omitempty"`
	FontBold          *string  `json:"fontBold,omitempty"`
	FontItalic        *string  `json:"fontItalic,omitempty"`
	FontStrike        *string  `json:"fontStrike,omitempty"`
	TextAlign         *string  `json:"textAlign,omitempty"`
	TextColor         *string  `json:"textColor,omitempty"`
	OfflineClientSync *string  `json:"offlineClientSync,omitempty"`
	Image             *string  `json:"image,omitempty"`
	CropX             *float64 `json:"crop_x,omitempty"`
	CropY             *float64 `json:"crop_y,omitempty"`
	CropW             *float64 `json:"crop_w,omitempty"`
	CropH             *float64 `json:"crop_h,omitempty"`
	DatetimeFormat    *string  `json:"datetimeFormat,omitempty"`
	Latitude          *string  `json:"latitude,omitempty"`
	Longitude         *string  `json:"longitude,omitempty"`
	LocationName      *string  `json:"locationName,omitempty"`
	WeatherStyle      *string  `json:"weatherStyle,omitempty"`
	TimerTarget       *string  `json:"timerTarget,omitempty"`
	TimerFormat       *string  `json:"timerFormat,omitempty"`
	CalendarURL       *string  `json:"calendarURL,omitempty"`
	MaxEvents         *string  `json:"maxEvents,omitempty"`
	NewsHeadline      *string  `json:"newsHeadline,omitempty"`
}

// Module represents a single UI module placed on the design canvas.
type Module struct {
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Position  Position  `json:"position"`
	Size      Size      `json:"size"`
	StyleData StyleData `json:"styleData"`
}

// Design represents a complete e-ink display design with its modules and metadata.
type Design struct {
	Name       string   `json:"name"`
	Timestamp  string   `json:"timestamp"`
	Active     bool     `json:"active"`
	KeepAlive  bool     `json:"keep_alive"`
	Resolution []int    `json:"resolution"`
	Filename   string   `json:"filename"`
	Modules    []Module `json:"modules"`
}

// DesignMeta holds lightweight metadata about a design.
type DesignMeta struct {
	Name     string `json:"name"`
	IsActive bool   `json:"is_active"`
}

// FileInfo holds basic file information.
type FileInfo struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}
