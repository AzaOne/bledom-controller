// web/js/ui.js

import { HARDWARE_PATTERNS } from './constants.js';
import { pad } from './utils.js';

// DOM elements collected into a single object
export const ui = {
    statusDiv: document.getElementById('connectionStatus'),
    controls: document.getElementById('controls'),
    darkModeToggle: document.getElementById('darkModeToggle'),

    powerOnBtn: document.getElementById('powerOnBtn'),
    powerOffBtn: document.getElementById('powerOffBtn'),
    colorPickerContainer: document.getElementById('colorPickerContainer'),
    brightnessSlider: document.getElementById('brightnessSlider'),
    brightnessValue: document.getElementById('brightnessValue'),

    // Presets Container
    customPresetsContainer: document.getElementById('customPresetsContainer'),

    patternGrid: document.getElementById('patternGrid'),
    speedSlider: document.getElementById('speedSlider'),
    speedValue: document.getElementById('speedValue'),

    syncTimeBtn: document.getElementById('syncTimeBtn'),
    setScheduleBtn: document.getElementById('setScheduleBtn'),
    clearScheduleBtn: document.getElementById('clearScheduleBtn'),
    scheduleHour: document.getElementById('scheduleHour'),
    scheduleMinute: document.getElementById('scheduleMinute'),
    scheduleSecond: document.getElementById('scheduleSecond'),
    scheduleAction: document.getElementById('scheduleAction'),
    setRgbOrderBtn: document.getElementById('setRgbOrderBtn'),
    wire1: document.getElementById('wire1'), wire2: document.getElementById('wire2'), wire3: document.getElementById('wire3'),

    patternSelector: document.getElementById('patternSelector'),
    editorPatternSelector: document.getElementById('editorPatternSelector'),
    runPatternBtn: document.getElementById('runPatternBtn'),
    stopPatternBtn: document.getElementById('stopPatternBtn'),
    patternStatus: document.getElementById('patternStatus'),
    loadPatternBtn: document.getElementById('loadPatternBtn'),
    newPatternBtn: document.getElementById('newPatternBtn'),
    savePatternBtn: document.getElementById('savePatternBtn'),
    deletePatternBtn: document.getElementById('deletePatternBtn'),
    editorFilename: document.getElementById('editorFilename'),

    scheduleList: document.getElementById('scheduleList'),
    addScheduleBtn: document.getElementById('addScheduleBtn'),
    scheduleSpec: document.getElementById('scheduleSpec'),
    scheduleCommand: document.getElementById('scheduleCommand'),
    scheduleEditBar: document.getElementById('scheduleEditBar'),
    scheduleEditLabel: document.getElementById('scheduleEditLabel'),
    cancelScheduleEditBtn: document.getElementById('cancelScheduleEditBtn'),
    pauseAllSchedulesBtn: document.getElementById('pauseAllSchedulesBtn'),
    resumeAllSchedulesBtn: document.getElementById('resumeAllSchedulesBtn'),

    // Cron builder
    cronTabSimple: document.getElementById('cronTabSimple'),
    cronTabAdvanced: document.getElementById('cronTabAdvanced'),
    cronSimpleMode: document.getElementById('cronSimpleMode'),
    cronAdvancedMode: document.getElementById('cronAdvancedMode'),
    cronHour: document.getElementById('cronHour'),
    cronMinute: document.getElementById('cronMinute'),
    cronEveryDay: document.getElementById('cronEveryDay'),
    cronCommandType: document.getElementById('cronCommandType'),
    cronPatternSelect: document.getElementById('cronPatternSelect'),

    // CodeMirror instance will be stored here
    codeEditor: null,
    scheduleEditId: null,
};

/**
 * Initializes the iro.js Color Picker.
 */
export function initColorPicker() {
    ui.colorPicker = new iro.ColorPicker("#colorPickerContainer", {
        width: 250,
        color: "#ff0000",
        borderWidth: 2,
        borderColor: "#fff",
        layout: [
            {
                component: iro.ui.Wheel,
                options: {}
            },
            {
                component: iro.ui.Slider,
                options: {
                    sliderType: 'value'
                }
            }
        ]
    });
    return ui.colorPicker;
}

/**
 * Initializes the CodeMirror editor.
 * @returns {CodeMirror.Editor} The CodeMirror editor instance.
 */
