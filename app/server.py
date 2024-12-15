from flask import Flask, jsonify, request, send_file, render_template
import os
import json
from werkzeug.utils import secure_filename
from PIL import Image, ImageDraw, ImageFont
import requests
from datetime import datetime, timezone
import calendar
import io
import logging
import glob
from icalendar import Calendar
from urllib.parse import urlparse, urlunparse
from tzlocal import get_localzone  # Import für lokale Zeitzone

app = Flask(__name__)

UPLOAD_FOLDER = './uploaded_images'
DESIGN_FOLDER = './designs'
FONT_FOLDER = './fonts'
WEATHER_STYLES_FOLDER = './weather_styles'
ALLOWED_EXTENSIONS = {'png', 'jpg', 'jpeg', 'ttf', 'otf'}
ALLOWED_IMAGE_EXTENSIONS = {'png','jpg','jpeg'}
ALLOWED_FONT_EXTENSIONS = {'ttf','otf'}

app.config['UPLOAD_FOLDER'] = UPLOAD_FOLDER
app.config['DESIGN_FOLDER'] = DESIGN_FOLDER
app.config['FONT_FOLDER'] = FONT_FOLDER
app.config['WEATHER_STYLES_FOLDER'] = WEATHER_STYLES_FOLDER

os.makedirs(UPLOAD_FOLDER, exist_ok=True)
os.makedirs(DESIGN_FOLDER, exist_ok=True)
os.makedirs(FONT_FOLDER, exist_ok=True)
os.makedirs(WEATHER_STYLES_FOLDER, exist_ok=True)

logging.basicConfig(level=logging.INFO)

EINK_OFFSET_X = 200
EINK_OFFSET_Y = 160
EINK_WIDTH = 800
EINK_HEIGHT = 480

def allowed_image_file(filename):
    ext = '.' in filename and filename.rsplit('.', 1)[1].lower()
    return ext in ALLOWED_IMAGE_EXTENSIONS

def allowed_font_file(filename):
    ext = '.' in filename and filename.rsplit('.', 1)[1].lower()
    return ext in ALLOWED_FONT_EXTENSIONS

def ensure_active_design():
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    if not files:
        default_design = {
            "modules": [],
            "resolution": [800, 480],
            "name": "Default Design",
            "timestamp": "initial",
            "active": True,
            "keep_alive": False
        }
        filename = "design_default.json"
        with open(os.path.join(DESIGN_FOLDER, filename), 'w') as f:
            json.dump(default_design, f)
    set_active_design()

def set_active_design(name=None):
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    designs = []
    for fn in files:
        with open(os.path.join(DESIGN_FOLDER, fn), 'r') as f:
            d = json.load(f)
            d['filename'] = fn
            designs.append(d)
    if name:
        for d in designs:
            if d['name'] == name:
                d['active'] = True
            else:
                d['active'] = False
            with open(os.path.join(DESIGN_FOLDER, d['filename']), 'w') as f:
                json.dump(d, f)
    else:
        if not any(d.get('active', False) for d in designs):
            if designs:
                designs[0]['active'] = True
                with open(os.path.join(DESIGN_FOLDER, designs[0]['filename']), 'w') as f:
                    json.dump(designs[0], f)

def get_active_design():
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    for fn in files:
        with open(os.path.join(DESIGN_FOLDER, fn), 'r') as f:
            d = json.load(f)
            if d.get('active'):
                return d, fn
    return None, None

ensure_active_design()

def get_design_by_name(name):
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    for fn in files:
        with open(os.path.join(DESIGN_FOLDER, fn), 'r') as f:
            d = json.load(f)
            if d['name'] == name:
                return d
    return None

def format_datetime(fmt):
    fmt = fmt.replace('YYYY','%Y')
    fmt = fmt.replace('MM','%m')
    fmt = fmt.replace('DD','%d')
    fmt = fmt.replace('HH','%H')
    fmt = fmt.replace('mm','%M')
    fmt = fmt.replace('ss','%S')
    local_tz = get_localzone()
    now = datetime.now(local_tz)
    return now.strftime(fmt)

