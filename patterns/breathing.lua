-- Breathing Effect
set_power(true)
set_color(255, 0, 50) -- Red-pink

while true do
    for b = 1, 100 do
        if should_stop() then return end
        set_brightness(b)
        sleep(30)
    end
    for b = 100, 1, -1 do
        if should_stop() then return end
        set_brightness(b)
        sleep(30)
    end
end

