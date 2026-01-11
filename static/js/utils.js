// static/js/utils.js

const debounceTimeouts = {};

/**
 * Debounces a function call.
 * @param {Function} func The function to debounce.
 * @param {Array} args Arguments to pass to the function.
 * @param {string} key A unique key for this debounce.
 * @param {number} delay The debounce delay in milliseconds.
 */
export function debounce(func, args, key, delay = 50) {
    clearTimeout(debounceTimeouts[key]);
    debounceTimeouts[key] = setTimeout(() => func(...args), delay);
}

/**
 * Pads a number with a leading zero if it's less than 10.
 * @param {number} num The number to pad.
 * @returns {string} The padded number as a string.
 */
export function pad(num) {
    return String(num).padStart(2, '0');
}

/**
 * Normalizes a hex color string to a 6-digit uppercase hex code.
 * Handles:
 * - Missing hash (adds it) (not strictly needed for iro.js but good for robustness)
 * - 3-digit hex (#RGB -> #RRGGBB)
 * - 4-digit hex (#RGBA -> #RRGGBB, strips alpha)
 * - 8-digit hex (#RRGGBBAA -> #RRGGBB, strips alpha)
 * - Case insensitivity (converts to uppercase)
 * @param {string} hex The hex string to normalize.
 * @returns {string} The normalized 6-digit hex string (e.g., "#FF0000").
 */
export function normalizeHex(hex) {
    if (!hex) return '';
    hex = hex.trim().toUpperCase();

    if (hex.startsWith('#')) {
        hex = hex.substring(1);
    }

    // Handle short hex codes
    if (hex.length === 3) {
        hex = hex.split('').map(char => char + char).join('');
    } else if (hex.length === 4) {
        // #RGBA -> #RRGGBB (strip A)
        hex = hex.substring(0, 3).split('').map(char => char + char).join('');
    } else if (hex.length > 6) {
        // #RRGGBBAA -> #RRGGBB (strip AA)
        hex = hex.substring(0, 6);
    } else if (hex.length < 6) {
        // Pad with 0 if it's some weird partial hex? Or just return as is?
        // For safety, let's just pad end with 0s if it's 5 digits or something weird,
        // but 3 and 6 are the main valid ones.
        hex = hex.padEnd(6, '0');
    }

    return '#' + hex;
}