def fetch_weather(lat, lon):
    url = f"https://api.open-meteo.com/v1/forecast?latitude={lat}&longitude={lon}&hourly=temperature_2m,weathercode,precipitation&daily=weathercode,temperature_2m_max,temperature_2m_min,sunrise,sunset&current_weather=true&forecast_days=4&timezone=Europe%2FBerlin"
    r = requests.get(url)
    if r.status_code!=200:
        return None
    data = r.json()
    if 'current_weather' not in data:
        return None

    current_temp = data['current_weather']['temperature']
    current_code = data['current_weather']['weathercode']
    current_desc, current_icon = weathercode_to_desc_icon(current_code,False)

    daily_list = []
    for i in range(4):
        code = data['daily']['weathercode'][i]
        tmax = data['daily']['temperature_2m_max'][i]
        tmin = data['daily']['temperature_2m_min'][i]
        desc, icon = weathercode_to_desc_icon(code,False)
        dt_str = data['daily']['time'][i]
        dt = datetime.strptime(dt_str, '%Y-%m-%d')
        weekday = calendar.day_name[dt.weekday()]
        daily_list.append({
            "min": tmin,
            "max": tmax,
            "desc": desc,
            "icon": icon,
            "date": dt_str,
            "weekday": weekday
        })

    hourly_list = []
    times = data['hourly']['time']
    temps = data['hourly']['temperature_2m']
    codes = data['hourly']['weathercode']
    prec = data['hourly']['precipitation']
    for i in range(0,len(times),2):
        htime = times[i][11:16]
        htemp = temps[i]
        hcode = codes[i]
        hdesc,hicon = weathercode_to_desc_icon(hcode, False)
        hprec = prec[i]
        hourly_list.append({"time":htime, "temp":htemp, "desc":hdesc, "icon":hicon, "precip":hprec})

    sunrise = data['daily']['sunrise'][0]
    sunset = data['daily']['sunset'][0]

    return {
        "current_temp": current_temp,
        "current_desc": current_desc,
        "current_icon": current_icon,
        "current_code": current_code,
        "daily": daily_list,
        "hourly": hourly_list,
        "sunrise": sunrise,
        "sunset": sunset
    }

def fetch_calendar(url, max_events):
    try:
        # Parse die URL
        parsed_url = urlparse(url)
        
        # Wenn das Schema 'webcal' ist, ändere es zu 'https'
        if parsed_url.scheme == 'webcal':
            parsed_url = parsed_url._replace(scheme='https')
            url = urlunparse(parsed_url)
        
        # Führe die Anfrage durch mit Timeout
        response = requests.get(url, timeout=10)
        if response.status_code != 200:
            logging.error(f"Failed to fetch calendar. Status code: {response.status_code}")
            return None
        
        # Parse den Kalenderinhalt
        cal = Calendar.from_ical(response.content)
        events = []
        
        # Lokale Zeitzone ermitteln
        local_tz = get_localzone()
        now = datetime.now(local_tz)
        
        for component in cal.walk():
            if component.name == "VEVENT":
                start = component.get('dtstart').dt
                
                # Sicherstellen, dass 'start' ein datetime-Objekt ist
                if isinstance(start, datetime):
                    # Wenn 'start' offset-naive ist, setze es auf lokale Zeitzone
                    if start.tzinfo is None:
                        start = start.replace(tzinfo=local_tz)
                    else:
                        start = start.astimezone(local_tz)
                    
                    if start >= now:
                        summary = component.get('summary')
                        description = component.get('description', '')
                        events.append({'start': start, 'summary': summary, 'description': description})
        
        # Sortiere die Ereignisse und begrenze die Anzahl
        events = sorted(events, key=lambda x: x['start'])[:max_events]
        return events
    except Exception as e:
        logging.error(f"Error fetching calendar: {e}")
        return None

