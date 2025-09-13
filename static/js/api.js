// static/js/api.js

import { debounce } from './utils.js';
import { ui } from './ui.js'; // We might need `ui.scheduleHour.value` etc here.

let socketInstance = null; // Private variable to hold the WebSocket instance

/**
 * Sets the WebSocket instance for the API module to use.
 * This allows `main.js` to initialize the socket and then provide it to `api.js`.
 * @param {WebSocket} ws The WebSocket instance.
 */
export function setSocket(ws) {
    socketInstance = ws;
}

/**
 * Sends a command over the WebSocket connection.
 * @param {string} type The command type.
 * @param {Object} payload The command payload.
 */
function sendSocketCommand(type, payload) {
    if (!socketInstance || socketInstance.readyState !== WebSocket.OPEN) {
        console.warn("WebSocket not open. Ignoring command:", type, payload);
        return;
    }
    socketInstance.send(JSON.stringify({ type, payload }));
}

// Exported API functions, using sendSocketCommand
export const deviceAPI = {
    setPower: (isOn) => sendSocketCommand('setPower', { isOn }),
    setColor: (r, g, b) => sendSocketCommand('setColor', { r, g, b }),
    // Debounce brightness and speed to prevent flooding the server
    setBrightness: (value) => debounce(sendSocketCommand, ['setBrightness', { value: parseInt(value) }], 'brightness', 100),
    setHardwarePattern: (id) => sendSocketCommand('setHardwarePattern', { id }),
    setSpeed: (value) => debounce(sendSocketCommand, ['setSpeed', { value: parseInt(value) }], 'speed', 50),
    syncTime: () => sendSocketCommand('syncTime', {}),
    setRgbOrder: (v1, v2, v3) => sendSocketCommand('setRgbOrder', { v1, v2, v3 }),
    setDeviceSchedule: (isSet) => {
        const hour = parseInt(ui.scheduleHour.value);
        const minute = parseInt(ui.scheduleMinute.value);
        const second = parseInt(ui.scheduleSecond.value);
        const isOn = ui.scheduleAction.value === 'on';
        let weekdays = 0;
        document.querySelectorAll('input[name="weekday"]:checked').forEach(day => {
            weekdays |= (1 << parseInt(day.value));
        });
        sendSocketCommand('setSchedule', { hour, minute, second, weekdays, isOn, isSet });
    },
    runPattern: (name) => sendSocketCommand('runPattern', { name }),
    stopPattern: () => sendSocketCommand('stopPattern', {}),
    addSchedule: (spec, command) => sendSocketCommand('addSchedule', { spec, command }),
    removeSchedule: (id) => sendSocketCommand('removeSchedule', { id }),
    getPatternCode: (name) => sendSocketCommand('getPatternCode', { name }),
    savePatternCode: (name, code) => sendSocketCommand('savePatternCode', { name, code }),
    deletePattern: (name) => sendSocketCommand('deletePattern', { name }),
};
