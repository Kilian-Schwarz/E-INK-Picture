#!/usr/bin/python
# -*- coding:utf-8 -*-
import os
import requests
import logging
from PIL import Image, ImageDraw, ImageFont
import json
import subprocess
from datetime import datetime
from datetime import timedelta

logging.basicConfig(level=logging.DEBUG)

BASE_URL = "http://127.0.0.1:5000"
DESIGN_PATH = "/design"
IMAGE_PATH = "/image/"
FONT_PATH = "/font/"

EINK_OFFSET_X = 200
EINK_OFFSET_Y = 160
EINK_WIDTH = 800
EINK_HEIGHT = 480
TEMP_DIR = "./temp_files/"  # Temporary directory for downloaded files
LOCAL_DESIGN_FILE = "./local_design.json"  # Local file to save last known design


def check_internet_connectivity():
    """
    Check if the internet is reachable by pinging a reliable external IP.
    """
    try:
        subprocess.check_output(["ping", "-c", "1", "1.1.1.1"])
        logging.info("Internet connectivity verified.")
        return True
    except subprocess.CalledProcessError:
        logging.warning("No internet connection.")
        return False


def clean_temp_files():
    """
    Delete all temporary files, including fonts, images, and other unused resources.
    """
    if os.path.exists(TEMP_DIR):
        for file_name in os.listdir(TEMP_DIR):
            file_path = os.path.join(TEMP_DIR, file_name)
            try:
                os.remove(file_path)
                logging.info(f"Deleted old file: {file_path}")
            except Exception as e:
                logging.error(f"Failed to delete {file_path}: {e}")


def fetch_design(base_url):
    """
    Fetch the design JSON from the server and save it locally.
    """
    try:
        url = base_url + DESIGN_PATH
        response = requests.get(url, timeout=5)
        if response.status_code == 200:
            design_data = response.json()
            with open(LOCAL_DESIGN_FILE, "w") as file:
                json.dump(design_data, file)
            logging.info("Design fetched and saved successfully.")
            return design_data
        else:
            logging.error(f"Failed to fetch design. HTTP status: {response.status_code}.")
            return None
    except requests.RequestException as e:
        logging.error(f"Error while fetching design: {e}")
        return None


def load_local_design():
    """
    Load the last known design from the local file if the server is unreachable.
    """
    if os.path.exists(LOCAL_DESIGN_FILE):
        with open(LOCAL_DESIGN_FILE, "r") as file:
            logging.info("Local design data loaded.")
            return json.load(file)
    logging.error("No local design available.")
    return None


def update_datetime_fields(design):
    """
    Update all datetime fields in the design with the current system time if offlineClientSync is enabled
    and no server connectivity.
    """
    for module in design.get('modules', []):
        if module.get('type') == 'datetime':
            styleData = module.get('styleData', {})
            format_string = styleData.get('datetimeFormat', '%Y-%m-%d %H:%M:%S')
            # Replace placeholders
            format_string = format_string.replace("YYYY", "%Y").replace("MM", "%m").replace("DD", "%d")
            format_string = format_string.replace("HH", "%H").replace("mm", "%M").replace("ss", "%S")

            # If offlineClientSync is true, we can update the time from local system if server not available
            if styleData.get('offlineClientSync', 'false') == 'true':
                current_time = datetime.now().strftime(format_string)
                module['content'] = current_time
                logging.info(f"Updated datetime to {current_time}")
    return design


