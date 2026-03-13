// web/js/ui.js

import { HARDWARE_PATTERNS } from './constants.js';
import { pad } from './utils.js';

// ──────────────────────────────────────────────────────────────
// DOM element registry
// ──────────────────────────────────────────────────────────────
export const ui = {
    // Top bar
    statusPill:              document.getElementById('statusPill'),
    statusDot:               document.getElementById('statusDot'),
    statusText:              document.getElementById('statusText'),
    rssiPill:                document.getElementById('rssiPill'),
    rssiText:                document.getElementById('rssiText'),
    darkModeToggle:          document.getElementById('darkModeToggle'),
    sidebarToggle:           document.getElementById('sidebarToggle'),
    offlineOverlay:          document.getElementById('offlineOverlay'),

    // Power
    powerOnBtn:              document.getElementById('powerOnBtn'),
    powerOffBtn:             document.getElementById('powerOffBtn'),

    // Color
    colorPickerContainer:    document.getElementById('colorPickerContainer'),
    customPresetsContainer:  document.getElementById('customPresetsContainer'),
    brightnessSlider:        document.getElementById('brightnessSlider'),
    brightnessValue:         document.getElementById('brightnessValue'),

    // Effects
    patternGrid:             document.getElementById('patternGrid'),
    speedSlider:             document.getElementById('speedSlider'),
    speedValue:              document.getElementById('speedValue'),

    // Lua
    patternSelector:         document.getElementById('patternSelector'),
    runPatternBtn:           document.getElementById('runPatternBtn'),
    stopPatternBtn:          document.getElementById('stopPatternBtn'),
    patternStatus:           document.getElementById('patternStatus'),

    // Editor
    editorPatternSelector:   document.getElementById('editorPatternSelector'),
    loadPatternBtn:          document.getElementById('loadPatternBtn'),
    newPatternBtn:           document.getElementById('newPatternBtn'),
    savePatternBtn:          document.getElementById('savePatternBtn'),
    deletePatternBtn:        document.getElementById('deletePatternBtn'),
    editorFilename:          document.getElementById('editorFilename'),

    // Scheduler
    cronTabSimple:           document.getElementById('cronTabSimple'),
    cronTabAdvanced:         document.getElementById('cronTabAdvanced'),
    cronSimpleMode:          document.getElementById('cronSimpleMode'),
    cronAdvancedMode:        document.getElementById('cronAdvancedMode'),
    cronHour:                document.getElementById('cronHour'),
    cronMinute:              document.getElementById('cronMinute'),
    cronEveryDay:            document.getElementById('cronEveryDay'),
    cronCommandType:         document.getElementById('cronCommandType'),
    cronPatternSelect:       document.getElementById('cronPatternSelect'),
    scheduleSpec:            document.getElementById('scheduleSpec'),
    scheduleCommand:         document.getElementById('scheduleCommand'),
    addScheduleBtn:          document.getElementById('addScheduleBtn'),
    scheduleEditBar:         document.getElementById('scheduleEditBar'),
    scheduleEditLabel:       document.getElementById('scheduleEditLabel'),
    cancelScheduleEditBtn:   document.getElementById('cancelScheduleEditBtn'),
    pauseAllSchedulesBtn:    document.getElementById('pauseAllSchedulesBtn'),
    resumeAllSchedulesBtn:   document.getElementById('resumeAllSchedulesBtn'),
    scheduleList:            document.getElementById('scheduleList'),

    // Advanced
    syncTimeBtn:             document.getElementById('syncTimeBtn'),
    setScheduleBtn:          document.getElementById('setScheduleBtn'),
    clearScheduleBtn:        document.getElementById('clearScheduleBtn'),
    scheduleHour:            document.getElementById('scheduleHour'),
    scheduleMinute:          document.getElementById('scheduleMinute'),
    scheduleSecond:          document.getElementById('scheduleSecond'),
    scheduleAction:          document.getElementById('scheduleAction'),
    setRgbOrderBtn:          document.getElementById('setRgbOrderBtn'),
    wire1:                   document.getElementById('wire1'),
    wire2:                   document.getElementById('wire2'),
    wire3:                   document.getElementById('wire3'),

    // CodeMirror & iro.js instances (set later)
    codeEditor:              null,
    colorPicker:             null,
    scheduleEditId:          null,
};

// ──────────────────────────────────────────────────────────────
// Navigation
// ──────────────────────────────────────────────────────────────
export function navigateTo(sectionId) {
    document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
    document.querySelectorAll('.nav-item').forEach(b => {
        b.classList.toggle('active', b.dataset.section === sectionId);
        b.setAttribute('aria-selected', b.dataset.section === sectionId ? 'true' : 'false');
    });
    const target = document.getElementById(sectionId);
    if (target) target.classList.add('active');
    if (sectionId === 'sectionColor' && ui.colorPicker) {
        requestAnimationFrame(() => ui.colorPicker.resize(230));
    }
}

