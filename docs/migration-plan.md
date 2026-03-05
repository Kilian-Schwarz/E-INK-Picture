# Migration Plan: Python Flask -> Go

## 1. API-Endpunkte

| Method | URL | Request Body | Response Body | Content-Type |
|--------|-----|-------------|---------------|--------------|
| GET | `/designer` | - | HTML (designer.html template) | text/html |
| GET | `/design` | - | JSON: Active design object with filled content fields | application/json |
| GET | `/designs` | - | JSON: Array of `{name, active}` objects | application/json |
| GET | `/get_design_by_name?name={name}` | - | JSON: Design object with filled content fields | application/json |
| POST | `/set_active_design` | `{"name": "..."}` | `{"message": "Active design set."}` | application/json |
| POST | `/update_design` | Design object + `save_as_new`, `keep_alive` | `{"message": "..."}` | application/json |
| POST | `/clone_design` | `{"name": "..."}` | `{"message": "Design cloned"}` | application/json |
| POST | `/delete_design` | `{"name": "..."}` | `{"message": "Design deleted"}` | application/json |
| POST | `/upload_image` | multipart/form-data: `file` | `{"message": "...", "file_path": "name.png"}` or `{"font_path": "name.ttf"}` | application/json |
| GET | `/images_all` | - | JSON: Array of PNG filenames | application/json |
| GET | `/image/{filename}` | - | Binary PNG file | image/png |
| POST | `/delete_image` | `{"filename": "name.png"}` | `{"message": "Image deleted"}` | application/json |
| GET | `/fonts_all` | - | JSON: Array of TTF/OTF filenames | application/json |
| GET | `/font/{filename}` | - | Binary font file | application/octet-stream |
| GET | `/weather_styles` | - | JSON: Array of style names | application/json |
| GET | `/location_search?q={query}` | - | JSON: Array of `{display_name, lat, lon}` | application/json |
| GET | `/preview?name={name}` | - | PNG image (800x480, 1-bit monochrome) | image/png |
| POST | `/update_settings` | any | `{"message": "Settings updated."}` | application/json |

## 2. Frontend-API-Calls

| JS-Funktion | Method | URL | Payload | Erwartete Response |
|-------------|--------|-----|---------|-------------------|
| `loadDesignList()` | GET | `/designs` | - | JSON Array `[{name, active}]` |
| `loadActiveDesign()` | GET | `/design` | - | JSON Design object |
| Load Design (inline) | GET | `/get_design_by_name?name=<name>` | - | JSON Design object |
| Clone Design (inline) | POST | `/clone_design` | `{name: string}` | `{message: string}` |
| Delete Design (inline) | POST | `/delete_design` | `{name: string}` | `{message: string}` |
| Set Active (inline) | POST | `/set_active_design` | `{name: string}` | `{message: string}` |
| `saveDesign(asNew)` | POST | `/update_design` | Full design + save_as_new + keep_alive | `{message: string}` |
| Preview (inline) | GET | `/preview` or `/preview?name=<name>` | - | Blob (PNG image) |
| Media Library load | GET | `/images_all` | - | JSON Array of filenames |
| Image display | GET | `/image/<filename>` | - | Image binary |
| Fonts load | GET | `/fonts_all` | - | JSON Array of font names |
| Font load | GET | `/font/<fontname>` | - | Font binary |
| Delete Image (inline) | POST | `/delete_image` | `{filename: string}` | `{message: string}` |
| Location Search | GET | `/location_search?q=<query>` | - | JSON Array `[{display_name, lat, lon}]` |
| Weather Styles load | GET | `/weather_styles` | - | JSON Array of style names |
| Upload (XMLHttpRequest) | POST | `/upload_image` | FormData with file | JSON response |

## 3. Design-JSON-Schema

```json
{
  "name": "string",
  "timestamp": "YYYY-MM-DD_HH-MM-SS",
  "active": true,
  "keep_alive": false,
  "resolution": [800, 480],
  "filename": "design_YYYY-MM-DD_HH-MM-SS.json",
  "modules": [
    {
      "type": "text|image|weather|datetime|timer|calendar|line|news",
      "content": "string (filled dynamically for weather/datetime/timer/calendar)",
      "position": { "x": 200, "y": 160 },
      "size": { "width": 400, "height": 200 },
      "styleData": {
        "font": "Arial.ttf",
        "fontSize": "18",
        "fontBold": "true",
        "fontItalic": "false",
        "fontStrike": "false",
        "textAlign": "left",
        "textColor": "#000000",
        "offlineClientSync": "false",
        "image": "photo.png",
        "crop_x": 0,
        "crop_y": 0,
        "crop_w": 100,
        "crop_h": 100,
        "datetimeFormat": "YYYY-MM-DD HH:mm",
        "latitude": "52.52",
        "longitude": "13.41",
        "locationName": "Berlin",
        "weatherStyle": "default",
        "timerTarget": "2025-01-01 00:00:00",
        "timerFormat": "D days, HH:MM:SS",
        "calendarURL": "https://...",
        "maxEvents": "5"
      }
    }
  ]
}
```

