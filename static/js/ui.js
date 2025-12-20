// static/js/ui.js

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

    // CodeMirror instance will be stored here
    codeEditor: null,
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
 * Populates the hour, minute, and second dropdowns for scheduling.
 */
export function populateTimePickers() {
    for (let i = 0; i < 60; i++) {
        if (i < 24) ui.scheduleHour.add(new Option(pad(i), i));
        ui.scheduleMinute.add(new Option(pad(i), i));
        ui.scheduleSecond.add(new Option(pad(i), i));
    }
}

/**
 * Updates the Lua pattern selection dropdowns.
 * @param {string[]} patterns An array of pattern filenames.
 */
export function updatePatternLists(patterns) {
    const createOptions = (selectElem) => {
        const currentVal = selectElem.value;
        selectElem.innerHTML = '';
         if (patterns && patterns.length > 0) {
            patterns.forEach(name => selectElem.add(new Option(name, name)));
            if(patterns.includes(currentVal)) selectElem.value = currentVal;
            else if (patterns.length > 0) selectElem.value = patterns[0]; // Select first if current not found
        } else {
            selectElem.innerHTML = '<option disabled>No patterns found</option>';
        }
    };
    createOptions(ui.patternSelector);
    createOptions(ui.editorPatternSelector);
}

/**
 * Updates the list of agent-side schedules.
 * @param {Object} schedules An object mapping schedule IDs to schedule entries.
 * @param {Function} removeScheduleApi A function to call when a remove button is clicked.
 */
export function updateScheduleList(schedules, removeScheduleApi) {
    ui.scheduleList.innerHTML = '';
    if (schedules && Object.keys(schedules).length > 0) {
        for (const id in schedules) {
            const item = schedules[id];
            const li = document.createElement('li');
            li.innerHTML = `<span><code>${item.spec}</code> &rarr; <code>${item.command}</code></span>
                          <button class="remove-schedule-btn" data-id="${id}" style="background-color:var(--warn-color); padding: 5px 10px; font-size: 0.8em;">Remove</button>`;
            ui.scheduleList.appendChild(li);
        }
    } else {
        ui.scheduleList.innerHTML = '<li>No schedules defined.</li>';
    }
    // Re-attach listeners for dynamically added buttons
    ui.scheduleList.querySelectorAll('.remove-schedule-btn').forEach(button => {
        button.onclick = (e) => {
            const id = e.target.dataset.id;
            if (confirm(`Are you sure you want to remove this schedule?`)) removeScheduleApi(id);
        };
    });
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
