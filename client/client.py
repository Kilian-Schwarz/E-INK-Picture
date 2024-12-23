#!/bin/bash

echo "🚀 Starte vollständiges Setup inklusive aller Dateien, Funktionen und Default-Admin!"

# Überprüfen auf sudo/root
if [ "$EUID" -ne 0 ]; then
  echo "Bitte führe dieses Skript als root oder mit sudo aus."
  exit 1
fi

# Systempakete aktualisieren
echo "🔄 Aktualisiere Systempakete..."
apt update && apt upgrade -y

# Installiere erforderliche Programme
echo "🐍 Installiere Python, pip und andere Pakete..."
apt install -y python3 python3-pip python3-venv wget git

# Projektstruktur erstellen
echo "📂 Erstelle Projektstruktur..."
mkdir -p /opt/webapp
cd /opt/webapp

mkdir -p app/templates app/static/css app/static/js app/geoip backups migrations
touch app/__init__.py app/models.py app/routes.py config.py run.py requirements.txt
touch app/templates/base.html app/templates/index.html app/templates/login.html app/templates/register.html
touch app/templates/dashboard.html app/templates/edit_domain.html app/templates/delete_confirm.html
touch app/static/css/styles.css app/static/js/scripts.js

# GeoIP-Datenbank herunterladen (optional, mit Fehlerprüfung)
echo "🌍 Lade GeoIP-Datenbank herunter (optional)..."
# Hinweis: Aktualisierte URL mit Lizenzschlüssel erforderlich
# DL_URL="https://geolite.maxmind.com/download/geoip/database/GeoLite2-City.mmdb"
# Beispiel für neue URL mit Lizenzschlüssel:
# DL_URL="https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-City&license_key=YOUR_LICENSE_KEY&suffix=tar.gz"

DL_URL="https://geolite.maxmind.com/download/geoip/database/GeoLite2-City.mmdb"

if wget -q --spider "$DL_URL"; then
  wget -O app/geoip/GeoLite2-City.mmdb "$DL_URL"
  echo "✅ GeoIP-Datenbank erfolgreich heruntergeladen."
else
  echo "⚠️  Konnte $DL_URL nicht erreichen (DNS oder veraltet?)."
  echo "⚠️  Bitte ggf. URL / Lizenzschlüssel anpassen, siehe MaxMind-Doku!"
fi

# Anforderungen für Python erstellen (cachelib nun auf 0.13.0)
echo "📦 Generiere requirements.txt..."
cat <<EOF > requirements.txt
Flask==2.3.3
Flask-SQLAlchemy==3.0.5
Flask-Login==0.6.3
Flask-Caching==1.11.0
Flask-Limiter==3.0.0
geoip2==4.7.0
cachelib==0.13.0
EOF

# Beispielkonfiguration erstellen
echo "⚙️ Generiere config.py..."
cat <<EOF > config.py
import os

BASE_DIR = os.path.abspath(os.path.dirname(__file__))

class Config:
    SECRET_KEY = os.environ.get('SECRET_KEY', 'default_secret_key')
    SQLALCHEMY_DATABASE_URI = 'sqlite:///' + os.path.join(BASE_DIR, 'app.db')
    SQLALCHEMY_TRACK_MODIFICATIONS = False
EOF

# Beispielcode für app/__init__.py erstellen
echo "📝 Generiere app/__init__.py..."
cat <<EOF > app/__init__.py
from flask import Flask
from flask_sqlalchemy import SQLAlchemy
from flask_caching import Cache
from flask_login import LoginManager

app = Flask(__name__)
app.config.from_object('config.Config')

db = SQLAlchemy(app)
cache = Cache(app, config={'CACHE_TYPE': 'SimpleCache'})
login_manager = LoginManager(app)
login_manager.login_view = "login"

from app import routes, models

# user_loader Funktion hinzufügen
@login_manager.user_loader
def load_user(user_id):
    from app.models import User
    return User.query.get(int(user_id))
EOF

# Beispielcode für app/models.py erstellen
echo "📝 Generiere app/models.py..."
cat <<EOF > app/models.py
from app import db
from flask_login import UserMixin
from werkzeug.security import generate_password_hash, check_password_hash

class User(UserMixin, db.Model):
    id = db.Column(db.Integer, primary_key=True)
    username = db.Column(db.String(150), nullable=False, unique=True)
    password_hash = db.Column(db.String(150), nullable=False)

    def set_password(self, password):
        self.password_hash = generate_password_hash(password)

    def check_password(self, password):
        return check_password_hash(self.password_hash, password)

