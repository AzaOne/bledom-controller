-- effect-showcase.lua: Demonstrates the new built-in effect functions.

print("Starting effect showcase...")
set_power(true)
set_brightness(100)

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
end