export function initNavigation() {
    document.querySelectorAll('.nav-item').forEach(btn => {
        btn.addEventListener('click', () => navigateTo(btn.dataset.section));
    });
}

// ──────────────────────────────────────────────────────────────
// Sidebar toggle
// ──────────────────────────────────────────────────────────────
export function initSidebarToggle() {
    const sidebar = document.getElementById('sidebar');
    if (!ui.sidebarToggle || !sidebar) return;
    const key = 'sidebar_collapsed';
    if (localStorage.getItem(key) === 'true') sidebar.classList.add('collapsed');

    ui.sidebarToggle.addEventListener('click', () => {
        sidebar.classList.toggle('collapsed');
        localStorage.setItem(key, sidebar.classList.contains('collapsed'));
    });
}

// ──────────────────────────────────────────────────────────────
// Dark mode
// ──────────────────────────────────────────────────────────────
export function initDarkMode() {
    // Default is dark – only switch to light if explicitly saved
    if (localStorage.getItem('darkMode') === 'false') {
        document.body.classList.remove('dark-mode');
        document.body.classList.add('light-mode');
    }
    updateDarkModeIcon();
}

function updateDarkModeIcon() {
    const isDark = document.body.classList.contains('dark-mode');
    const icon = ui.darkModeToggle.querySelector('.material-icons-round');
    if (icon) icon.textContent = isDark ? 'light_mode' : 'dark_mode';
}

export function toggleDarkMode() {
    const isDark = document.body.classList.contains('dark-mode');
    document.body.classList.toggle('dark-mode', !isDark);
    document.body.classList.toggle('light-mode', isDark);
    localStorage.setItem('darkMode', !isDark);
    updateDarkModeIcon();
}

// ──────────────────────────────────────────────────────────────
// Connection status
// ──────────────────────────────────────────────────────────────
const STATUS_CLASSES = ['connecting', 'disconnected', 'agent-connected', 'device-disconnected', 'connected'];

export function setStatus(cssClass, message) {
    ui.statusPill.classList.remove(...STATUS_CLASSES);
    ui.statusPill.classList.add(cssClass);
    ui.statusText.textContent = message;
}

export function setRSSI(rssi) {
    if (rssi && rssi !== 0) {
        ui.rssiText.textContent = `${rssi} dBm`;
        ui.rssiPill.style.display = 'flex';
    } else {
        ui.rssiPill.style.display = 'none';
    }
}

// ──────────────────────────────────────────────────────────────
// Offline overlay
// ──────────────────────────────────────────────────────────────
export function showControls(visible) {
    if (visible) {
        ui.offlineOverlay.classList.add('hidden');
    } else {
        ui.offlineOverlay.classList.remove('hidden');
    }
}

// ──────────────────────────────────────────────────────────────
// Color Picker (iro.js)
// ──────────────────────────────────────────────────────────────
export function initColorPicker() {
    const Iro = window.iro;
    if (!Iro || !Iro.ColorPicker) {
        if (ui.colorPickerContainer) {
            ui.colorPickerContainer.innerHTML = '<div class="hint">Color picker failed to load.</div>';
        }
        console.warn('iro.js is not available on window.iro');
        ui.colorPicker = null;
        return null;
    }

    ui.colorPicker = new Iro.ColorPicker('#colorPickerContainer', {
        width: 230,
        color: '#ff0000',
        borderWidth: 2,
        borderColor: 'rgba(255,255,255,0.15)',
        layout: [
            { component: Iro.ui.Wheel, options: {} },
        ],
    });
    requestAnimationFrame(() => ui.colorPicker.resize(230));
    return ui.colorPicker;
}

// ──────────────────────────────────────────────────────────────
// CodeMirror
// ──────────────────────────────────────────────────────────────
export function initCodeMirror() {
    ui.codeEditor = CodeMirror(document.getElementById('codeEditor'), {
        mode:              'lua',
        theme:             'material-darker',
        lineNumbers:       false,
        matchBrackets:     true,
        autoCloseBrackets: true,
        styleActiveLine:   true,
        lineWrapping:      true,
        indentUnit:        4,
        tabSize:           4,
    });
    return ui.codeEditor;
}

// ──────────────────────────────────────────────────────────────
// Time pickers
// ──────────────────────────────────────────────────────────────
export function populateTimePickers() {
    for (let i = 0; i < 60; i++) {
        if (i < 24) ui.scheduleHour.add(new Option(pad(i), i));
        ui.scheduleMinute.add(new Option(pad(i), i));
        ui.scheduleSecond.add(new Option(pad(i), i));
    }
}