def weathercode_to_desc_icon(code, is_night=False):
    day_map = {
        0: ("Clear sky", "clear_day.png"),
        1: ("Mainly clear", "clear_day.png"),
        2: ("Partly cloudy", "cloudy_day.png"),
        3: ("Overcast", "cloudy_day.png"),
        45:("Fog", "fog_day.png"),
        48:("Rime fog", "fog_day.png"),
        51:("Light drizzle", "drizzle_day.png"),
        61:("Slight rain", "rain_day.png"),
        63:("Moderate rain", "rain_day.png"),
        65:("Heavy rain", "rain_day.png"),
        80:("Rain showers", "shower_day.png")
    }

    night_map = {
        0: ("Clear sky", "clear_night.png"),
        1: ("Mainly clear", "clear_night.png"),
        2: ("Partly cloudy", "cloudy_night.png"),
        3: ("Overcast", "cloudy_night.png"),
        45:("Fog", "fog_night.png"),
        48:("Rime fog", "fog_night.png"),
        51:("Light drizzle", "drizzle_night.png"),
        61:("Slight rain", "rain_night.png"),
        63:("Moderate rain", "rain_night.png"),
        65:("Heavy rain", "rain_night.png"),
        80:("Rain showers", "shower_night.png")
    }

    if is_night and code in night_map:
        return night_map[code]
    return day_map.get(code, ("Unknown","cloudy_day.png"))

@app.route('/weather_styles', methods=['GET'])
def weather_styles():
    files = glob.glob(os.path.join(WEATHER_STYLES_FOLDER, '*.json'))
    styles = [os.path.splitext(os.path.basename(f))[0] for f in files]
    return jsonify(styles)

def apply_weather_style(style, wdata):
    style_file = os.path.join(app.config['WEATHER_STYLES_FOLDER'], style+'.json')
    if not os.path.exists(style_file):
        return "No data"

    with open(style_file, 'r') as f:
        tmpl = json.load(f)

    text = tmpl.get('format', "No format")
    text = text.replace('{current_temp}', str(wdata['current_temp']))
    text = text.replace('{current_desc}', wdata['current_desc'])

    df_lines = []
    for day in wdata['daily']:
        line = f"{day['weekday']}: {int(day['min'])}-{int(day['max'])}°C {day['desc']}"
        df_lines.append(line)
    daily_forecast_text = "\n".join(df_lines)

    text = text.replace('{daily_forecast}', daily_forecast_text)
    return text

@app.route('/get_design_by_name', methods=['GET'])
def get_design_by_name_endpoint():
    name = request.args.get('name','')
    d = get_design_by_name(name)
    if not d:
        return jsonify({"message":"Design not found"}),404
    for m in d['modules']:
        if m['type']=='datetime':
            styleData = m.get('styleData',{})
            dt_fmt = styleData.get('datetimeFormat','YYYY-MM-DD HH:mm')
            current_dt = format_datetime(dt_fmt)
            m['content'] = current_dt
        elif m['type']=='weather':
            styleData = m.get('styleData',{})
            lat = styleData.get('latitude','52.52')
            lon = styleData.get('longitude','13.41')
            wdata = fetch_weather(lat, lon)
            if not wdata:
                m['content'] = "No data"
            else:
                ws = styleData.get('weatherStyle','default')
                m['content'] = apply_weather_style(ws, wdata)
        elif m['type']=='timer':
            styleData = m.get('styleData', {})
            target_str = styleData.get('timerTarget', '2025-01-01 00:00:00')
            try:
                target_dt = datetime.strptime(target_str, "%Y-%m-%d %H:%M:%S")
                local_tz = get_localzone()
                # Wenn 'target_dt' offset-naive ist, setze es auf lokale Zeitzone
                if target_dt.tzinfo is None:
                    target_dt = target_dt.replace(tzinfo=local_tz)
                else:
                    target_dt = target_dt.astimezone(local_tz)
                now = datetime.now(local_tz)
                diff = target_dt - now
                if diff.total_seconds() < 0:
                    m['content'] = "Time's up!"
                else:
                    fmt = styleData.get('timerFormat', 'D days, HH:MM:SS')
                    days = diff.days
                    sec = diff.seconds
                    hours = sec // 3600
                    minutes = (sec % 3600) // 60
                    seconds = sec % 60
                    display = fmt.replace('D', str(days))
                    display = display.replace('HH', f"{hours:02d}")
                    display = display.replace('MM', f"{minutes:02d}")
                    display = display.replace('SS', f"{seconds:02d}")
                    m['content'] = display
            except Exception as e:
                logging.error(f"Error processing timer: {e}")
                m['content'] = "Invalid timer target"
        elif m['type'] == 'calendar':
            styleData = m.get('styleData', {})
            calendar_url = styleData.get('calendarURL', '')
            max_events = int(styleData.get('maxEvents', 5))
            events = fetch_calendar(calendar_url, max_events)
            if not events:
                m['content'] = "No events"
            else:
                formatted_events = "\n".join([f"{event['start'].strftime('%Y-%m-%d %H:%M')} - {event['summary']}" for event in events])
                m['content'] = formatted_events
    return jsonify(d)

