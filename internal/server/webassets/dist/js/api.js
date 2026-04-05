import { debounce } from './utils.js';
import { ui } from './ui.js';

let socketInstance = null;

export function setSocket(ws) {
    socketInstance = ws;
}

function sendSocketCommand(type, payload) {
    if (!socketInstance || socketInstance.readyState !== WebSocket.OPEN) {
        console.warn('WebSocket not open. Ignoring command:', type, payload);
        return;
    }
    socketInstance.send(JSON.stringify({ type, payload }));
}

export const deviceAPI = {
    setPower: (isOn) => sendSocketCommand('setPower', { isOn }),
    setColor: (r, g, b) => sendSocketCommand('setColor', { r, g, b }),
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
    updateSchedule: (id, spec, command) => sendSocketCommand('updateSchedule', { id, spec, command }),
    removeSchedule: (id) => sendSocketCommand('removeSchedule', { id }),
    runScheduleNow: (id) => sendSocketCommand('runScheduleNow', { id }),
    setScheduleEnabled: (id, enabled) => sendSocketCommand('setScheduleEnabled', { id, enabled }),
    setAllSchedulesEnabled: (enabled) => sendSocketCommand('setAllSchedulesEnabled', { enabled }),
    getPatternCode: (name) => sendSocketCommand('getPatternCode', { name }),
    savePatternCode: (name, code) => sendSocketCommand('savePatternCode', { name, code }),
    deletePattern: (name) => sendSocketCommand('deletePattern', { name }),
};