export function populateCronTimePickers() {
    for (let i = 0; i < 24; i++) ui.cronHour.add(new Option(pad(i), i));
    for (let i = 0; i < 60; i += 5) ui.cronMinute.add(new Option(pad(i), i));
    ui.cronHour.value   = 22;
    ui.cronMinute.value = 0;
}

// ──────────────────────────────────────────────────────────────
// Cron mode tabs
// ──────────────────────────────────────────────────────────────
function setCronMode(mode) {
    if (!ui.cronTabSimple || !ui.cronTabAdvanced) return;
    ui.cronTabSimple.classList.toggle('active',  mode === 'simple');
    ui.cronTabAdvanced.classList.toggle('active', mode === 'advanced');
    if (ui.cronSimpleMode)   ui.cronSimpleMode.style.display   = mode === 'simple'   ? '' : 'none';
    if (ui.cronAdvancedMode) ui.cronAdvancedMode.style.display = mode === 'advanced' ? '' : 'none';
}

// ──────────────────────────────────────────────────────────────
// Schedule edit mode
// ──────────────────────────────────────────────────────────────
export function enterScheduleEditMode(id, spec, command) {
    ui.scheduleEditId = id;
    if (ui.scheduleSpec)    ui.scheduleSpec.value    = spec    || '';
    if (ui.scheduleCommand) ui.scheduleCommand.value = command || '';
    if (ui.addScheduleBtn)  ui.addScheduleBtn.innerHTML = `<span class="material-icons-round">edit</span> Update Schedule`;
    if (ui.scheduleEditLabel) ui.scheduleEditLabel.textContent = `Editing schedule #${id}`;
    if (ui.scheduleEditBar)   ui.scheduleEditBar.style.display = '';
    setCronMode('advanced');
    // navigate to scheduler
    navigateTo('sectionScheduler');
}

export function clearScheduleEditMode() {
    ui.scheduleEditId = null;
    if (ui.addScheduleBtn) ui.addScheduleBtn.innerHTML = `<span class="material-icons-round">add_alarm</span> Add Schedule`;
    if (ui.scheduleEditBar) ui.scheduleEditBar.style.display = 'none';
}

// ──────────────────────────────────────────────────────────────
// Hardware pattern grid
// ──────────────────────────────────────────────────────────────
export function renderHardwarePatterns(setHardwarePatternApi) {
    ui.patternGrid.innerHTML = '';
    HARDWARE_PATTERNS.forEach(p => {
        const btn = document.createElement('button');
        btn.className = 'pattern-button';
        btn.title     = p.name;
        btn.innerHTML = `
            <div class="pattern-preview ${p.animClass || ''}" style="${p.style || ''}"></div>
            <span class="pattern-name">${p.name}</span>`;
        btn.onclick = () => setHardwarePatternApi(p.id);
        ui.patternGrid.appendChild(btn);
    });
}

// ──────────────────────────────────────────────────────────────
// Pattern list dropdowns + cron pattern select
// ──────────────────────────────────────────────────────────────
export function updatePatternLists(patterns) {
    const fill = (sel) => {
        const cur = sel.value;
        sel.innerHTML = '';
        if (patterns && patterns.length > 0) {
            patterns.forEach(name => sel.add(new Option(name, name)));
            sel.value = patterns.includes(cur) ? cur : patterns[0];
        } else {
            sel.innerHTML = '<option disabled>No patterns found</option>';
        }
    };
    fill(ui.patternSelector);
    fill(ui.editorPatternSelector);
    if (ui.cronPatternSelect) fill(ui.cronPatternSelect);
}