export function initCodeMirror() {
    ui.codeEditor = CodeMirror(document.getElementById('codeEditor'), {
        mode: 'lua',
        theme: 'material-darker',
        lineNumbers: false,
        matchBrackets: true,
        autoCloseBrackets: true,
        styleActiveLine: true,
        lineWrapping: true,
        indentUnit: 4,
        tabSize: 4
    });
    return ui.codeEditor;
}

/**
 * Updates the connection status display.
 * @param {string} cssClass CSS class to apply for styling (e.g., 'connected', 'disconnected').
 * @param {string} message Text message to display.
 */
export function setStatus(cssClass, message) {
    const statusClasses = ['connecting', 'disconnected', 'agent-connected', 'device-disconnected', 'connected'];
    ui.statusDiv.classList.remove(...statusClasses);
    ui.statusDiv.classList.add(cssClass);
    ui.statusDiv.textContent = message;
}

/**
 * Renders the hardware pattern buttons based on constants.
 * @param {Function} setHardwarePatternApi A function to call when a pattern button is clicked.
 */
export function renderHardwarePatterns(setHardwarePatternApi) {
    ui.patternGrid.innerHTML = '';
    HARDWARE_PATTERNS.forEach(p => {
        const button = document.createElement('button');
        button.className = 'pattern-button';
        button.title = p.name;
        button.innerHTML = `<div class="pattern-preview ${p.animClass || ''}" style="${p.style || ''}"></div><span class="pattern-name">${p.name}</span>`;
        button.onclick = () => setHardwarePatternApi(p.id);
        ui.patternGrid.appendChild(button);
    });
}

/**
 * Populates the cronHour and cronMinute selects for the simple cron builder.
 */
export function populateCronTimePickers() {
    for (let i = 0; i < 24; i++) {
        ui.cronHour.add(new Option(pad(i), i));
    }
    for (let i = 0; i < 60; i += 5) {
        ui.cronMinute.add(new Option(pad(i), i));
    }
    // Add a default "00" if not already there (first option)
    // The loop already includes 00. Just set a sensible default.
    ui.cronHour.value = 22;
    ui.cronMinute.value = 0;
}

/**
 * Populates the hour, minute, and second dropdowns for scheduling.
 */
export function populateTimePickers() {
    for (let i = 0; i < 60; i++) {
        if (i < 24) ui.scheduleHour.add(new Option(pad(i), i));
        ui.scheduleMinute.add(new Option(pad(i), i));
        ui.scheduleSecond.add(new Option(pad(i), i));
    }
}

function setCronMode(mode) {
    if (!ui.cronTabSimple || !ui.cronTabAdvanced) return;
    ui.cronTabSimple.classList.toggle('active', mode === 'simple');
    ui.cronTabAdvanced.classList.toggle('active', mode === 'advanced');
    if (ui.cronSimpleMode) ui.cronSimpleMode.style.display = mode === 'simple' ? '' : 'none';
    if (ui.cronAdvancedMode) ui.cronAdvancedMode.style.display = mode === 'advanced' ? '' : 'none';
}

export function enterScheduleEditMode(id, spec, command) {
    ui.scheduleEditId = id;
    if (ui.scheduleSpec) ui.scheduleSpec.value = spec || '';
    if (ui.scheduleCommand) ui.scheduleCommand.value = command || '';
    if (ui.addScheduleBtn) {
        ui.addScheduleBtn.innerHTML = `<span class="material-icons" style="vertical-align: middle; font-size: 18px;">edit</span>
            Update Schedule`;
    }
    if (ui.scheduleEditLabel) ui.scheduleEditLabel.textContent = `Editing schedule #${id}`;
    if (ui.scheduleEditBar) ui.scheduleEditBar.style.display = '';
    setCronMode('advanced');
}

export function clearScheduleEditMode() {
    ui.scheduleEditId = null;
    if (ui.addScheduleBtn) {
        ui.addScheduleBtn.innerHTML = `<span class="material-icons" style="vertical-align: middle; font-size: 18px;">add_alarm</span>
            Add Schedule`;
    }
    if (ui.scheduleEditBar) ui.scheduleEditBar.style.display = 'none';
}

