# Verwenden Sie das offizielle Python-Image als Basis
FROM python:3.11-slim

# Setzen Sie die Zeitzone als Build-Argument (optional)
ARG TZ=Europe/Berlin

# Setzen Sie Umgebungsvariablen
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1
ENV TZ=${TZ}

# Setzen Sie das Arbeitsverzeichnis
WORKDIR /app

# Installieren Sie Systemabhängigkeiten und tzdata
RUN apt-get update && apt-get install -y \
    build-essential \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Setzen Sie die Zeitzone
RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime && echo $TZ > /etc/timezone

# Installieren Sie Python-Abhängigkeiten
COPY requirements.txt .
RUN pip install --upgrade pip
RUN pip install --no-cache-dir -r requirements.txt

# Kopieren Sie den Anwendungscode
COPY app/ /app/

# Exponieren Sie den Port, auf dem Flask läuft
EXPOSE 5000

# Erstellen Sie notwendige Verzeichnisse mit entsprechenden Berechtigungen
RUN mkdir -p /app/uploaded_images /app/designs /app/fonts /app/weather_styles

# Setzen Sie Umgebungsvariablen für Flask
ENV FLASK_APP=server.py
ENV FLASK_RUN_HOST=0.0.0.0
ENV FLASK_RUN_PORT=5000

# (Optional) Wenn Sie einen Benutzer haben, setzen Sie ihn hier für bessere Sicherheit
# RUN useradd -m myuser
# USER myuser

# Starten Sie den Flask-Server mit Gunicorn und konfigurieren Sie die Logs
CMD ["gunicorn", "--bind", "0.0.0.0:5000", "--access-logfile", "-", "--error-logfile", "-", "server:app"]