@app.route('/design', methods=['GET'])
def get_design():
    design, fn = get_active_design()
    if not design:
        return jsonify({"message":"No design"}),404

    for m in design['modules']:
        if m['type']=='datetime':
            styleData = m.get('styleData',{})
            dt_fmt = styleData.get('datetimeFormat','YYYY-MM-DD HH:mm')
            current_dt = format_datetime(dt_fmt)
            m['content'] = current_dt
        elif m['type']=='weather':
            styleData = m.get('styleData',{})
            lat = styleData.get('latitude','52.52')
            lon = styleData.get('longitude','13.41')
            wdata = fetch_weather(lat, lon)
            if not wdata:
                m['content'] = "No data"
            else:
                ws = styleData.get('weatherStyle','default')
                m['content'] = apply_weather_style(ws, wdata)
        elif m['type']=='timer':
            styleData = m.get('styleData', {})
            target_str = styleData.get('timerTarget', '2025-01-01 00:00:00')
            try:
                target_dt = datetime.strptime(target_str, "%Y-%m-%d %H:%M:%S")
                local_tz = get_localzone()
                # Wenn 'target_dt' offset-naive ist, setze es auf lokale Zeitzone
                if target_dt.tzinfo is None:
                    target_dt = target_dt.replace(tzinfo=local_tz)
                else:
                    target_dt = target_dt.astimezone(local_tz)
                now = datetime.now(local_tz)
                diff = target_dt - now
                if diff.total_seconds() < 0:
                    m['content'] = "Time's up!"
                else:
                    fmt = styleData.get('timerFormat', 'D days, HH:MM:SS')
                    days = diff.days
                    sec = diff.seconds
                    hours = sec // 3600
                    minutes = (sec % 3600) // 60
                    seconds = sec % 60
                    display = fmt.replace('D', str(days))
                    display = display.replace('HH', f"{hours:02d}")
                    display = display.replace('MM', f"{minutes:02d}")
                    display = display.replace('SS', f"{seconds:02d}")
                    m['content'] = display
            except Exception as e:
                logging.error(f"Error processing timer: {e}")
                m['content'] = "Invalid timer target"
        elif m['type'] == 'calendar':
            styleData = m.get('styleData', {})
            calendar_url = styleData.get('calendarURL', '')
            max_events = int(styleData.get('maxEvents', 5))
            events = fetch_calendar(calendar_url, max_events)
            if not events:
                m['content'] = "No events"
            else:
                formatted_events = "\n".join([f"{event['start'].strftime('%Y-%m-%d %H:%M')} - {event['summary']}" for event in events])
                m['content'] = formatted_events

    return jsonify(design)

@app.route('/designs', methods=['GET'])
def list_designs():
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    designs = []
    for fn in files:
        with open(os.path.join(DESIGN_FOLDER, fn), 'r') as f:
            d = json.load(f)
            d['filename'] = fn
            designs.append(d)
    return jsonify(designs)

@app.route('/set_active_design', methods=['POST'])
def set_active():
    data = request.json
    name = data.get('name')
    set_active_design(name)
    return jsonify({"message": "Active design set."})

@app.route('/clone_design', methods=['POST'])
def clone_design():
    data = request.json
    source_name = data.get('name')
    d = get_design_by_name(source_name)
    if not d:
        return jsonify({"message": "Design not found"}),404
    d['name'] = d['name'] + " (Clone)"
    d['timestamp'] = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")
    d['active'] = False
    new_fn = f"design_{d['timestamp']}.json"
    with open(os.path.join(DESIGN_FOLDER, new_fn), 'w') as fw:
        json.dump(d, fw)
    return jsonify({"message": "Design cloned"})