/**
 * Updates the Lua pattern selection dropdowns AND the cron builder pattern select.
 * @param {string[]} patterns An array of pattern filenames.
 */
export function updatePatternLists(patterns) {
    const createOptions = (selectElem) => {
        const currentVal = selectElem.value;
        selectElem.innerHTML = '';
        if (patterns && patterns.length > 0) {
            patterns.forEach(name => selectElem.add(new Option(name, name)));
            if (patterns.includes(currentVal)) selectElem.value = currentVal;
            else if (patterns.length > 0) selectElem.value = patterns[0];
        } else {
            selectElem.innerHTML = '<option disabled>No patterns found</option>';
        }
    };
    createOptions(ui.patternSelector);
    createOptions(ui.editorPatternSelector);
    if (ui.cronPatternSelect) createOptions(ui.cronPatternSelect);
}

/**
 * Updates the list of agent-side schedules with styled card rendering.
 * @param {Object} schedules An object mapping schedule IDs to schedule entries.
 * @param {Function} removeScheduleApi A function to call when a remove button is clicked.
 */
export function updateScheduleList(schedules) {
    ui.scheduleList.innerHTML = '';
    const ids = schedules ? Object.keys(schedules) : [];
    if (ui.scheduleEditId && !ids.includes(String(ui.scheduleEditId))) {
        clearScheduleEditMode();
    }
    if (ids.length > 0) {
        ids.forEach(id => {
            const item = schedules[id];
            const li = document.createElement('li');
            li.className = 'schedule-item';
            if (!item.enabled) li.classList.add('schedule-paused');

            const info = document.createElement('div');
            info.className = 'schedule-info';

            const line = document.createElement('div');
            line.className = 'schedule-line';

            const spec = document.createElement('span');
            spec.className = 'schedule-spec';
            spec.textContent = item.spec;

            const command = document.createElement('span');
            command.className = 'schedule-command';
            command.textContent = item.command;

            line.append(spec, command);

            const meta = document.createElement('div');
            meta.className = 'schedule-meta';

            const state = document.createElement('span');
            state.className = `schedule-state${item.enabled ? '' : ' paused'}`;
            state.textContent = item.enabled ? 'Active' : 'Paused';

            const next = document.createElement('span');
            next.className = 'schedule-time';
            next.textContent = item.enabled && item.next_run ? `Next: ${formatRunTime(item.next_run)}` : 'Next: —';

            const last = document.createElement('span');
            last.className = 'schedule-time';
            last.textContent = item.last_run ? `Last: ${formatRunTime(item.last_run)}` : 'Last: —';

            meta.append(state, next, last);
            info.append(line, meta);

            const actions = document.createElement('div');
            actions.className = 'schedule-actions';

            const toggleBtn = document.createElement('button');
            toggleBtn.className = 'schedule-btn schedule-toggle-btn';
            toggleBtn.dataset.id = id;
            toggleBtn.dataset.enabled = item.enabled ? 'true' : 'false';
            toggleBtn.title = item.enabled ? 'Pause schedule' : 'Resume schedule';
            toggleBtn.innerHTML = `<span class="material-icons" style="font-size:16px; vertical-align:middle;">${item.enabled ? 'pause' : 'play_arrow'}</span>`;

            const runBtn = document.createElement('button');
            runBtn.className = 'schedule-btn schedule-run-btn';
            runBtn.dataset.id = id;
            runBtn.title = 'Run now';
            runBtn.innerHTML = `<span class="material-icons" style="font-size:16px; vertical-align:middle;">play_circle</span>`;

            const editBtn = document.createElement('button');
            editBtn.className = 'schedule-btn schedule-edit-btn';
            editBtn.dataset.id = id;
            editBtn.dataset.spec = item.spec;
            editBtn.dataset.command = item.command;
            editBtn.title = 'Edit schedule';
            editBtn.innerHTML = `<span class="material-icons" style="font-size:16px; vertical-align:middle;">edit</span>`;

            const removeBtn = document.createElement('button');
            removeBtn.className = 'schedule-btn remove-schedule-btn';
            removeBtn.dataset.id = id;
            removeBtn.title = 'Remove schedule';
            removeBtn.innerHTML = `<span class="material-icons" style="font-size:16px; vertical-align:middle;">delete</span>`;

            actions.append(toggleBtn, runBtn, editBtn, removeBtn);

            li.append(info, actions);
            ui.scheduleList.appendChild(li);
        });
    } else {
        ui.scheduleList.innerHTML = '<li class="schedule-empty">No schedules defined.</li>';
    }
}