class Domain(db.Model):
    id = db.Column(db.Integer, primary_key=True)
    domain_name = db.Column(db.String(255), nullable=False)
    alias_name = db.Column(db.String(255), nullable=False)
EOF

# Beispielcode für app/routes.py erstellen
echo "📝 Generiere app/routes.py..."
cat <<EOF > app/routes.py
from flask import render_template, request, redirect, url_for, flash
from flask_login import login_user, logout_user, login_required, current_user
from app import app, db
from app.models import Domain, User

# Login- und Logout-Funktionen
@app.route('/login', methods=['GET', 'POST'])
def login():
    if request.method == 'POST':
        username = request.form.get('username')
        password = request.form.get('password')
        user = User.query.filter_by(username=username).first()
        if user and user.check_password(password):
            login_user(user)
            return redirect(url_for('dashboard'))
        flash('Login fehlgeschlagen. Überprüfen Sie Benutzername und Passwort.')
    return render_template('login.html')

@app.route('/logout')
@login_required
def logout():
    logout_user()
    return redirect(url_for('login'))

# Dashboard anzeigen
@app.route('/dashboard')
@login_required
def dashboard():
    domains = Domain.query.all()
    return render_template('dashboard.html', domains=domains)

# Domain hinzufügen
@app.route('/add', methods=['GET', 'POST'])
@login_required
def add_domain():
    if request.method == 'POST':
        domain_name = request.form.get('domain_name')
        alias_name = request.form.get('alias_name')
        new_domain = Domain(domain_name=domain_name, alias_name=alias_name)
        db.session.add(new_domain)
        db.session.commit()
        flash('Domain erfolgreich hinzugefügt!')
        return redirect(url_for('dashboard'))
    return render_template('add_domain.html')

# Domain bearbeiten
@app.route('/edit/<int:id>', methods=['GET', 'POST'])
@login_required
def edit_domain(id):
    domain = Domain.query.get_or_404(id)
    if request.method == 'POST':
        domain.domain_name = request.form.get('domain_name')
        domain.alias_name = request.form.get('alias_name')
        db.session.commit()
        flash('Domain erfolgreich aktualisiert!')
        return redirect(url_for('dashboard'))
    return render_template('edit_domain.html', domain=domain)

# Domain löschen
@app.route('/delete/<int:id>', methods=['POST'])
@login_required
def delete_domain(id):
    domain = Domain.query.get_or_404(id)
    db.session.delete(domain)
    db.session.commit()
    flash('Domain erfolgreich gelöscht!')
    return redirect(url_for('dashboard'))

# Startseite Route hinzufügen
@app.route('/')
def index():
    return render_template('index.html')
EOF

# HTML-Templates generieren
echo "📝 Generiere HTML-Templates..."
cat <<EOF > app/templates/login.html
{% extends "base.html" %}
{% block content %}
<h2>Login</h2>
<form method="POST">
    <input type="text" name="username" placeholder="Benutzername" required>
    <input type="password" name="password" placeholder="Passwort" required>
    <button type="submit">Login</button>
</form>
{% endblock %}
EOF

cat <<EOF > app/templates/dashboard.html
{% extends "base.html" %}
{% block content %}
<h2>Willkommen, {{ current_user.username }}!</h2>
<a href="{{ url_for('add_domain') }}">Domain hinzufügen</a>
<h3>Domain-Liste</h3>
<table>
    <tr>
        <th>Domain</th>
        <th>Alias</th>
        <th>Aktionen</th>
    </tr>
    {% for domain in domains %}
    <tr>
        <td>{{ domain.domain_name }}</td>
        <td>{{ domain.alias_name }}</td>
        <td>
            <a href="{{ url_for('edit_domain', id=domain.id) }}">Bearbeiten</a>
            <form action="{{ url_for('delete_domain', id=domain.id) }}" method="POST" style="display:inline;">
                <button type="submit">Löschen</button>
            </form>
        </td>
    </tr>
    {% endfor %}
</table>
{% endblock %}
EOF

cat <<EOF > app/templates/add_domain.html
{% extends "base.html" %}
{% block content %}
<h2>Domain hinzufügen</h2>
<form method="POST">
    <input type="text" name="domain_name" placeholder="Domain Name" required>
    <input type="text" name="alias_name" placeholder="Alias Name" required>
    <button type="submit">Hinzufügen</button>
</form>
{% endblock %}
EOF

cat <<EOF > app/templates/edit_domain.html
{% extends "base.html" %}
{% block content %}
<h2>Domain bearbeiten</h2>
<form method="POST">
    <input type="text" name="domain_name" value="{{ domain.domain_name }}" required>
    <input type="text" name="alias_name" value="{{ domain.alias_name }}" required>
    <button type="submit">Speichern</button>