@app.route('/delete_design', methods=['POST'])
def delete_design():
    data = request.json
    name = data.get('name')
    files = [f for f in os.listdir(DESIGN_FOLDER) if f.endswith('.json')]
    found_file = None
    for fn in files:
        with open(os.path.join(DESIGN_FOLDER, fn), 'r') as f:
            dd = json.load(f)
            if dd['name'] == name:
                found_file = fn
                break
    if found_file:
        os.remove(os.path.join(DESIGN_FOLDER, found_file))
        set_active_design()
        return jsonify({"message": "Design deleted"})
    return jsonify({"message": "Design not found"}), 404

@app.route('/update_design', methods=['POST'])
def update_design():
    data = request.json
    save_as_new = data.get('save_as_new', False)
    designs_name = data.get('name', 'Unnamed Design')
    keep_alive = data.get('keep_alive', False)
    timestamp = datetime.now().strftime("%Y-%m-%d_%H-%M-%S")

    files = [f for f in os.listdir(app.config['DESIGN_FOLDER']) if f.endswith('.json')]

    if save_as_new:
        for fn in files:
            with open(os.path.join(app.config['DESIGN_FOLDER'], fn), 'r') as f:
                dd = json.load(f)
            dd['active'] = False
            with open(os.path.join(app.config['DESIGN_FOLDER'], fn), 'w') as f:
                json.dump(dd, f)

        data['timestamp'] = timestamp
        data['active'] = True
        data['name'] = designs_name
        data['keep_alive'] = keep_alive
        new_fn = f"design_{timestamp}.json"
        with open(os.path.join(app.config['DESIGN_FOLDER'], new_fn), 'w') as f:
            json.dump(data, f)

        return jsonify({"message": "New design saved and set active."})

    else:
        active_design, fn = get_active_design()
        if not active_design:
            return jsonify({"message": "No active design found."}), 404

        data['timestamp'] = active_design['timestamp']
        data['active'] = True
        data['name'] = designs_name
        data['keep_alive'] = keep_alive

        for afn in files:
            with open(os.path.join(app.config['DESIGN_FOLDER'], afn), 'r') as f:
                dd = json.load(f)
            dd['active'] = (afn == fn)
            with open(os.path.join(app.config['DESIGN_FOLDER'], afn), 'w') as f:
                json.dump(dd, f)

        with open(os.path.join(app.config['DESIGN_FOLDER'], fn), 'w') as f:
            json.dump(data, f)

        return jsonify({"message": "Design updated successfully!"})

@app.route('/designer', methods=['GET'])
def designer():
    return render_template('designer.html')

@app.route('/upload_image', methods=['POST'])
def upload_image():
    if 'file' not in request.files:
        return jsonify({"message": "No file part"}), 400
    file = request.files['file']
    filename = secure_filename(file.filename)
    if file and allowed_image_file(filename):
        # Speichern der Datei im Originalformat oder als PNG
        original_ext = os.path.splitext(filename)[1].lower()
        base_name = os.path.splitext(filename)[0]
        # Optional: Konvertiere alle Bilder zu PNG für einheitliches Format
        png_filename = base_name + ".png"
        filepath = os.path.join(app.config['UPLOAD_FOLDER'], png_filename)
        try:
            img = Image.open(file).convert("RGBA")
            img.save(filepath, "PNG")
        except Exception as e:
            return jsonify({"message": f"Image processing failed: {e}"}), 400
        return jsonify({"message": "File uploaded successfully!", "file_path": png_filename})
    elif file and allowed_font_file(filename):
        fontpath = os.path.join(app.config['FONT_FOLDER'], filename)
        file.save(fontpath)
        return jsonify({"message":"Font uploaded successfully!", "font_path": filename})
    return jsonify({"message": "Invalid file type!"}), 400

@app.route('/images_all', methods=['GET'])
def images_all():
    files = os.listdir(app.config['UPLOAD_FOLDER'])
    images = [f for f in files if f.lower().endswith('.png')]
    return jsonify(images)

@app.route('/fonts_all', methods=['GET'])
def fonts_all():
    files = os.listdir(app.config['FONT_FOLDER'])
    fonts = [f for f in files if f.lower().endswith('.ttf') or f.lower().endswith('.otf')]
    return jsonify(fonts)

