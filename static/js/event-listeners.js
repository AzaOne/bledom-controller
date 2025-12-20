// static/js/event-listeners.js

import { ui } from './ui.js';
import { deviceAPI } from './api.js';
import { debounce } from './utils.js';

/**
 * Initializes all event listeners for UI elements.
 */
export function initEventListeners() {
    // 1. Power Controls
    ui.powerOnBtn.addEventListener('click', () => deviceAPI.setPower(true));
    ui.powerOffBtn.addEventListener('click', () => deviceAPI.setPower(false));

    // 2. Iro.js Color Picker Logic
    const sendColor = (color) => {
        const { r, g, b } = color.rgb;
        deviceAPI.setColor(r, g, b);
    };

    if (ui.colorPicker) {
        ui.colorPicker.on('input:change', (color) => {
            debounce(sendColor, [color], 'colorChange', 100); 
        });
    }

    // 3. Color Presets
    const handlePresetClick = (hex) => {
        if (ui.colorPicker) {
            ui.colorPicker.color.hexString = hex;
        }
        const bigint = parseInt(hex.substring(1), 16);
        deviceAPI.setColor((bigint >> 16) & 255, (bigint >> 8) & 255, bigint & 255);
    };

    document.querySelectorAll('.color-presets button[data-color]').forEach(button => {
        button.addEventListener('click', () => {
            handlePresetClick(button.dataset.color);
        });
    });

    // 4. Brightness Slider
    ui.brightnessSlider.addEventListener('input', (e) => {
        ui.brightnessValue.textContent = `${e.target.value}%`;
        deviceAPI.setBrightness(e.target.value)
    });

    // 5. Brightness Presets
    document.querySelectorAll('.brightness-presets button').forEach(button => {
        button.addEventListener('click', () => {
            const val = button.dataset.brightness;
            ui.brightnessSlider.value = val;
            ui.brightnessValue.textContent = `${val}%`;
            deviceAPI.setBrightness(val);
        });
    });

    // 6. Speed Slider
    ui.speedSlider.addEventListener('input', (e) => {
        ui.speedValue.textContent = `${e.target.value}%`;
        const maxDuration = 15; const minDuration = 0.2;
        const duration = maxDuration - ((parseInt(e.target.value) / 100) * (maxDuration - minDuration));
        deviceAPI.setSpeed(e.target.value);
        document.querySelectorAll('.pattern-preview').forEach(preview => {
            if (window.getComputedStyle(preview).animationName !== 'none') {
                preview.style.animationDuration = `${duration}s`;
            }
        });
    });

    // 7. Other settings
    ui.syncTimeBtn.addEventListener('click', deviceAPI.syncTime);
    ui.setRgbOrderBtn.addEventListener('click', () => {
        deviceAPI.setRgbOrder(parseInt(ui.wire1.value), parseInt(ui.wire2.value), parseInt(ui.wire3.value));
    });
    ui.setScheduleBtn.addEventListener('click', () => deviceAPI.setDeviceSchedule(true));
    ui.clearScheduleBtn.addEventListener('click', () => deviceAPI.setDeviceSchedule(false));
    
    // 8. Patterns & Scheduler
    ui.runPatternBtn.addEventListener('click', () => { if (ui.patternSelector.value) deviceAPI.runPattern(ui.patternSelector.value); });
    ui.stopPatternBtn.addEventListener('click', deviceAPI.stopPattern);

    ui.loadPatternBtn.addEventListener('click', () => { if (ui.editorPatternSelector.value) deviceAPI.getPatternCode(ui.editorPatternSelector.value); });
    ui.newPatternBtn.addEventListener('click', () => {
        ui.editorFilename.value = 'new-pattern.lua';
        ui.editorFilename.focus();
    });
    ui.savePatternBtn.addEventListener('click', () => {
        const filename = ui.editorFilename.value.trim();
        if (!filename || !filename.endsWith('.lua')) { alert('Filename is invalid. It must not be empty and must end with .lua'); return; }
        deviceAPI.savePatternCode(filename, ui.codeEditor.getValue());
        alert(`Pattern "${filename}" saved!`);
    });
    ui.deletePatternBtn.addEventListener('click', () => {
        const filename = ui.editorPatternSelector.value;
        if (filename && confirm(`Are you sure you want to permanently delete "${filename}"?`)) {
            deviceAPI.deletePattern(filename);
            if (ui.editorFilename.value === filename) ui.newPatternBtn.click();
        }
    });
    ui.addScheduleBtn.addEventListener('click', () => {
        const spec = ui.scheduleSpec.value.trim();
        const command = ui.scheduleCommand.value.trim();
        if (spec && command) deviceAPI.addSchedule(spec, command);
        else alert('Please provide both a cron spec and a command.');
    });
    ui.darkModeToggle.addEventListener('click', () => {
        document.body.classList.toggle('dark-mode');
        const isDark = document.body.classList.contains('dark-mode');
        localStorage.setItem('darkMode', isDark);
        document.querySelector('#darkModeToggle .material-icons').textContent = isDark ? 'light_mode' : 'dark_mode';
    });
}