function formatRunTime(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return value;
    return date.toLocaleString();
}

/**
 * Initializes dark mode based on local storage and sets the toggle button icon.
 */
export function initDarkMode() {
    if (localStorage.getItem('darkMode') === 'true') {
        document.body.classList.add('dark-mode');
    }
    const isDark = document.body.classList.contains('dark-mode');
    document.querySelector('#darkModeToggle .material-icons').textContent = isDark ? 'light_mode' : 'dark_mode';
}

/**
 * Renders the custom color preset buttons AND the Add button at the end.
 * Includes Touch Support (Long Press to Delete).
 * @param {string[]} presets Array of hex color strings.
 * @param {Function} onApply Callback when a preset is clicked.
 * @param {Function} onDelete Callback when a preset is right-clicked or long-pressed.
 * @param {Function} onAdd Callback when the + button is clicked.
 */

export function renderPresets(presets, onApply, onDelete, onAdd) {
    ui.customPresetsContainer.innerHTML = '';

    // 1. Render Saved Presets
    if (presets) {
        presets.forEach((hex, index) => {
            const button = document.createElement('button');
            button.style.backgroundColor = hex;
            button.title = `${hex}`;
            button.dataset.color = hex;

            // Add border to light colors if needed
            // const isLight = (hex.toLowerCase() === '#ffffff') || (parseInt(hex.substring(1), 16) > 0xEEEEEE);
            // if (isLight) {
            //    button.style.border = '1px solid #ccc';
            // }

            // --- Interaction Logic (Mouse & Touch) ---
            let pressTimer = null;
            let isLongPress = false;

            // 1. Right Click (Desktop)
            button.addEventListener('contextmenu', (e) => {
                e.preventDefault();
                // Prevent double firing if the browser maps long-press to contextmenu automatically
                if (!isLongPress) {
                    if (confirm(`Delete preset ${hex}?`)) {
                        onDelete(index);
                    }
                }
            });

            // 2. Touch Start (Start Timer)
            button.addEventListener('touchstart', (e) => {
                isLongPress = false;
                pressTimer = setTimeout(() => {
                    isLongPress = true;
                    // Haptic feedback if supported
                    if (navigator.vibrate) navigator.vibrate(50);
                    if (confirm(`Delete preset ${hex}?`)) {
                        onDelete(index);
                    }
                }, 600); // 600ms threshold for long press
            }, { passive: true });

            // 3. Touch End / Move (Cancel Timer)
            const cancelPress = () => {
                if (pressTimer) clearTimeout(pressTimer);
            };
            button.addEventListener('touchend', cancelPress);
            button.addEventListener('touchmove', cancelPress); // Cancel if user tries to scroll

            // 4. Click / Tap (Apply Color)
            button.addEventListener('click', (e) => {
                // If we just triggered a long press delete, ignore this click
                if (isLongPress) {
                    e.preventDefault();
                    e.stopPropagation();
                    return;
                }
                onApply(hex);
            });

            ui.customPresetsContainer.appendChild(button);
        });
    }

    // 2. Render the "+" Button at the end
    const addBtn = document.createElement('button');
    addBtn.innerHTML = '<span class="material-icons" style="font-size: 24px; vertical-align: middle;">add</span>';
    addBtn.title = "Save current color";
    addBtn.style.backgroundColor = "#eee"; // Light gray background
    addBtn.style.color = "#555";
    addBtn.style.border = "1px dashed #aaa"; // Dashed border to distinguish it
    addBtn.style.display = "flex";
    addBtn.style.alignItems = "center";
    addBtn.style.justifyContent = "center";

    // Standard click for Add button is sufficient
    addBtn.addEventListener('click', onAdd);

    // Dark mode style adjustment
    if (document.body.classList.contains('dark-mode')) {
        addBtn.style.backgroundColor = "#444";
        addBtn.style.color = "#ccc";
        addBtn.style.borderColor = "#666";
    }

    ui.customPresetsContainer.appendChild(addBtn);
}