@app.route('/image/<filename>', methods=['GET'])
def get_image(filename):
    filepath = os.path.join(app.config['UPLOAD_FOLDER'], filename)
    if os.path.exists(filepath):
        return send_file(filepath, mimetype='image/png')  # Ändere den MIME-Typ zu PNG
    return jsonify({"message": "File not found!"}), 404

@app.route('/font/<filename>', methods=['GET'])
def get_font(filename):
    fontpath = os.path.join(app.config['FONT_FOLDER'], filename)
    if os.path.exists(fontpath):
        return send_file(fontpath, mimetype='application/octet-stream')
    return jsonify({"message":"Font not found"}),404

@app.route('/delete_image', methods=['POST'])
def delete_image_route():
    data = request.json
    filename = data.get('filename')
    if not filename:
        return jsonify({"message":"No filename provided"}),400
    filepath = os.path.join(UPLOAD_FOLDER, filename)
    if os.path.exists(filepath):
        os.remove(filepath)
        return jsonify({"message":"Image deleted"})
    return jsonify({"message":"File not found"}),404

@app.route('/location_search', methods=['GET'])
def location_search():
    q = request.args.get('q','')
    if not q:
        return jsonify([])
    url = f"https://nominatim.openstreetmap.org/search?format=json&q={q}"
    r = requests.get(url, headers={"User-Agent":"Mozilla/5.0"})
    if r.status_code==200:
        data = r.json()
        results = []
        for item in data[:10]:
            results.append({
                "display_name": item.get('display_name',''),
                "lat": item.get('lat',''),
                "lon": item.get('lon','')
            })
        return jsonify(results)
    return jsonify([])

def download_font(font_name):
    fontpath = os.path.join(app.config['FONT_FOLDER'], font_name)
    if os.path.exists(fontpath):
        return fontpath
    return None

def render_text(draw, x, y, w, h, text, font, bold=False, italic=False, strike=False, align='left'):
    lines = text.split('\n')
    wrapped_lines=[]
    for line in lines:
        words = line.split(' ')
        cl = []
        cw=0
        for wo in words:
            ww=font.getlength(wo)
            if cl:
                if cw+font.getlength(' ') + ww <=w:
                    cl.append(wo)
                    cw += font.getlength(' ') + ww
                else:
                    wrapped_lines.append(' '.join(cl))
                    cl=[wo]
                    cw=ww
            else:
                if ww <=w:
                    cl=[wo]
                    cw=ww
                else:
                    cl=[wo]
                    cw=ww

        if cl:
            wrapped_lines.append(' '.join(cl))

    line_height = font.size+2
    iy=y
    for line in wrapped_lines:
        line_width = font.getlength(line)
        if align=='center':
            lx = x + (w - line_width)/2
        elif align=='right':
            lx = x + (w - line_width)
        else:
            lx = x
        if italic:
            lx +=1
        if iy + line_height > y + h:
            break
        if bold:
            draw.text((lx,iy), line, font=font, fill=0)
            draw.text((lx+1,iy), line, font=font, fill=0)
        else:
            draw.text((lx,iy), line, font=font, fill=0)
        if strike:
            draw.line((lx, iy + font.size/2, lx + line_width, iy + font.size/2), fill=0)
        iy += line_height

