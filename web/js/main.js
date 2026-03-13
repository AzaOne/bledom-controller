import {
    ui,
    initCodeMirror,
    initColorPicker,
    setStatus,
    setRSSI,
    showControls,
    renderHardwarePatterns,
    populateTimePickers,
    populateCronTimePickers,
    updatePatternLists,
    updateScheduleList,
    initDarkMode,
    initNavigation,
    initSidebarToggle,
} from './ui.js';
import { deviceAPI, setSocket } from './api.js';
import { initEventListeners } from './event-listeners.js';

document.addEventListener('DOMContentLoaded', () => {
    let socket;

    initCodeMirror();
    initColorPicker();
    initNavigation();
    initSidebarToggle();
    initDarkMode();
    initEventListeners();
    renderHardwarePatterns(deviceAPI.setHardwarePattern);
    populateTimePickers();
    populateCronTimePickers();

    function connect() {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${proto}//${window.location.host}/ws`;
        socket = new WebSocket(wsUrl);
        setSocket(socket);

        socket.onopen = () => {
            console.log('WebSocket connection established');
            setStatus('agent-connected', 'Agent Connected');
        };

        socket.onclose = () => {
            console.log('WebSocket connection closed. Retrying…');
            setStatus('disconnected', 'Agent Disconnected');
            showControls(false);
            setTimeout(connect, 3000);
        };

        socket.onmessage = (event) => {
            const msg = JSON.parse(event.data);

            switch (msg.type) {
                case 'ble_status': {
                    showControls(true);
                    if (msg.payload.connected) {
                        setRSSI(msg.payload.rssi);
                        setStatus('connected', 'Connected');
                    } else {
                        setRSSI(0);
                        setStatus('device-disconnected', 'Disconnected');
                    }
                    break;
                }

                case 'device_state': {
                    const state = msg.payload;
                    if (ui.colorPicker && state.hex) ui.colorPicker.color.hexString = state.hex;
                    if (state.brightness !== undefined) {
                        ui.brightnessSlider.value = state.brightness;
                        ui.brightnessValue.textContent = `${state.brightness}%`;
                    }
                    if (state.speed !== undefined) {
                        const maxV = parseInt(ui.speedSlider.max, 10) || 100;
                        const sVal = Math.min(parseInt(state.speed, 10), maxV);
                        ui.speedSlider.value = sVal;
                        ui.speedValue.textContent = `${sVal}%`;
                    }
                    updatePowerVisual(state.isOn);
                    break;
                }

                case 'color_update':
                    if (ui.colorPicker && msg.payload.hex) ui.colorPicker.color.hexString = msg.payload.hex;
                    break;

                case 'brightness_update': {
                    const bVal = msg.payload.value;
                    ui.brightnessSlider.value      = bVal;
                    ui.brightnessValue.textContent = `${bVal}%`;
                    break;
                }

                case 'power_update':
                    updatePowerVisual(msg.payload.isOn);
                    break;

                case 'pattern_list': updatePatternLists(msg.payload); break;
                case 'schedule_list': updateScheduleList(msg.payload); break;
                case 'pattern_status': ui.patternStatus.textContent = msg.payload.running || 'Idle'; break;

                case 'pattern_code':
                    ui.editorFilename.value = msg.payload.name;
                    ui.codeEditor.setValue(msg.payload.code);
                    break;

                default:
                    console.log('Unknown message type:', msg.type, msg.payload);
            }
        };

        socket.onerror = (error) => {
            console.error('WebSocket Error:', error);
            setStatus('disconnected', 'Error');
            socket.close();
        };
    }

    function updatePowerVisual(isOn) {
        ui.powerOnBtn.classList.toggle('active-state',  !!isOn);
        ui.powerOffBtn.classList.toggle('active-state', !isOn);
    }

    connect();
});
