// static/js/main.js

import { ui, initCodeMirror, initColorPicker, setStatus, renderHardwarePatterns, populateTimePickers, updatePatternLists, updateScheduleList, initDarkMode } from './ui.js';
import { deviceAPI, setSocket } from './api.js';
import { initEventListeners } from './event-listeners.js';

document.addEventListener('DOMContentLoaded', () => {
    let socket;

    initCodeMirror();
    initColorPicker();
    initEventListeners();
    renderHardwarePatterns(deviceAPI.setHardwarePattern);
    populateTimePickers();
    initDarkMode();

    function connect() {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${proto}//${window.location.host}/ws`;
        socket = new WebSocket(wsUrl);

        setSocket(socket);

        socket.onopen = () => {
            console.log("WebSocket connection established");
            setStatus('agent-connected', 'Agent Connected, awaiting device status...');
        };

        socket.onclose = () => {
            console.log('WebSocket connection closed. Retrying...');
            setStatus('disconnected', 'Agent Disconnected. Retrying...');
            ui.controls.style.display = 'none';
            setTimeout(connect, 3000);
        };

        socket.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            switch (msg.type) {
                case 'ble_status':
                    ui.controls.style.display = 'block';
                    if (msg.payload.connected) {
                        const rssi = msg.payload.rssi;
                        let statusText = 'Device Connected';
                        if (rssi && rssi !== 0) statusText += ` (RSSI: ${rssi} dBm)`;
                        setStatus('connected', statusText);
                    } else {
                        setStatus('device-disconnected', 'Device Disconnected (Agent is running)');
                    }
                    break;

                // --- NEW: Receive Initial Device State ---
                case 'device_state':
                    const state = msg.payload;

                    // 1. Update Color
                    if (ui.colorPicker && state.hex) {
                        ui.colorPicker.color.hexString = state.hex;
                    }

                    // 2. Update Brightness
                    if (state.brightness !== undefined) {
                        ui.brightnessSlider.value = state.brightness;
                        ui.brightnessValue.textContent = `${state.brightness}%`;
                    }

                    // 3. Update Speed
                    if (state.speed !== undefined) {
                        ui.speedSlider.value = state.speed;
                        ui.speedValue.textContent = `${state.speed}%`;
                    }

                    // 4. Update Power Buttons Visuals (Optional)
                     if (state.isOn) {
                         ui.powerOnBtn.style.border = "2px solid #fff";
                         ui.powerOffBtn.style.border = "none";
                    } else {
                         ui.powerOnBtn.style.border = "none";
                         ui.powerOffBtn.style.border = "2px solid #fff";
                    }
                    break;
                // ----------------------------------------

                case 'color_update':
                    if (ui.colorPicker && msg.payload.hex) {
                        ui.colorPicker.color.hexString = msg.payload.hex;
                    }
                    break;
                case 'brightness_update':
                    const bVal = msg.payload.value;
                    ui.brightnessSlider.value = bVal;
                    ui.brightnessValue.textContent = `${bVal}%`;
                    break;
                case 'power_update':
                     if (msg.payload.isOn) {
                         ui.powerOnBtn.style.border = "2px solid #fff";
                         ui.powerOffBtn.style.border = "none";
                    } else {
                         ui.powerOnBtn.style.border = "none";
                         ui.powerOffBtn.style.border = "2px solid #fff";
                    }
                    break;

                case 'pattern_list': updatePatternLists(msg.payload); break;
                case 'schedule_list': updateScheduleList(msg.payload, deviceAPI.removeSchedule); break;
                case 'pattern_status': ui.patternStatus.textContent = msg.payload.running || 'Idle'; break;
                case 'pattern_code':
                    ui.editorFilename.value = msg.payload.name;
                    ui.codeEditor.setValue(msg.payload.code);
                    break;
                default:
                    console.log("Unknown message type:", msg.type, msg.payload);
            }
        };

        socket.onerror = (error) => {
            console.error("WebSocket Error:", error);
            setStatus('disconnected', 'WebSocket Error. Retrying...');
            socket.close();
        };
    }

    connect();
});