</form>
{% endblock %}
EOF

cat <<EOF > app/templates/delete_confirm.html
{% extends "base.html" %}
{% block content %}
<h2>Domain löschen</h2>
<p>Möchten Sie die Domain "{{ domain.domain_name }}" wirklich löschen?</p>
<form method="POST">
    <button type="submit">Ja, löschen</button>
    <a href="{{ url_for('dashboard') }}">Abbrechen</a>
</form>
{% endblock %}
EOF

# Basis-Template (base.html) und ggf. index.html
cat <<EOF > app/templates/base.html
<!DOCTYPE html>
<html lang="de">
<head>
    <meta charset="UTF-8">
    <title>WebApp</title>
    <link rel="stylesheet" href="{{ url_for('static', filename='css/styles.css') }}">
</head>
<body>
<header>
    <h1>Meine WebApp</h1>
    {% if current_user.is_authenticated %}
    <nav>
        <a href="{{ url_for('dashboard') }}">Dashboard</a>
        <a href="{{ url_for('logout') }}">Logout</a>
    </nav>
    {% else %}
    <nav>
        <a href="{{ url_for('login') }}">Login</a>
    </nav>
    {% endif %}
</header>
<main>
    {% with messages = get_flashed_messages() %}
      {% if messages %}
        <ul>
        {% for message in messages %}
          <li>{{ message }}</li>
        {% endfor %}
        </ul>
      {% endif %}
    {% endwith %}
    {% block content %}{% endblock %}
</main>
<script src="{{ url_for('static', filename='js/scripts.js') }}"></script>
</body>
</html>
EOF

cat <<EOF > app/templates/index.html
{% extends "base.html" %}
{% block content %}
<h2>Startseite</h2>
<p>Willkommen auf der Startseite</p>
{% endblock %}
EOF

# CSS und JS generieren
echo "🎨 Generiere CSS- und JS-Dateien..."
cat <<EOF > app/static/css/styles.css
body { font-family: Arial, sans-serif; margin: 0; padding: 0; }
header { background: #333; color: white; padding: 1rem; text-align: center; }
nav a { color: white; margin: 0 1rem; text-decoration: none; }
main { padding: 2rem; }
form { margin-top: 1rem; }
table, td, th {
    border: 1px solid #ccc;
    border-collapse: collapse;
    padding: 0.5rem;
}
ul { list-style-type: none; padding: 0; }
li { background: #f2f2f2; margin-bottom: 0.5rem; padding: 0.5rem; }
EOF

cat <<EOF > app/static/js/scripts.js
console.log("JavaScript geladen!");
EOF

# run.py anlegen (hier startet die Flask-App)
echo "🚀 Erstelle run.py..."
cat <<EOF > run.py
from app import app

if __name__ == "__main__":
    # Standard-Port 5000, kann angepasst werden
    app.run(host="0.0.0.0", port=5000)
EOF

# Virtual Environment anlegen und Pakete installieren
echo "📦 Erstelle Virtual Environment und installiere Pakete..."
python3 -m venv venv
source venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
deactivate

# Datenbank initialisieren und Default-Admin erstellen
echo "🗄️ Initialisiere SQLite-Datenbank..."
source venv/bin/activate
python3 -c "
from app import app, db
from app.models import User

with app.app_context():
    db.create_all()
    if not User.query.filter_by(username='PAM').first():
        admin = User(username='PAM')
        admin.set_password('PAM')  # Hinweis: Verwenden Sie sichere Passwörter in Produktionsumgebungen
        db.session.add(admin)
        db.session.commit()
    print('✅ Default-Admin erstellt (Benutzername: PAM, Passwort: PAM)')
"
deactivate

# Systemd-Service erstellen
echo "📝 Erstelle Systemd-Service-Datei..."
cat <<EOF > /etc/systemd/system/webapp.service
[Unit]
Description=Web Application (Flask)
After=network.target

[Service]
# Es wird empfohlen, einen dedizierten Benutzer für die Anwendung zu verwenden, z.B. "webapp"
User=root
Group=root
WorkingDirectory=/opt/webapp
ExecStart=/opt/webapp/venv/bin/python /opt/webapp/run.py
Restart=always

[Install]
WantedBy=multi-user.target
EOF

# Service aktivieren und starten
echo "📦 Aktivieren und Starten des Systemd-Services..."
systemctl daemon-reload
systemctl enable webapp.service
systemctl start webapp.service

# Abschluss
echo "✅ Setup abgeschlossen! Die Anwendung läuft (sofern keine Firewall blockiert) unter http://<server-ip>:5000"
