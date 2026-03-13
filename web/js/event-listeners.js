import {
    ui,
    renderPresets,
    enterScheduleEditMode,
    clearScheduleEditMode,
    toggleDarkMode,
    navigateTo,
} from './ui.js';
import { deviceAPI } from './api.js';
import { debounce, normalizeHex, pad } from './utils.js';
import { DEFAULT_PRESETS } from './constants.js';

export function initEventListeners() {
    ui.powerOnBtn.addEventListener('click', () => deviceAPI.setPower(true));
    ui.powerOffBtn.addEventListener('click', () => deviceAPI.setPower(false));

    const sendColor = (color) => {
        const { r, g, b } = color.rgb;
        deviceAPI.setColor(r, g, b);
    };

    if (ui.colorPicker) {
        ui.colorPicker.on('input:change', (color) => {
            debounce(sendColor, [color], 'colorChange', 100);
        });
    }

    const STORAGE_KEY = 'ble_presets';
    const OLD_STORAGE_KEY = 'ble_custom_presets';

    const handlePresetClick = (hex) => {
        if (ui.colorPicker) ui.colorPicker.color.hexString = hex;
        const bigint = parseInt(hex.substring(1), 16);
        deviceAPI.setColor((bigint >> 16) & 255, (bigint >> 8) & 255, bigint & 255);
    };

    const handleAddColor = () => {
        if (!ui.colorPicker) return;
        const currentColor = normalizeHex(ui.colorPicker.color.hexString);
        const stored = localStorage.getItem(STORAGE_KEY);
        const presets = stored ? JSON.parse(stored).map(c => normalizeHex(c)) : [];
        if (!presets.includes(currentColor)) {
            presets.push(currentColor);
            localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
            loadAndRender();
        } else {
            alert('This color is already in your presets.');
        }
    };

    const loadAndRender = () => {
        let stored = localStorage.getItem(STORAGE_KEY);
        let presets = [];
        if (!stored) {
            const oldStored = localStorage.getItem(OLD_STORAGE_KEY);
            const oldPresets = oldStored ? JSON.parse(oldStored) : [];
            presets = [...new Set([...DEFAULT_PRESETS, ...oldPresets].map(c => normalizeHex(c)))];
            localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
        } else {
            presets = JSON.parse(stored).map(c => normalizeHex(c));
        }
        renderPresets(
            presets,
            (hex) => handlePresetClick(hex),
            (index) => {
                presets.splice(index, 1);
                localStorage.setItem(STORAGE_KEY, JSON.stringify(presets));
                loadAndRender();
            },
            handleAddColor,
        );
    };

    loadAndRender();

    ui.brightnessSlider.addEventListener('input', e => {
        ui.brightnessValue.textContent = `${e.target.value}%`;
        deviceAPI.setBrightness(e.target.value);
        highlightActiveBrightnessChip(parseInt(e.target.value));
    });

    document.querySelectorAll('#brightnessPresets .chip').forEach(chip => {
        chip.addEventListener('click', () => {
            const val = chip.dataset.brightness;
            ui.brightnessSlider.value = val;
            ui.brightnessValue.textContent = `${val}%`;
            deviceAPI.setBrightness(val);
            highlightActiveBrightnessChip(parseInt(val));
        });
    });

    function highlightActiveBrightnessChip(val) {
        document.querySelectorAll('#brightnessPresets .chip').forEach(c => {
            c.classList.toggle('active', parseInt(c.dataset.brightness) === val);
        });
    }

    ui.speedSlider.addEventListener('input', e => {
        ui.speedValue.textContent = `${e.target.value}%`;
        const maxD = 15, minD = 0.8;
        const maxV = parseInt(e.target.max, 10) || 100;
        const dur = maxD - ((parseInt(e.target.value) / maxV) * (maxD - minD));
        deviceAPI.setSpeed(e.target.value);
        document.querySelectorAll('.pattern-preview').forEach(preview => {
            if (window.getComputedStyle(preview).animationName !== 'none') {
                preview.style.animationDuration = `${dur}s`;
            }
        });
    });

    ui.syncTimeBtn.addEventListener('click', deviceAPI.syncTime);
    ui.setRgbOrderBtn.addEventListener('click', () => {
        deviceAPI.setRgbOrder(parseInt(ui.wire1.value), parseInt(ui.wire2.value), parseInt(ui.wire3.value));
    });
    ui.setScheduleBtn.addEventListener('click', () => deviceAPI.setDeviceSchedule(true));
    ui.clearScheduleBtn.addEventListener('click', () => deviceAPI.setDeviceSchedule(false));

    ui.runPatternBtn.addEventListener('click', () => { if (ui.patternSelector.value) deviceAPI.runPattern(ui.patternSelector.value); });
    ui.stopPatternBtn.addEventListener('click', deviceAPI.stopPattern);

    ui.loadPatternBtn.addEventListener('click', () => {
        if (ui.editorPatternSelector.value) deviceAPI.getPatternCode(ui.editorPatternSelector.value);
    });
    ui.newPatternBtn.addEventListener('click', () => {
        ui.editorFilename.value = 'new-pattern.lua';
        ui.editorFilename.focus();
    });
    ui.savePatternBtn.addEventListener('click', () => {
        const filename = ui.editorFilename.value.trim();
        if (!filename || !filename.endsWith('.lua')) {
            alert('Filename is invalid. It must not be empty and must end with .lua');
            return;
        }
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
        const isEditing = !!ui.scheduleEditId;
        const isSimple = ui.cronSimpleMode && ui.cronSimpleMode.style.display !== 'none';

        if (isEditing) {
            const spec = ui.scheduleSpec.value.trim();
            const command = ui.scheduleCommand.value.trim();
            if (!spec || !command) { alert('Please provide both a cron spec and a command.'); return; }
            deviceAPI.updateSchedule(ui.scheduleEditId, spec, command);
            clearScheduleEditMode();
            return;
        }

        if (isSimple) {
            const hour = parseInt(ui.cronHour.value);
            const minute = parseInt(ui.cronMinute.value);
            const everyDay = ui.cronEveryDay && ui.cronEveryDay.checked;
            const checkedDays = [...document.querySelectorAll('input[name="cronDay"]:checked')].map(cb => parseInt(cb.value));

            if (!everyDay && checkedDays.length === 0) {
                alert('Please select at least one day, or check "Every Day".');
                return;
            }

            // Map 0=Mon … 6=Sun -> cron DOW (Mon=1,Tue=2,…,Sun=0)
            const cronDOW = { 0:'1', 1:'2', 2:'3', 3:'4', 4:'5', 5:'6', 6:'0' };
            const dowPart = everyDay ? '*' : checkedDays.map(d => cronDOW[d]).join(',');
            const spec = `${minute} ${hour} * * ${dowPart}`;

            let command = ui.cronCommandType.value;
            if (command === 'pattern') {
                const patternName = ui.cronPatternSelect.value;
                if (!patternName) { alert('Please select a pattern.'); return; }
                command = `pattern ${patternName}`;
            }
            deviceAPI.addSchedule(spec, command);
        } else {
            const spec = ui.scheduleSpec.value.trim();
            const command = ui.scheduleCommand.value.trim();
            if (spec && command) deviceAPI.addSchedule(spec, command);
            else alert('Please provide both a cron spec and a command.');
        }
    });

    if (ui.cronTabSimple && ui.cronTabAdvanced) {
        [ui.cronTabSimple, ui.cronTabAdvanced].forEach(tab => {
            tab.addEventListener('click', () => {
                const mode = tab.dataset.mode;
                ui.cronTabSimple.classList.toggle('active', mode === 'simple');
                ui.cronTabAdvanced.classList.toggle('active', mode === 'advanced');
                ui.cronSimpleMode.style.display = mode === 'simple' ? '' : 'none';
                ui.cronAdvancedMode.style.display = mode === 'advanced' ? '' : 'none';
            });
        });
    }

    if (ui.cronEveryDay) {
        ui.cronEveryDay.addEventListener('change', () => {
            const checked = ui.cronEveryDay.checked;
            document.querySelectorAll('input[name="cronDay"]').forEach(cb => {
                cb.checked = false;
                cb.disabled = checked;
            });
        });
    }
    document.querySelectorAll('input[name="cronDay"]').forEach(cb => {
        cb.addEventListener('change', () => {
            if (ui.cronEveryDay && cb.checked) ui.cronEveryDay.checked = false;
        });
    });

    if (ui.cronCommandType) {
        ui.cronCommandType.addEventListener('change', () => {
            ui.cronPatternSelect.style.display = ui.cronCommandType.value === 'pattern' ? '' : 'none';
        });
    }

    if (ui.cancelScheduleEditBtn) {
        ui.cancelScheduleEditBtn.addEventListener('click', () => clearScheduleEditMode());
    }

    if (ui.pauseAllSchedulesBtn) ui.pauseAllSchedulesBtn.addEventListener('click', () => deviceAPI.setAllSchedulesEnabled(false));
    if (ui.resumeAllSchedulesBtn) ui.resumeAllSchedulesBtn.addEventListener('click', () => deviceAPI.setAllSchedulesEnabled(true));

    if (ui.scheduleList) {
        ui.scheduleList.addEventListener('click', e => {
            const btn = e.target.closest('button');
            if (!btn) return;
            const id = btn.dataset.id;
            if (!id) return;

            if (btn.classList.contains('remove-schedule-btn')) {
                if (confirm('Remove this schedule?')) {
                    deviceAPI.removeSchedule(id);
                    if (ui.scheduleEditId && String(ui.scheduleEditId) === String(id)) clearScheduleEditMode();
                }
                return;
            }
            if (btn.classList.contains('schedule-toggle-btn')) {
                deviceAPI.setScheduleEnabled(id, btn.dataset.enabled !== 'true');
                return;
            }
            if (btn.classList.contains('schedule-run-btn')) {
                deviceAPI.runScheduleNow(id);
                return;
            }
            if (btn.classList.contains('schedule-edit-btn')) {
                enterScheduleEditMode(id, btn.dataset.spec, btn.dataset.command);
            }
        });

        ui.scheduleList.addEventListener('change', e => {
            const input = e.target;
            if (!input || !input.classList.contains('schedule-toggle-input')) return;
            const id = input.dataset.id;
            if (!id) return;
            deviceAPI.setScheduleEnabled(id, input.checked);
        });
    }

    ui.darkModeToggle.addEventListener('click', toggleDarkMode);
}