@app.route('/preview', methods=['GET'])
def preview_image():
    name = request.args.get('name', None)
    d = None
    if name:
        d = get_design_by_name(name)
        if not d:
            return jsonify({"message":"Design not found"}),404
    else:
        d,fn = get_active_design()
        if not d:
            return jsonify({"message":"No design"}),404

    for m in d.get('modules', []):
        if m['type']=='datetime':
            styleData = m.get('styleData',{})
            dt_fmt = styleData.get('datetimeFormat','YYYY-MM-DD HH:mm')
            current_dt = format_datetime(dt_fmt)
            m['content'] = current_dt
        elif m['type']=='weather':
            styleData = m.get('styleData',{})
            lat = styleData.get('latitude','52.52')
            lon = styleData.get('longitude','13.41')
            wdata = fetch_weather(lat, lon)
            if not wdata:
                m['content'] = "No data"
            else:
                ws = styleData.get('weatherStyle','default')
                m['content'] = apply_weather_style(ws, wdata)
        elif m['type']=='timer':
            styleData = m.get('styleData', {})
            target_str = styleData.get('timerTarget', '2025-01-01 00:00:00')
            try:
                target_dt = datetime.strptime(target_str, "%Y-%m-%d %H:%M:%S")
                local_tz = get_localzone()
                # Wenn 'target_dt' offset-naive ist, setze es auf lokale Zeitzone
                if target_dt.tzinfo is None:
                    target_dt = target_dt.replace(tzinfo=local_tz)
                else:
                    target_dt = target_dt.astimezone(local_tz)
                now = datetime.now(local_tz)
                diff = target_dt - now
                if diff.total_seconds()<0:
                    m['content'] = "Time's up!"
                else:
                    fmt = styleData.get('timerFormat','D days, HH:MM:SS')
                    days = diff.days
                    sec = diff.seconds
                    hours = sec // 3600
                    minutes = (sec % 3600) // 60
                    seconds = sec % 60
                    display = fmt.replace('D', str(days))
                    display = display.replace('HH', f"{hours:02d}")
                    display = display.replace('MM', f"{minutes:02d}")
                    display = display.replace('SS', f"{seconds:02d}")
                    m['content'] = display
            except Exception as e:
                logging.error(f"Error processing timer: {e}")
                m['content'] = "Invalid timer target"
        elif m['type'] == 'calendar':
            styleData = m.get('styleData', {})
            calendar_url = styleData.get('calendarURL', '')
            max_events = int(styleData.get('maxEvents', 5))
            events = fetch_calendar(calendar_url, max_events)
            if not events:
                m['content'] = "No events"
            else:
                formatted_events = "\n".join([f"{event['start'].strftime('%Y-%m-%d %H:%M')} - {event['summary']}" for event in events])
                m['content'] = formatted_events

    blackImage = Image.new('1', (EINK_WIDTH, EINK_HEIGHT), 255)
    draw_black = ImageDraw.Draw(blackImage)

    for m in d.get('modules', []):
        x = m['position']['x'] - EINK_OFFSET_X
        y = m['position']['y'] - EINK_OFFSET_Y
        w = m['size']['width']
        h = m['size']['height']
        content = m['content']
        t = m['type']
        styleData = m.get('styleData', {})

        if x + w < 0 or x > EINK_WIDTH or y + h < 0 or y > EINK_HEIGHT:
            continue

        font_name = styleData.get('font','')
        font_size = int(styleData.get('fontSize','18'))
        bold = (styleData.get('fontBold','false')=='true')
        italic = (styleData.get('fontItalic','false')=='true')
        strike = (styleData.get('fontStrike','false')=='true')
        align = styleData.get('textAlign','left')

        font_path = None
        if font_name:
            font_path = download_font(font_name)
        if not font_path:
            def_font = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
            if os.path.exists(def_font):
                font_path = def_font
            else:
                font_path = None

        if font_path:
            try:
                font = ImageFont.truetype(font_path, font_size)
            except:
                font = ImageFont.load_default()
        else:
            font = ImageFont.load_default()

        if t in ['text','news','weather','datetime','timer','calendar']:
            render_text(draw_black, x, y, w, h, content, font, bold=bold, italic=italic, strike=strike, align=align)
        elif t == 'image':
            img_name = styleData.get('image', None)
            if img_name:
                img_path = os.path.join(app.config['UPLOAD_FOLDER'], img_name)
                if os.path.exists(img_path):
                    imgi = Image.open(img_path).convert('1')
                    cx = int(styleData.get('crop_x',0))
                    cy = int(styleData.get('crop_y',0))
                    cw = int(styleData.get('crop_w',imgi.width))
                    ch = int(styleData.get('crop_h',imgi.height))
                    cropped = imgi.crop((cx,cy,cx+cw,cy+ch))
                    cropped = cropped.resize((w, h))
                    blackImage.paste(cropped, (x, y))
        elif t == 'line':
            draw_black.rectangle((x, y, x + w, y + h), fill=0)

    buf = io.BytesIO()
    blackImage.save(buf, format='PNG')
    buf.seek(0)
    return send_file(buf, mimetype='image/png')

@app.route('/update_settings', methods=['POST'])
def update_settings():
    return jsonify({"message":"Settings updated."})

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000, debug=True)