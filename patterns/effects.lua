-- effects.lua: Demonstrates the new built-in effect functions.

print("Starting effect showcase...")
set_power(true)
set_brightness(100) -- Ensure brightness is 100% at start for some effects

while true do
    -- If user stopped us during the last effect, exit now.
    if should_stop() then return end

    print("Fading from Red to Blue over 3 seconds")
    fade(255, 0, 0, 0, 0, 255, 3000)
    sleep(1000)

    if should_stop() then return end

    print("Breathing Green for 4 seconds")
    set_color(0, 255, 0)
    breathe(4000)
    sleep(1000)

    if should_stop() then return end

    print("Strobe White for 2 seconds at 5Hz")
    strobe(255, 255, 255, 2000, 5)
    sleep(1000)

    if should_stop() then return end
    
    print("Fading from Blue back to Red over 3 seconds")
    fade(0, 0, 255, 255, 0, 0, 3000)
    sleep(1000)

    -- === NEW EFFECT DEMO: fade_brightness ===
    if should_stop() then return end

    print("Fading brightness IN (1% to 100%) for Yellow over 2 seconds")
    set_color(255, 255, 0) -- Set a color (e.g., yellow) before fading brightness
    fade_brightness(1, 100, 2000)
    sleep(1000)

    if should_stop() then return end

    print("Fading brightness OUT (100% to 20%) for Cyan over 2.5 seconds")
    set_color(0, 255, 255) -- Set a new color (e.g., cyan)
    fade_brightness(100, 20, 2500)
    sleep(1000)
end
