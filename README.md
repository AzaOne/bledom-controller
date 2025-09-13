# BLEDOM Controller

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

An advanced web-based controller for "BLEDOM" Bluetooth Low Energy (BLE) LED light strips. This project provides a self-hosted agent that connects directly to your light strip, exposing a rich web interface for real-time control, powerful Lua scripting for custom animations, and robust scheduling.

## Features

- **Full Web Interface:** Control power, color, brightness, and hardware effects from any browser.
- **Lua Scripting Engine:** Write and save your own custom color patterns and animations directly in the UI.
- **High-Level Effects API:** Simple Lua functions like `breathe()`, `strobe()`, and `fade()` for creating complex animations with one line of code.
- **Live Pattern Editor:** A full-featured code editor (CodeMirror) is built into the web UI for creating, editing, and deleting Lua patterns.
- **Agent-Side Scheduling:** Use standard cron syntax to schedule commands (e.g., `power on`, `run pattern sunrise.lua`). Schedules are saved and persist across restarts.
- **On-Device Scheduling:** Sync the device's time and set its internal on/off schedule.
- **Dockerized & Cross-Platform:** Easy deployment using Docker and Docker Compose. Build scripts are included for native cross-platform binaries (Linux AMD64/ARM64).
- **Automatic Reconnection:** The agent constantly monitors the BLE connection and will automatically reconnect and resume patterns if the device disconnects.

## Getting Started

The easiest way to run the BLEDOM Controller is with Docker and Docker Compose.

### Prerequisites

- A Linux host with a working Bluetooth adapter (e.g., a Raspberry Pi, NUC, or any server).
- [Docker](https://docs.docker.com/engine/install/) and [Docker Compose](https://docs.docker.com/compose/install/) installed on the host.
- The host's Bluetooth service must be running and not actively used by another application.

### Installation

1.  **Clone the repository:**
    ```sh
    git clone https://github.com/AzaOne/bledom-controller.git
    cd bledom-controller
    ```

2.  **Review Configuration (Optional):**
    - The `patterns/` directory contains your Lua scripts. You can add your own `.lua` files here before starting.
    - `schedules.json` stores your cron schedules. You can pre-configure it or manage schedules through the UI.

3.  **Run with Docker Compose:**
    This command will build the Docker image and start the controller in the background.
    ```sh
    docker compose up --build -d
    ```

4.  **Access the Web UI:**
    Open your web browser and navigate to `http://<your-host-ip>:8080`. The controller will automatically scan for and connect to your BLEDOM device.

## Usage

### Main Controls
- **Power, Color, Brightness:** Use the sliders and color pickers for direct control. These actions will stop any running Lua pattern.
- **Hardware Effects:** Select one of the built-in device patterns. This will also stop any running Lua pattern.

### Lua Patterns & Editor
- **Run a Pattern:** Select a pattern from the "Lua Patterns" dropdown and click "Run Pattern".
- **Stop a Pattern:** Click "Stop Pattern" to cancel the currently running script.
- **Edit a Pattern:**
    1. In the "Pattern Editor" section, select a file and click "Load".
    2. Make your changes in the code editor.
    3. To save, ensure the filename is correct and click "Save Pattern". To create a new file, change the filename to something new before saving.

### Scheduler (Agent)
The scheduler uses standard cron syntax to automate tasks.

- **Cron Syntax:** `MINUTE HOUR DAY-OF-MONTH MONTH DAY-OF-WEEK`
  - Use [crontab.guru](https://crontab.guru/) to easily build expressions.
- **Available Commands:**
    - `power on` / `power off`: Turns the lights on or off.
    - `pattern [filename.lua]`: Runs a specific Lua pattern file. Example: `pattern sunrise.lua`.
    - `lua [lua_code]`: Executes a single line of Lua code. Example: `lua set_color(255, 100, 0)`.

## Lua API Reference

You can call these global functions from your Lua scripts.

#### Core Functions
- `set_power(boolean)`: Turns the LEDs on (`true`) or off (`false`).
- `set_color(r, g, b)`: Sets the color (values `0-255`).
- `set_brightness(value)`: Sets the brightness (value `1-100`).
- `sleep(milliseconds)`: Pauses the script. This sleep is cancellable by the "Stop Pattern" button.
- `should_stop()`: Returns `true` if the user has requested the pattern to stop. Check this in long loops to make your scripts responsive.
- `print(message)`: Logs a message to the agent's console output.

#### High-Level Effects
These are blocking functions that run a complete animation. They are also cancellable.

- `breathe(duration_ms)`: Performs a smooth pulse from 1% to 100% brightness and back. Set the color first.
  - *Example:* `set_color(0, 255, 255); breathe(4000)`
- `strobe(r, g, b, duration_ms, frequency_hz)`: Flashes a color for a total duration at a specific frequency.
  - *Example:* `strobe(255, 255, 255, 5000, 10)`
- `fade(r1, g1, b1, r2, g2, b2, duration_ms)`: Smoothly transitions from a start color to an end color.
  - *Example:* `fade(255, 0, 0, 0, 0, 255, 3000)`
- `fade_brightness(start_brightness, end_brightness, duration_ms)`: Smoothly transitions the brightness from a start value to an end value. Brightness values are 1-100. The color should be set beforehand. 
  - *Example:* `fade_brightness(100, 20, 2000)`: 2 second fade out to 20% brightness.


## Building from Source

If you prefer to run the agent without Docker:

1.  **Install Prerequisites:**
    - Go 1.25+
    - C compiler and D-Bus development libraries. Installation commands for common distributions:
      - **Debian/Ubuntu:** `sudo apt update && sudo apt install build-essential libdbus-1-dev`
      - **Arch Linux:** `sudo pacman -S base-devel dbus`

2.  **Run the build script:**
    The script will build binaries for multiple platforms and place them in the `build/` directory.
    ```sh
    ./build.sh
    ```

3.  **Run the agent:**
    Ensure the `static/` and `patterns/` directories are in the same location as the binary.
    ```sh
    ./build/bledom-controller-linux-amd64
    ```

## Project Structure

- `cmd/agent/main.go`: Application entry point.
- `internal/agent`: Core agent logic, ties all services together.
- `internal/ble`: Handles Bluetooth LE connection and command packets.
- `internal/lua`: The Lua scripting engine and Go function bindings.
- `internal/server`: The WebSocket and HTTP server.
- `internal/scheduler`: The cron-based job scheduler.
- `static/`: Contains the frontend HTML, CSS, and JavaScript.
- `patterns/`: Default location for user-created Lua patterns.
- `Dockerfile`: Defines the container for production deployment.
- `compose.yml`: Easy-to-use Docker Compose file for deployment.

## Contributing

Contributions, issues, and feature requests are welcome! Feel free to check the [issues page](https://github.com/AzaOne/bledom-controller/issues).

## License

This project is licensed under the MIT License. See the [LICENSE](./LICENSE) file for details.