// ──────────────────────────────────────────────────────────────
// Schedule list
// ──────────────────────────────────────────────────────────────
export function updateScheduleList(schedules) {
    ui.scheduleList.innerHTML = '';
    const ids = schedules ? Object.keys(schedules) : [];

    if (ui.scheduleEditId && !ids.includes(String(ui.scheduleEditId))) {
        clearScheduleEditMode();
    }

    if (ids.length === 0) {
        ui.scheduleList.innerHTML = '<li class="schedule-empty">No schedules defined.</li>';
        return;
    }

    ids.forEach(id => {
        const item = schedules[id];
        const li   = document.createElement('li');
        li.className = 'schedule-item';
        if (!item.enabled) li.classList.add('schedule-paused');

        const info = document.createElement('div');
        info.className = 'schedule-info';

        const commandType = getCommandType(item.command);
        const top = document.createElement('div');
        top.className = 'schedule-top';

        const command = document.createElement('div');
        command.className = 'schedule-command';
        command.textContent = item.command;

        const type = document.createElement('span');
        type.className = 'schedule-type';
        type.textContent = commandType;

        const toggleLabel = document.createElement('label');
        toggleLabel.className = 'schedule-toggle';
        toggleLabel.title = item.enabled ? 'Disable schedule' : 'Enable schedule';
        toggleLabel.innerHTML = `
            <input class="schedule-toggle-input" type="checkbox" aria-label="Schedule enabled">
            <span class="schedule-toggle-slider"></span>`;
        const toggleInput = toggleLabel.querySelector('input');
        toggleInput.checked = !!item.enabled;
        toggleInput.dataset.id = id;

        top.append(command, type, toggleLabel);

        const middle = document.createElement('div');
        middle.className = 'schedule-middle';
        middle.innerHTML = `
            <span class="schedule-cron">
                <span class="material-icons-round">schedule</span>
                ${item.spec}
            </span>
            <span class="schedule-human">Runs automatically</span>`;

        const bottom = document.createElement('div');
        bottom.className = 'schedule-bottom';

        const meta = document.createElement('div');
        meta.className = 'schedule-meta';
        meta.innerHTML = `
            <span class="schedule-meta-item">
                <span class="material-icons-round">event</span>
                ${item.enabled && item.next_run ? `Next: ${formatRunTime(item.next_run)}` : 'Next: —'}
            </span>
            <span class="schedule-meta-item">
                <span class="material-icons-round">history</span>
                ${item.last_run ? `Last: ${formatRunTime(item.last_run)}` : 'Last: —'}
            </span>`;

        const actions = document.createElement('div');
        actions.className = 'schedule-actions';

        const makeBtn = (extraClass, icon, title, dataAttrs) => {
            const b = document.createElement('button');
            b.className = `schedule-btn ${extraClass}`;
            b.title = title;
            b.dataset.id = id;
            Object.assign(b.dataset, dataAttrs || {});
            b.innerHTML = `<span class="material-icons-round">${icon}</span>`;
            return b;
        };

        const runBtn  = makeBtn('schedule-run-btn',  'play_circle', 'Run now');
        const editBtn = makeBtn('schedule-edit-btn', 'edit',        'Edit schedule',
            { spec: item.spec, command: item.command });
        const delBtn  = makeBtn('remove-schedule-btn', 'delete', 'Remove schedule');

        actions.append(runBtn, editBtn, delBtn);
        bottom.append(meta, actions);
        info.append(top, middle, bottom);
        li.append(info);
        ui.scheduleList.appendChild(li);
    });
}

function formatRunTime(value) {
    const date = new Date(value);
    return isNaN(date.getTime()) ? value : date.toLocaleString();
}

function getCommandType(command) {
    const cmd = String(command || '').trim().toLowerCase();
    if (cmd.startsWith('power on')) return 'Power On';
    if (cmd.startsWith('power off')) return 'Power Off';
    if (cmd.startsWith('pattern')) return 'Pattern';
    if (cmd.startsWith('lua')) return 'Lua';
    return 'Command';
}

// ──────────────────────────────────────────────────────────────
// Color presets
// ──────────────────────────────────────────────────────────────
export function renderPresets(presets, onApply, onDelete, onAdd) {
    ui.customPresetsContainer.innerHTML = '';

    if (presets) {
        presets.forEach((hex, index) => {
            const btn = document.createElement('button');
            btn.className           = 'preset-swatch';
            btn.style.backgroundColor = hex;
            btn.title               = hex;
            btn.dataset.color       = hex;

            let pressTimer    = null;
            let isLongPress   = false;

            btn.addEventListener('contextmenu', e => {
                e.preventDefault();
                if (!isLongPress && confirm(`Delete preset ${hex}?`)) onDelete(index);
            });

            btn.addEventListener('touchstart', () => {
                isLongPress = false;
                pressTimer  = setTimeout(() => {
                    isLongPress = true;
                    if (navigator.vibrate) navigator.vibrate(50);
                    if (confirm(`Delete preset ${hex}?`)) onDelete(index);
                }, 600);
            }, { passive: true });

            const cancelPress = () => { if (pressTimer) clearTimeout(pressTimer); };
            btn.addEventListener('touchend',  cancelPress);
            btn.addEventListener('touchmove', cancelPress);

            btn.addEventListener('click', e => {
                if (isLongPress) { e.preventDefault(); e.stopPropagation(); return; }
                onApply(hex);
            });

            ui.customPresetsContainer.appendChild(btn);
        });
    }

    const addBtn = document.createElement('button');
    addBtn.className = 'preset-add-btn';
    addBtn.title     = 'Save current color';
    addBtn.innerHTML = '<span class="material-icons-round">add</span>';
    addBtn.addEventListener('click', onAdd);
    ui.customPresetsContainer.appendChild(addBtn);
}
