// static/js/main.js

import { ui, initCodeMirror, setStatus, renderHardwarePatterns, populateTimePickers, updatePatternLists, updateScheduleList, initDarkMode } from './ui.js';
import { deviceAPI, setSocket } from './api.js';
import { initEventListeners } from './event-listeners.js';

document.addEventListener('DOMContentLoaded', () => {
    let socket; // Declared here to be accessible within this module's scope

    // Initialize UI components and state
    initCodeMirror();
    initEventListeners(); // Event listeners now depend on `ui` and `deviceAPI`
    renderHardwarePatterns(deviceAPI.setHardwarePattern); // Pass the API function needed for rendering
    populateTimePickers();
    initDarkMode();

    /**
     * Establishes and manages the WebSocket connection.
     */
    function connect() {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${proto}//${window.location.host}/ws`;
        socket = new WebSocket(wsUrl);

        // Provide the socket instance to the API module
        setSocket(socket);

        socket.onopen = () => {
            console.log("WebSocket connection established");
            setStatus('agent-connected', 'Agent Connected, awaiting device status...');
        };

        socket.onclose = () => {
            console.log('WebSocket connection closed. Retrying...');
            setStatus('disconnected', 'Agent Disconnected. Retrying...');
            ui.controls.style.display = 'none'; // Hide controls when disconnected
            setTimeout(connect, 3000); // Attempt to reconnect after 3 seconds
        };

        socket.onmessage = (event) => {
            const msg = JSON.parse(event.data);
            switch (msg.type) {
                case 'ble_status':
                    ui.controls.style.display = 'block'; // Show controls once status is received
                    if (msg.payload.connected) {
                        const rssi = msg.payload.rssi;
                        let statusText = 'Device Connected';
                        if (rssi && rssi !== 0) {
                             statusText += ` (RSSI: ${rssi} dBm)`;
                        }
                        setStatus('connected', statusText);
                    } else {
                        setStatus('device-disconnected', 'Device Disconnected (Agent is running)');
                    }
                    break;
                case 'pattern_list': updatePatternLists(msg.payload); break;
                case 'schedule_list': updateScheduleList(msg.payload, deviceAPI.removeSchedule); break; // Pass remove API function
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
            socket.close(); // Force close to trigger onclose and reconnect logic
        };
    }

    // Start the WebSocket connection process
    connect();
});
