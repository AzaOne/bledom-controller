-- ocean_wave.lua: A gentle blue and green color cycle that mimics waves.

print("Starting ocean wave pattern...")

set_power(true)

while true do
    if should_stop() then return end
    for i = 0, 255, 5 do
        if should_stop() then return end
        set_color(0, 100 + (i % 155), 200) -- green/blue mix
        set_brightness(40 + (i % 60))      -- wave swell
        sleep(80)
    end
end