**Storage:** Files in `./designs/` directory, named `design_{timestamp}.json` or `design_default.json`.

**Active Design:** Only one design has `"active": true`. On startup, `ensure_active_design()` creates default if none exists.

## 4. Modul-Typen

### Text
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| textContent | string | "" | Static text content |
| font | string | "sans-serif" | Font filename |
| fontSize | string/number | "18" | Font size in px |
| fontBold | string/bool | "false" | Bold flag |
| fontItalic | string/bool | "false" | Italic flag |
| fontStrike | string/bool | "false" | Strikethrough flag |
| textAlign | string | "left" | left/center/right |
| offlineClientSync | string/bool | "false" | Enable offline sync |

### Image
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| image | string | "" | Image filename |
| crop_x | number | 0 | Crop X offset |
| crop_y | number | 0 | Crop Y offset |
| crop_w | number | full width | Crop width |
| crop_h | number | full height | Crop height |

### Weather
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| latitude | string | "52.52" | Location latitude |
| longitude | string | "13.41" | Location longitude |
| locationName | string | "" | Human-readable location |
| weatherStyle | string | "default" | Weather format template |
| font, fontSize, fontBold, fontItalic, fontStrike, textAlign | - | - | Same as Text |
| offlineClientSync | string/bool | "false" | Enable offline weather fetch |

### DateTime
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| datetimeFormat | string | "YYYY-MM-DD HH:mm" | Format string |
| font, fontSize, fontBold, fontItalic, fontStrike, textAlign | - | - | Same as Text |
| offlineClientSync | string/bool | "false" | Enable offline time update |

### Timer
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| timerTarget | string | "2025-01-01 00:00:00" | Target datetime |
| timerFormat | string | "D days, HH:MM:SS" | Display format |
| font, fontSize, fontBold, fontItalic, fontStrike, textAlign | - | - | Same as Text |
| offlineClientSync | string/bool | "false" | Enable offline countdown |

### Calendar
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| calendarURL | string | "" | iCal/Webcal URL |
| maxEvents | string/number | "5" | Max events to show |
| font, fontSize, fontBold, fontItalic, fontStrike, textAlign | - | - | Same as Text |
| offlineClientSync | string/bool | "false" | Enable offline fetch |

### News
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| newsHeadline | string | "" | News headline text |
| font, fontSize, fontBold, fontItalic, fontStrike, textAlign | - | - | Same as Text |

### Line/Shape
| Feld | Typ | Default | Beschreibung |
|------|-----|---------|-------------|
| (none) | - | - | Only uses position and size, renders as filled rectangle |

## 5. Client-Kommunikation

| Endpunkt | Method | Parameter | Response | Zweck |
|----------|--------|-----------|----------|-------|
| `/design` | GET | - | JSON Design | Aktives Design laden |
| `/image/{filename}` | GET | filename from styleData | Binary PNG | Bilder herunterladen |
| `/font/{filename}` | GET | font name from styleData | Binary TTF/OTF | Fonts herunterladen |

**Client Config:**
- BASE_URL: `http://127.0.0.1:5000`
- Timeout: 5 seconds
- Max Retries: 3, Retry Delay: 20 seconds
- Fallback: `./local_design.json` (cached design)
- Cache: `./temp_files/` for images and fonts

**Offline Sync:** When `offlineClientSync=true`:
- DateTime: Uses local system time
- Timer: Calculates countdown locally
- Weather: Fetches from open-meteo.com directly
- Calendar: Fetches from iCal URL directly

**Display:** 800x480px, 1-bit monochrome, Waveshare epd7in5_V2

## 6. Dateisystem

### Verzeichnisse

```
./designs/              → Design JSON files
./uploaded_images/      → Uploaded PNG images (auto-converted from JPG/JPEG)
./fonts/                → Custom TTF/OTF fonts
./weather_styles/       → Weather format templates (JSON)
./templates/            → HTML templates (designer.html)
```

### Dateinamen-Konventionen

- Designs: `design_{YYYY-MM-DD_HH-MM-SS}.json` or `design_default.json`
- Images: Original filename, secured via `secure_filename()`, always PNG
- Fonts: Original filename, secured via `secure_filename()`, TTF/OTF
- Weather styles: `{style_name}.json`

### Erlaubte Dateitypen

- Images: `.png`, `.jpg`, `.jpeg` (converted to PNG on upload)
- Fonts: `.ttf`, `.otf`

### E-Ink Display Constants

- Width: 800px, Height: 480px
- Offset X: 200px, Offset Y: 160px
- Display area in editor: 1200x800px total, E-Ink frame at (200, 160)

### Preview Rendering

- Output: 1-bit monochrome PNG (Pillow Mode '1')
- Size: 800x480px
- Modules outside frame are skipped
- Text: Word-wrapped with font support
- Images: Cropped, resized, converted to 1-bit
- Lines: Filled black rectangles