def update_timer_fields(design):
    """
    Update all timer fields in the design with the current countdown if offlineClientSync is enabled
    and no server connectivity. timerTarget expected as 'YYYY-MM-DD HH:mm:ss' in styleData.
    timerFormat might be something like 'D days, HH:MM:SS'
    """
    for module in design.get('modules', []):
        if module.get('type') == 'timer':
            styleData = module.get('styleData', {})
            target_str = styleData.get('timerTarget', '2025-01-01 00:00:00')
            # offlineClientSync check
            if styleData.get('offlineClientSync', 'false') == 'true':
                try:
                    target_dt = datetime.strptime(target_str, "%Y-%m-%d %H:%M:%S")
                    now = datetime.now()
                    diff = target_dt - now
                    if diff.total_seconds() < 0:
                        # Timer finished
                        module['content'] = "Time's up!"
                    else:
                        # Format the difference based on timerFormat
                        fmt = styleData.get('timerFormat', 'D days, HH:MM:SS')
                        days = diff.days
                        sec = diff.seconds
                        hours = sec // 3600
                        minutes = (sec % 3600) // 60
                        seconds = sec % 60

                        # Replace placeholders
                        display = fmt.replace('D', str(days))
                        display = display.replace('HH', f"{hours:02d}")
                        display = display.replace('MM', f"{minutes:02d}")
                        display = display.replace('SS', f"{seconds:02d}")
                        module['content'] = display
                        logging.info(f"Updated timer to {display}")
                except Exception as e:
                    logging.error(f"Error updating timer: {e}")
    return design


def fetch_weather_data(lat, lon):
    """
    Fetch weather data directly from open-meteo if offlineClientSync is enabled and we have internet,
    but server is not reachable.
    """
    url = f"https://api.open-meteo.com/v1/forecast?latitude={lat}&longitude={lon}&hourly=temperature_2m,weathercode,precipitation&daily=weathercode,temperature_2m_max,temperature_2m_min,sunrise,sunset&current_weather=true&forecast_days=4&timezone=Europe%2FBerlin"
    try:
        r = requests.get(url, timeout=5)
        if r.status_code == 200:
            data = r.json()
            # Just put something basic as content here or a simplified style:
            # This is a simplified fallback if server unreachable
            if 'current_weather' in data:
                temp = data['current_weather']['temperature']
                return f"Temp: {temp}°C"
            else:
                return "No weather data"
        else:
            return "No weather data"
    except:
        return "No weather data"


def download_file(base_url, path, file_name):
    """
    Download a file (image or font) from the server and save it in the temporary directory.
    """
    if not os.path.exists(TEMP_DIR):
        os.makedirs(TEMP_DIR)

    file_path = os.path.join(TEMP_DIR, file_name)
    if os.path.exists(file_path):
        return file_path

    try:
        file_url = base_url + path + file_name
        response = requests.get(file_url, stream=True)
        if response.status_code == 200:
            with open(file_path, "wb") as f:
                f.write(response.content)
            logging.info(f"File {file_name} downloaded and saved.")
            return file_path
    except requests.RequestException as e:
        logging.error(f"Error while downloading file {file_name}: {e}")
    return None


def render_text(draw, x, y, w, h, text, font, bold=False, italic=False, strike=False, align='left'):
    lines = text.split('\n')
    wrapped_lines = []
    for line in lines:
        words = line.split(' ')
        cl = []
        cw = 0
        for word in words:
            ww = font.getlength(word)
            if cl:
                if cw + font.getlength(' ') + ww <= w:
                    cl.append(word)
                    cw += font.getlength(' ') + ww
                else:
                    wrapped_lines.append(' '.join(cl))
                    cl = [word]
                    cw = ww
            else:
                if ww <= w:
                    cl = [word]
                    cw = ww
        if cl:
            wrapped_lines.append(' '.join(cl))

    line_height = font.size + 2
    for line in wrapped_lines:
        line_width = font.getlength(line)
        if align == 'center':
            lx = x + (w - line_width) / 2
        elif align == 'right':
            lx = x + (w - line_width)
        else:
            lx = x
        draw.text((lx, y), line, font=font, fill=0)
        if strike:
            draw.line((lx, y + font.size / 2, lx + line_width, y + font.size / 2), fill=0)
        y += line_height


