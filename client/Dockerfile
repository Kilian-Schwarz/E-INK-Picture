FROM python:3.11-slim-bookworm

# Optional: custom CA certs for SSL-inspecting proxies
COPY certs/ /tmp/certs/
RUN if ls /tmp/certs/*.pem 1>/dev/null 2>&1; then \
        for f in /tmp/certs/*.pem; do \
            cp "$f" "/usr/local/share/ca-certificates/$(basename "$f" .pem).crt"; \
        done && \
        update-ca-certificates; \
    fi
ENV REQUESTS_CA_BUNDLE=/etc/ssl/certs/ca-certificates.crt
ENV SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt
ENV PIP_CERT=/etc/ssl/certs/ca-certificates.crt

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgpiod2 \
        libgpiod-dev \
        gcc \
        libjpeg-dev \
        zlib1g-dev \
        git && \
    rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Waveshare EPD library (not on PyPI, installed from GitHub)
RUN pip install --no-cache-dir \
    git+https://github.com/waveshare/e-Paper.git#subdirectory=RaspberryPi_JetsonNano/python

COPY . .

ENTRYPOINT ["python3", "client.py"]
