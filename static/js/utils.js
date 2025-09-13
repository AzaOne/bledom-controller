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
