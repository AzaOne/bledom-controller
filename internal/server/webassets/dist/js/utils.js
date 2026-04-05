// web/js/utils.js

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
 * @param {string} hex The hex string to normalize.
 * @returns {string} The normalized 6-digit hex string (e.g., "#FF0000").
 */
export function normalizeHex(hex) {
    if (!hex) return '';
    hex = hex.trim().toUpperCase();

    if (hex.startsWith('#')) hex = hex.substring(1);

    if (hex.length === 3) {
        hex = hex.split('').map(c => c + c).join('');
    } else if (hex.length === 4) {
        hex = hex.substring(0, 3).split('').map(c => c + c).join('');
    } else if (hex.length > 6) {
        hex = hex.substring(0, 6);
    } else if (hex.length < 6) {
        hex = hex.padEnd(6, '0');
    }

    return '#' + hex;
}
