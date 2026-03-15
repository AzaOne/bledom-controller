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
    let manualRefresh = false;
    let manualRefreshResolver = null;

    initCodeMirror();
    initColorPicker();
    initNavigation();
    initSidebarToggle();
    initDarkMode();
    initEventListeners();
    renderHardwarePatterns(deviceAPI.setHardwarePattern);
    populateTimePickers();
    populateCronTimePickers();
    initPullToRefresh();

    function connect() {
        const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${proto}//${window.location.host}/ws`;
        socket = new WebSocket(wsUrl);
        setSocket(socket);

        socket.onopen = () => {
            console.log('WebSocket connection established');
            setStatus('agent-connected', 'Agent Connected');
            if (manualRefreshResolver) {
                manualRefreshResolver();
                manualRefreshResolver = null;
            }
            manualRefresh = false;
        };

        socket.onclose = () => {
            console.log('WebSocket connection closed. Retrying…');
            if (manualRefresh) {
                setTimeout(connect, 150);
                return;
            }
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
                    if (state.isOn !== undefined) {
                        updatePowerVisual(state.isOn);
                    }
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

    function refreshData() {
        if (manualRefresh) return Promise.resolve();
        manualRefresh = true;
        setStatus('connecting', 'Refreshing…');

        return new Promise((resolve) => {
            manualRefreshResolver = resolve;

            if (!socket || socket.readyState === WebSocket.CLOSED) {
                connect();
            } else {
                try {
                    socket.close(4000, 'refresh');
                } catch (err) {
                    connect();
                }
            }

            setTimeout(() => {
                if (manualRefreshResolver) {
                    manualRefreshResolver();
                    manualRefreshResolver = null;
                }
                manualRefresh = false;
            }, 3500);
        });
    }

    function initPullToRefresh() {
        const content = document.getElementById('content');
        const pullEl = document.getElementById('pullToRefresh');
        if (!content || !pullEl) return;

        const icon = pullEl.querySelector('.ptr-icon');
        const text = pullEl.querySelector('.ptr-text');
        const overlay = document.getElementById('offlineOverlay');

        const height = 60;
        const threshold = 80;
        const maxPull = 140;

        let pulling = false;
        let startY = 0;
        let pullDistance = 0;

        const resetIndicator = () => {
            pullEl.classList.remove('active', 'ready', 'refreshing');
            pullEl.style.transform = `translateY(-${height}px)`;
            pullEl.style.opacity = '0';
            if (icon) icon.textContent = 'arrow_downward';
            if (text) text.textContent = 'Pull to refresh';
        };

        const setIndicator = (distance) => {
            const offset = Math.min(distance, maxPull);
            const visible = Math.min(offset, height);
            pullEl.style.transform = `translateY(${visible - height}px)`;
            pullEl.style.opacity = String(Math.min(1, visible / height));
        };

        const canStartPull = (evt) => {
            if (manualRefresh) return false;
            if (overlay && !overlay.classList.contains('hidden')) return false;
            if (content.scrollTop > 0) return false;
            if (!evt.touches || evt.touches.length !== 1) return false;
            if (evt.target.closest('.CodeMirror')) return false;
            return true;
        };

        const triggerRefresh = () => {
            pullEl.classList.add('refreshing');
            pullEl.classList.remove('ready');
            pullEl.style.transform = 'translateY(0)';
            pullEl.style.opacity = '1';
            if (icon) icon.textContent = 'autorenew';
            if (text) text.textContent = 'Refreshing…';

            refreshData().finally(() => {
                resetIndicator();
            });
        };

        content.addEventListener('touchstart', (evt) => {
            if (!canStartPull(evt)) return;
            pulling = true;
            startY = evt.touches[0].clientY;
            pullDistance = 0;
        }, { passive: true });

        content.addEventListener('touchmove', (evt) => {
            if (!pulling) return;
            const currentY = evt.touches[0].clientY;
            pullDistance = Math.max(0, currentY - startY);
            if (pullDistance <= 0) return;

            evt.preventDefault();
            pullEl.classList.add('active');
            setIndicator(pullDistance);

            const isReady = pullDistance >= threshold;
            pullEl.classList.toggle('ready', isReady);
            if (text) text.textContent = isReady ? 'Release to refresh' : 'Pull to refresh';
        }, { passive: false });

        const finishPull = () => {
            if (!pulling) return;
            pulling = false;
            if (pullDistance >= threshold) {
                triggerRefresh();
            } else {
                resetIndicator();
            }
            pullDistance = 0;
        };

        content.addEventListener('touchend', finishPull);
        content.addEventListener('touchcancel', finishPull);

        resetIndicator();
    }

    function updatePowerVisual(isOn) {
        ui.powerOnBtn.classList.toggle('active-state',  !!isOn);
        ui.powerOffBtn.classList.toggle('active-state', !isOn);
    }

    connect();
});
