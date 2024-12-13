#!/usr/bin/python3
# -*- coding:utf-8 -*-
import os
import requests
import logging
from PIL import Image, ImageDraw, ImageFont
import json
import time
import subprocess
from datetime import datetime, timedelta
from waveshare_epd import epd7in5_V2  # Stellen Sie sicher, dass dieses Modul installiert ist

logging.basicConfig(level=logging.DEBUG)

BASE_URL = "http://127.0.0.1:5000"
DESIGN_PATH = "/design"
IMAGE_PATH = "/image/"
FONT_PATH = "/font/"

EINK_OFFSET_X = 200
EINK_OFFSET_Y = 160
EINK_WIDTH = 800
EINK_HEIGHT = 480

max_retries = 3
retry_delay = 20  # Sekunden

TEMP_DIR = "./temp_files/"  # Temporäres Verzeichnis für heruntergeladene Dateien
LOCAL_DESIGN_FILE = "./local_design.json"  # Lokale Datei zum Speichern des letzten bekannten Designs
OUTPUT_IMAGE_FILE = "output_image.bmp"  # Name der Ausgabedatei


def check_internet_connectivity():
    """
    Überprüft, ob das Internet erreichbar ist, indem eine zuverlässige externe IP angepingt wird.
    """
    try:
        subprocess.check_output(["ping", "-c", "1", "1.1.1.1"])
        logging.info("Internetverbindung bestätigt.")
        return True
    except subprocess.CalledProcessError:
        logging.warning("Keine Internetverbindung.")
        return False


def clean_temp_files():
    """
    Löscht alle temporären Dateien, einschließlich Schriftarten, Bilder und anderer nicht verwendeter Ressourcen.
    """
    if os.path.exists(TEMP_DIR):
        for file_name in os.listdir(TEMP_DIR):
            file_path = os.path.join(TEMP_DIR, file_name)
            try:
                os.remove(file_path)
                logging.info(f"Alte Datei gelöscht: {file_path}")
            except Exception as e:
                logging.error(f"Fehler beim Löschen von {file_path}: {e}")


def fetch_design(base_url):
    """
    Holt das Design-JSON vom Server und speichert es lokal.
    """
    try:
        url = base_url + DESIGN_PATH
        response = requests.get(url, timeout=5)
        if response.status_code == 200:
            design_data = response.json()
            with open(LOCAL_DESIGN_FILE, "w") as file:
                json.dump(design_data, file)
            logging.info("Design erfolgreich abgerufen und gespeichert.")
            return design_data
        else:
            logging.error(f"Fehler beim Abrufen des Designs. HTTP-Status: {response.status_code}.")
            return None
    except requests.RequestException as e:
        logging.error(f"Fehler beim Abrufen des Designs: {e}")
        return None


def load_local_design():
    """
    Lädt das zuletzt bekannte Design aus der lokalen Datei, falls der Server nicht erreichbar ist.
    """
    if os.path.exists(LOCAL_DESIGN_FILE):
        with open(LOCAL_DESIGN_FILE, "r") as file:
            logging.info("Lokale Designdaten geladen.")
            return json.load(file)
    logging.error("Kein lokales Design verfügbar.")
    return None


def update_datetime_fields(design):
    """
    Aktualisiert alle Datums- und Uhrzeitfelder im Design mit der aktuellen Systemzeit, wenn offlineClientSync aktiviert ist
    und keine Serververbindung besteht.
    """
    for module in design.get('modules', []):
        if module.get('type') == 'datetime':
            styleData = module.get('styleData', {})
            format_string = styleData.get('datetimeFormat', '%Y-%m-%d %H:%M:%S')
            # Platzhalter ersetzen
            format_string = format_string.replace("YYYY", "%Y").replace("MM", "%m").replace("DD", "%d")
            format_string = format_string.replace("HH", "%H").replace("mm", "%M").replace("ss", "%S")

            # Wenn offlineClientSync wahr ist, die Zeit vom lokalen System aktualisieren
            if styleData.get('offlineClientSync', 'false') == 'true':
                current_time = datetime.now().strftime(format_string)
                module['content'] = current_time
                logging.info(f"Datum und Uhrzeit aktualisiert auf {current_time}")
    return design


