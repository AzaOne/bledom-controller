-- meteor.lua: Creates a white flash followed by a fading blue trail.

print("Starting meteor pattern...")

set_power(true)

while true do
    if should_stop() then return end

    -- Flash
    set_color(255, 255, 255)
    set_brightness(100)
    sleep(150)

    -- Trail fade
    for b = 100, 20, -5 do
        if should_stop() then return end
        set_brightness(b)
        set_color(200, 200, 255) -- bluish-white trail
        sleep(60)
    end

    -- Pause before next meteor
    sleep(1000 + math.random(1500))
end