def main():
    internet_available = check_internet_connectivity()

    # Try to fetch design from server
    design = None
    server_design = fetch_design(BASE_URL)
    if server_design:
        design = server_design
        clean_temp_files()
    else:
        # Server unreachable, load local
        logging.warning("Server unreachable. Using local design data.")
        design = load_local_design()

    if not design:
        logging.error("No design data available. Exiting.")
        return

    # If server unreachable and we have internet, and offlineClientSync for weather modules is true, update weather from local.
    # If no internet but offlineClientSync for datetime or timer is true, update them from local system.
    # Actually, we already do datetime and timer updates from local if offlineClientSync is enabled.

    # Check modules and apply offline logic if needed
    server_reachable = (server_design is not None)

    for m in design.get('modules', []):
        t = m['type']
        styleData = m.get('styleData', {})
        offlineSync = (styleData.get('offlineClientSync', 'false') == 'true')

        if t == 'weather':
            # If offlineClientSync is enabled and no server but internet available, fetch from open-meteo.
            if offlineSync and (not server_reachable) and internet_available:
                lat = styleData.get('latitude', '52.52')
                lon = styleData.get('longitude', '13.41')
                m['content'] = fetch_weather_data(lat, lon)

        # datetime and timer are updated after design loaded, if offlineClientSync is enabled
        # datetime handled by update_datetime_fields
        # timer handled by update_timer_fields

    # Update datetime fields in the design (if offline and offlineClientSync)
    design = update_datetime_fields(design)
    # Update timer fields in the design
    design = update_timer_fields(design)

    blackImage = Image.new('1', (EINK_WIDTH, EINK_HEIGHT), 255)
    draw_black = ImageDraw.Draw(blackImage)

    for m in design.get('modules', []):
        x = m['position']['x'] - EINK_OFFSET_X
        y = m['position']['y'] - EINK_OFFSET_Y
        w = m['size']['width']
        h = m['size']['height']
        content = m['content']
        t = m['type']
        styleData = m.get('styleData', {})

        if x + w < 0 or x > EINK_WIDTH or y + h < 0 or y > EINK_HEIGHT:
            continue

        font_name = styleData.get('font', '').strip()
        font_size = int(styleData.get('fontSize', '18'))
        bold = (styleData.get('fontBold', 'false') == 'true')
        italic = (styleData.get('fontItalic', 'false') == 'true')
        strike = (styleData.get('fontStrike', 'false') == 'true')
        align = styleData.get('textAlign', 'left')

        if font_name:
            font_path = download_file(BASE_URL, FONT_PATH, font_name) if server_reachable else os.path.join(TEMP_DIR, font_name)
            if not font_path or not os.path.exists(font_path):
                # fallback
                font_path = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
                if not os.path.exists(font_path):
                    font_path = None
        else:
            font_path = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
            if not os.path.exists(font_path):
                font_path = None

        font = ImageFont.truetype(font_path, font_size) if font_path else ImageFont.load_default()

        if t in ['text', 'news', 'weather', 'datetime', 'timer']:
            render_text(draw_black, x, y, w, h, content, font, bold=bold, italic=italic, strike=strike, align=align)
        elif t == 'image':
            img_name = styleData.get('image', None)
            if img_name:
                img_path = None
                if server_reachable:
                    img_path = download_file(BASE_URL, IMAGE_PATH, img_name)
                else:
                    local_path = os.path.join(TEMP_DIR, img_name)
                    if os.path.exists(local_path):
                        img_path = local_path
                    else:
                        # no server and no local image - skip
                        pass
                if img_path and os.path.exists(img_path):
                    img = Image.open(img_path).convert('1')
                    cx = int(styleData.get('crop_x', 0))
                    cy = int(styleData.get('crop_y', 0))
                    cw = int(styleData.get('crop_w', img.width))
                    ch = int(styleData.get('crop_h', img.height))
                    img = img.crop((cx, cy, cx + cw, cy + ch))
                    img = img.resize((w, h))
                    blackImage.paste(img, (x, y))

    blackImage.save('output_black_and_white.png')
    logging.info("Image saved as output_black_and_white.png")


if __name__ == '__main__':
    main()