def update_timer_fields(design):
    """
    Aktualisiert alle Timerfelder im Design mit dem aktuellen Countdown, wenn offlineClientSync aktiviert ist
    und keine Serververbindung besteht. timerTarget wird als 'YYYY-MM-DD HH:mm:ss' in styleData erwartet.
    timerFormat könnte etwas wie 'D days, HH:MM:SS' sein.
    """
    for module in design.get('modules', []):
        if module.get('type') == 'timer':
            styleData = module.get('styleData', {})
            target_str = styleData.get('timerTarget', '2025-01-01 00:00:00')
            # offlineClientSync überprüfen
            if styleData.get('offlineClientSync', 'false') == 'true':
                try:
                    target_dt = datetime.strptime(target_str, "%Y-%m-%d %H:%M:%S")
                    now = datetime.now()
                    diff = target_dt - now
                    if diff.total_seconds() < 0:
                        # Timer beendet
                        module['content'] = "Zeit abgelaufen!"
                    else:
                        # Differenz basierend auf timerFormat formatieren
                        fmt = styleData.get('timerFormat', 'D days, HH:MM:SS')
                        days = diff.days
                        sec = diff.seconds
                        hours = sec // 3600
                        minutes = (sec % 3600) // 60
                        seconds = sec % 60

                        # Platzhalter ersetzen
                        display = fmt.replace('D', str(days))
                        display = display.replace('HH', f"{hours:02d}")
                        display = display.replace('MM', f"{minutes:02d}")
                        display = display.replace('SS', f"{seconds:02d}")
                        module['content'] = display
                        logging.info(f"Timer aktualisiert auf {display}")
                except Exception as e:
                    logging.error(f"Fehler beim Aktualisieren des Timers: {e}")
    return design


def fetch_weather_data(lat, lon):
    """
    Holt Wetterdaten direkt von open-meteo, wenn offlineClientSync aktiviert ist und wir Internet haben,
    aber der Server nicht erreichbar ist.
    """
    url = f"https://api.open-meteo.com/v1/forecast?latitude={lat}&longitude={lon}&hourly=temperature_2m,weathercode,precipitation&daily=weathercode,temperature_2m_max,temperature_2m_min,sunrise,sunset&current_weather=true&forecast_days=4&timezone=Europe%2FBerlin"
    try:
        r = requests.get(url, timeout=5)
        if r.status_code == 200:
            data = r.json()
            # Vereinfachte Darstellung der Wetterdaten
            if 'current_weather' in data:
                temp = data['current_weather']['temperature']
                return f"Temp: {temp}°C"
            else:
                return "Keine Wetterdaten"
        else:
            return "Keine Wetterdaten"
    except:
        return "Keine Wetterdaten"


