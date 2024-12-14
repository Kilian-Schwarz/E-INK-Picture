# E-Ink Picture

![E-Ink Picture Logo](https://github.com/Kilian-Schwarz/E-INK-Picture/blob/main/logo.png?raw=true)

**E-Ink Picture** is a comprehensive solution for designing and displaying dynamic content on E-Ink displays. It features a web-based designer interface, a Flask server for managing designs, and a client application that renders the designs on E-Ink hardware. The system supports various modules like text, images, weather updates, date/time, timers, news, and customizable shapes, providing flexibility and ease of use for creating visually appealing E-Ink displays.

## Table of Contents

- [Features](#features)
- [Architecture](#architecture)
- [Installation](#installation)
  - [Prerequisites](#prerequisites)
  - [Using Docker](#using-docker)
  - [Manual Setup](#manual-setup)
- [Usage](#usage)
  - [Running the Server](#running-the-server)
  - [Accessing the Designer](#accessing-the-designer)
  - [Running the Client](#running-the-client)
- [Directory Structure](#directory-structure)
- [Configuration](#configuration)
- [License](#license)
- [Acknowledgments](#acknowledgments)

## Features

- **Web-Based Designer:** Intuitive drag-and-drop interface for creating and customizing E-Ink display layouts.
- **Module Support:** Add and configure various modules including Text, Image, Weather, Date/Time, Timer, News, and Line/Shape.
- **Image and Font Upload:** Upload custom images and fonts to enhance your designs.
- **Weather Integration:** Fetch and display real-time weather data based on specified locations.
- **Timer Functionality:** Set countdown timers with customizable formats.
- **Preview Mode:** Generate and view previews of your designs before deploying them to the E-Ink display.
- **Offline Support:** Sync date/time and timers locally when offline.
- **Docker Support:** Easily deploy the server using Docker for streamlined setup and management.

## Architecture

The project consists of three main components:

1. **Flask Server (`Server.py`):** Handles API requests, manages designs, serves media files (images and fonts), and provides endpoints for the web-based designer.

2. **Web-Based Designer (`templates/designer.html`):** A frontend interface built with HTML, CSS, and JavaScript that allows users to create and manage their E-Ink display designs.

3. **Client Application (`Client.py`):** Fetches designs from the server, processes them, and renders the content on E-Ink hardware.

![Architecture Diagram](https://github.com/Kilian-Schwarz/E-INK-Picture/blob/main/architecture.png?raw=true)

## Installation

### Prerequisites

Before setting up **E-Ink Picture**, ensure you have the following installed on your system:

- **Python 3.7+**
- **Pip** (Python package installer)
- **Git** (for cloning the repository)
- **Docker** (optional, for containerized deployment)
- **Waveshare E-Ink Display** (compatible with the `epd7in5_V2` driver)
- **Waveshare E-Ink Drivers:** Ensure you have the [Waveshare E-Ink Display Drivers](https://www.waveshare.com/wiki/E-Paper) installed, especially the `epd7in5_V2` module used in `Client.py`.

### Using Docker

For an easy and consistent setup, you can use Docker to deploy the Flask server.

1. **Clone the Repository:**

   ```bash
   git clone https://github.com/Kilian-Schwarz/E-INK-Picture.git
   cd E-INK-Picture
   ```

2. **Docker Compose Setup:**

   Ensure you have Docker and Docker Compose installed. Create a `docker-compose.yml` file with the following content (as provided):

   ```yaml
   version: '3.8'

   services:
     e-ink-picture:
       image: kilianschwarz/e-ink-picture
       ports:
         - "5000:5000"
       volumes:
         - ./uploaded_images:/app/uploaded_images/
         - ./designs:/app/designs/
         - ./fonts:/app/fonts/
   ```

3. **Start the Container:**

   ```bash
   docker-compose up -d
   ```

   The Flask server will be accessible at `http://localhost:5000`.

### Manual Setup

If you prefer setting up the server manually without Docker:

1. **Clone the Repository:**

   ```bash
   git clone https://github.com/Kilian-Schwarz/E-INK-Picture.git
   cd E-INK-Picture
   ```

2. **Create and Activate a Virtual Environment:**

   ```bash
   python3 -m venv venv
   source venv/bin/activate
   ```

3. **Install Dependencies:**

   ```bash
   pip install -r requirements.txt
   ```

   **Note:** Ensure that the `requirements.txt` file includes all necessary Python packages such as `Flask`, `Pillow`, `requests`, etc.

4. **Set Up Directories:**

   Ensure the following directories exist:

   - `uploaded_images/`
   - `designs/`
   - `fonts/`
   - `weather_styles/`

   You can create them using:

   ```bash
   mkdir -p uploaded_images designs fonts weather_styles
   ```

5. **Run the Flask Server:**

   ```bash
   python Server.py
   ```

   The server will start on `http://0.0.0.0:5000`.

## Usage

### Running the Server

If you've set up using Docker, the server is already running at `http://localhost:5000`. For manual setups, ensure the Flask server is running as described in the [Manual Setup](#manual-setup) section.

### Accessing the Designer

1. Open your web browser and navigate to `http://localhost:5000/designer`.
2. Use the intuitive interface to drag and drop modules onto the design canvas.
3. Customize module properties, upload images and fonts, and arrange elements as desired.
4. Save your designs, clone existing ones, or set a design as active.
5. Preview your design to see how it will appear on the E-Ink display.

### Running the Client

The client application fetches the active design from the server and renders it on the E-Ink display.

1. **Ensure Dependencies:**

   The client relies on the `waveshare_epd` library. Install it using:

   ```bash
   pip install waveshare-epd
   ```

2. **Configure Client Settings:**

   - Verify the `BASE_URL` in `Client.py` points to your Flask server (`http://127.0.0.1:5000` if running locally).
   - Adjust `EINK_OFFSET_X`, `EINK_OFFSET_Y`, `EINK_WIDTH`, and `EINK_HEIGHT` in `Client.py` if your E-Ink display has different dimensions.

3. **Run the Client:**

   ```bash
   python Client.py
   ```

   The client will attempt to fetch the active design and display it on the connected E-Ink hardware. It also handles offline synchronization for date/time and timers.

## Directory Structure

```
E-INK-Picture/
├── Server.py
├── Client.py
├── docker-compose.yml
├── requirements.txt
├── designs/
│   └── design_default.json
├── uploaded_images/
│   └── ... (uploaded BMP images)
├── fonts/
│   └── ... (uploaded TTF/OTF fonts)
├── weather_styles/
│   └── ... (weather style JSON files)
└── templates/
    └── designer.html
```

- **Server.py:** Flask server managing designs and serving the designer interface.
- **Client.py:** Client application for rendering designs on E-Ink hardware.
- **docker-compose.yml:** Docker Compose configuration for containerized deployment.
- **requirements.txt:** Python dependencies.
- **designs/:** JSON files representing different display designs.
- **uploaded_images/:** Uploaded BMP images for use in designs.
- **fonts/:** Uploaded font files (TTF/OTF) for text modules.
- **weather_styles/:** JSON templates for weather module formatting.
- **templates/:** HTML templates for the Flask server, including the designer interface.

## Configuration

### Server Configuration

- **Port:** The Flask server runs on port `5000` by default. You can change this in `Server.py` if needed.
- **Folders:**
  - `uploaded_images/`: Stores uploaded images in BMP format.
  - `designs/`: Stores design JSON files.
  - `fonts/`: Stores uploaded font files.
  - `weather_styles/`: Stores weather style templates.

### Client Configuration

- **E-Ink Display Settings:**
  - Adjust `EINK_OFFSET_X`, `EINK_OFFSET_Y`, `EINK_WIDTH`, and `EINK_HEIGHT` in `Client.py` to match your display's specifications.
- **Design Paths:**
  - `DESIGN_PATH`: API endpoint for fetching designs.
  - `IMAGE_PATH` and `FONT_PATH`: API endpoints for accessing images and fonts.

## License

This project is licensed under the [MIT License](LICENSE).

## Acknowledgments

- [Flask](https://flask.palletsprojects.com/) - Lightweight WSGI web application framework.
- [Waveshare E-Ink Display Drivers](https://www.waveshare.com/) - Hardware integration for E-Ink displays.
- [Open-Meteo](https://open-meteo.com/) - Free weather API used for fetching weather data.
- [Docker](https://www.docker.com/) - Containerization platform for easy deployment.

---
