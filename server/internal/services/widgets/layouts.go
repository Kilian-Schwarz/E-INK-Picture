package widgets

// WidgetLayout describes a predefined layout for a widget type.
type WidgetLayout struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GetLayouts returns available layouts for a widget type.
func GetLayouts(widgetType string) []WidgetLayout {
	layouts, ok := allLayouts[widgetType]
	if !ok {
		return []WidgetLayout{{ID: "default", Name: "Default", Description: "Default layout"}}
	}
	return layouts
}

var allLayouts = map[string][]WidgetLayout{
	"widget_clock": {
		{ID: "digital_large", Name: "Digital Large", Description: "Large time display"},
		{ID: "digital_with_seconds", Name: "With Seconds", Description: "Time with seconds"},
		{ID: "digital_with_date", Name: "With Date", Description: "Time and date"},
		{ID: "date_only", Name: "Date Only", Description: "Date without time"},
		{ID: "full", Name: "Full", Description: "Weekday, date and time"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_weather": {
		{ID: "compact", Name: "Compact", Description: "Icon and temperature"},
		{ID: "standard", Name: "Standard", Description: "Temperature and description"},
		{ID: "detailed", Name: "Detailed", Description: "All weather info"},
		{ID: "minimal", Name: "Minimal", Description: "Temperature only"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_forecast": {
		{ID: "vertical", Name: "Vertical", Description: "Days listed vertically"},
		{ID: "compact_row", Name: "Compact", Description: "Compact single-line per day"},
		{ID: "detailed_list", Name: "Detailed", Description: "Day, high, low, description"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_calendar": {
		{ID: "list", Name: "List", Description: "Time and title per event"},
		{ID: "agenda", Name: "Agenda", Description: "Grouped by date"},
		{ID: "compact", Name: "Compact", Description: "Next 3 events only"},
		{ID: "detailed", Name: "Detailed", Description: "With location info"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_news": {
		{ID: "headlines", Name: "Headlines", Description: "Titles only"},
		{ID: "summary", Name: "Summary", Description: "Title and description"},
		{ID: "single", Name: "Single", Description: "Latest headline large"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_system": {
		{ID: "vertical", Name: "Vertical", Description: "Each metric on its own line"},
		{ID: "horizontal", Name: "Horizontal", Description: "All metrics in one line"},
		{ID: "compact", Name: "Compact", Description: "Values only, no labels"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
	"widget_timer": {
		{ID: "countdown_large", Name: "Large", Description: "Full countdown display"},
		{ID: "countdown_compact", Name: "Compact", Description: "Short format"},
		{ID: "label_above", Name: "Label Above", Description: "Label on top, countdown below"},
		{ID: "days_only", Name: "Days Only", Description: "Only remaining days"},
		{ID: "custom", Name: "Custom Template", Description: "Define your own format"},
	},
}

// Placeholders returns available placeholder keys for a widget type.
func Placeholders(widgetType string) []string {
	p, ok := allPlaceholders[widgetType]
	if !ok {
		return nil
	}
	return p
}

var allPlaceholders = map[string][]string{
	"widget_clock": {
		"%HH%", "%hh%", "%MM%", "%SS%",
		"%dd%", "%mm%", "%yyyy%",
		"%WEEKDAY%", "%WEEKDAY_SHORT%",
		"%MONTH_NAME%", "%AMPM%",
	},
	"widget_weather": {
		"%temperature%", "%feels_like%", "%description%",
		"%humidity%", "%wind_speed%", "%icon%",
		"%temp_min%", "%temp_max%",
	},
	"widget_forecast": {
		"%day_name%", "%temp_min%", "%temp_max%", "%description%",
	},
	"widget_calendar": {
		"%next_event%", "%next_time%", "%event_count%",
	},
	"widget_news": {
		"%headline_1%", "%headline_2%", "%headline_3%",
	},
	"widget_system": {
		"%cpu%", "%memory%", "%temperature%",
		"%uptime%", "%hostname%",
	},
	"widget_timer": {
		"%days%", "%hours%", "%minutes%", "%seconds%",
		"%total_hours%", "%label%",
	},
}