def download_file(base_url, path, file_name):
    """
    Lädt eine Datei (Bild oder Schriftart) vom Server herunter und speichert sie im temporären Verzeichnis.
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
            logging.info(f"Datei {file_name} heruntergeladen und gespeichert.")
            return file_path
    except requests.RequestException as e:
        logging.error(f"Fehler beim Herunterladen der Datei {file_name}: {e}")
    return None


def render_text(draw, x, y, w, h, text, font, color=(0, 0, 0), bold=False, italic=False, strike=False, align='left'):
    lines = text.split('\n')
    wrapped_lines = []
    for line in lines:
        words = line.split(' ')
        cl = []
        cw = 0
        for word in words:
            ww, _ = font.getsize(word)
            space_width, _ = font.getsize(' ')
            if cl:
                if cw + space_width + ww <= w:
                    cl.append(word)
                    cw += space_width + ww
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

    _, line_height = font.getsize('A')  # Verwendung einer Referenzzeile für die Höhe
    line_height += 2
    for line in wrapped_lines:
        line_width, _ = font.getsize(line)
        if align == 'center':
            lx = x + (w - line_width) / 2
        elif align == 'right':
            lx = x + (w - line_width)
        else:
            lx = x
        draw.text((lx, y), line, font=font, fill=color)
        if strike:
            draw.line((lx, y + line_height / 2, lx + line_width, y + line_height / 2), fill=color)
        y += line_height


def main():
    retry_count = 0  # Lokale Deklaration von retry_count
    design = None    # Initialisierung von design
    server_design = None  # Initialisierung von server_design

    internet_available = check_internet_connectivity()

    # Versuch, das Design vom Server abzurufen
    while retry_count < max_retries:
        server_design = fetch_design(BASE_URL)
        if server_design:
            design = server_design
            clean_temp_files()
            break
        else:
            logging.warning(f"Versuch {retry_count + 1} fehlgeschlagen. Erneuter Versuch in {retry_delay} Sekunden.")
            retry_count += 1
            time.sleep(retry_delay)

    # Falls das Design nach den Retries immer noch None ist, lade es lokal
    if not design:
        logging.warning("Server nicht erreichbar. Verwende lokale Designdaten.")
        design = load_local_design()

    # Wenn immer noch kein Design verfügbar ist, Programm beenden
    if not design:
        logging.error("Keine Designdaten verfügbar. Beende das Programm.")
        return

    # Prüfe die Module und wende Offline-Logik an, falls nötig
    server_reachable = (server_design is not None)

    for m in design.get('modules', []):
        t = m['type']
        styleData = m.get('styleData', {})
        offlineSync = (styleData.get('offlineClientSync', 'false').lower() == 'true')

        if t == 'weather':
            # Wenn offlineClientSync aktiviert ist und der Server nicht erreichbar ist, aber Internet verfügbar ist, hole Wetterdaten
            if offlineSync and (not server_reachable) and internet_available:
                lat = styleData.get('latitude', '52.52')
                lon = styleData.get('longitude', '13.41')
                m['content'] = fetch_weather_data(lat, lon)

        # Weitere Typen können hier behandelt werden

    # Aktualisiere Datums- und Uhrzeitfelder im Design (wenn offline und offlineClientSync aktiviert)
    design = update_datetime_fields(design)
    # Aktualisiere Timerfelder im Design
    design = update_timer_fields(design)

    # Erstelle ein farbiges Bild
    colorImage = Image.new('RGB', (EINK_WIDTH, EINK_HEIGHT), (255, 255, 255))
    draw_color = ImageDraw.Draw(colorImage)

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
        bold = (styleData.get('fontBold', 'false').lower() == 'true')
        italic = (styleData.get('fontItalic', 'false').lower() == 'true')
        strike = (styleData.get('fontStrike', 'false').lower() == 'true')
        align = styleData.get('textAlign', 'left')
        text_color = styleData.get('textColor', '#000000')  # Standardfarbe Schwarz

        # Konvertiere Hex-Farbe zu RGB
        try:
            if text_color.startswith('#') and len(text_color) == 7:
                color = tuple(int(text_color[i:i+2], 16) for i in (1, 3, 5))
            else:
                color = (0, 0, 0)  # Fallback zu Schwarz
        except:
            color = (0, 0, 0)

        if font_name:
            font_path = download_file(BASE_URL, FONT_PATH, font_name) if server_reachable else os.path.join(TEMP_DIR, font_name)
            if not font_path or not os.path.exists(font_path):
                # Fallback
                font_path = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
                if not os.path.exists(font_path):
                    font_path = None
        else:
            font_path = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
            if not os.path.exists(font_path):
                font_path = None

        try:
            font = ImageFont.truetype(font_path, font_size) if font_path else ImageFont.load_default()
        except Exception as e:
            logging.error(f"Fehler beim Laden der Schriftart: {e}")
            font = ImageFont.load_default()

        if t in ['text', 'news', 'weather', 'datetime', 'timer']:
            render_text(draw_color, x, y, w, h, content, font, color=color, bold=bold, italic=italic, strike=strike, align=align)
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
                        # Kein Server und kein lokales Bild - überspringen
                        logging.warning(f"Bild {img_name} nicht gefunden. Überspringe Modul.")
                if img_path and os.path.exists(img_path):
                    try:
                        img = Image.open(img_path).convert('RGB')
                        cx = int(styleData.get('crop_x', 0))
                        cy = int(styleData.get('crop_y', 0))
                        cw = int(styleData.get('crop_w', img.width))
                        ch = int(styleData.get('crop_h', img.height))
                        img = img.crop((cx, cy, cx + cw, cy + ch))
                        img = img.resize((w, h))
                        colorImage.paste(img, (x, y))
                    except Exception as e:
                        logging.error(f"Fehler beim Verarbeiten des Bildes {img_name}: {e}")

    # Speichere das Bild als BMP
    try:
        colorImage.save(OUTPUT_IMAGE_FILE, format='BMP')
        logging.info(f"Bild gespeichert als {OUTPUT_IMAGE_FILE}")
    except Exception as e:
        logging.error(f"Fehler beim Speichern des Bildes: {e}")
        return

    # Sende das Bild an das E-Ink-Display
    try:
        epd = epd7in5_V2.EPD()
        epd.init()
        epd.Clear()

        # Lade das Bild (800x480 Pixel, farbig)
        image = Image.open(OUTPUT_IMAGE_FILE)
        epd.display(epd.getbuffer(image))

        # Optional: Das Display schlafen legen
        epd.sleep()
        logging.info("Bild erfolgreich an das E-Ink-Display gesendet.")
    except IOError as e:
        logging.error(f"IOError: {e}")
    except KeyboardInterrupt:
        epd7in5_V2.epdconfig.module_exit()
        logging.info("Programm durch Benutzer unterbrochen.")
        exit()
    except Exception as e:
        logging.error(f"Unerwarteter Fehler: {e}")


if __name__ == '__main__':
    main()
