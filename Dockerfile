# Use the official Python image as the base
FROM python:3.11-slim

# Set environment variables
ENV PYTHONDONTWRITEBYTECODE=1
ENV PYTHONUNBUFFERED=1

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    libglib2.0-0 \
    libsm6 \
    libxext6 \
    libxrender-dev \
    && rm -rf /var/lib/apt/lists/*

# Install Python dependencies
COPY requirements.txt .
RUN pip install --upgrade pip
RUN pip install --no-cache-dir -r requirements.txt

# Copy the application code
COPY app/ /app/

# Expose the port Flask is running on
EXPOSE 5000

# Create necessary directories with appropriate permissions
RUN mkdir -p /app/uploaded_images /app/designs /app/fonts /app/weather_styles

# Set environment variables for Flask
ENV FLASK_APP=server.py
ENV FLASK_RUN_HOST=0.0.0.0
ENV FLASK_RUN_PORT=5000

# (Optional) If you have a user, set it here for better security
# RUN useradd -m myuser
# USER myuser

# Start the Flask server
CMD ["gunicorn", "--bind", "0.0.0.0:5000", "server:app"]