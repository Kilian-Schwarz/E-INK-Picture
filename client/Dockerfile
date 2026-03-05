# syntax=docker/dockerfile:1
FROM python:3.11-slim-bookworm

WORKDIR /app

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        libgpiod2 \
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
