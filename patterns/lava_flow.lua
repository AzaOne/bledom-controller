-- Lava Flow Effect
math.randomseed(os.time())
set_power(true)

while true do
    if should_stop() then return end

    local base_r = 150 + math.random(105) -- 150–255 red
    local base_g = 30 + math.random(80)   -- 30–110 green
    local base_b = math.random(40)        -- 0–40 blue

    set_color(base_r, base_g, base_b)

    for b = 60, 100, 4 do
        if should_stop() then return end
        set_brightness(b)
        sleep(50)
    end
    for b = 100, 60, -4 do
        if should_stop() then return end
        set_brightness(b)
        sleep(50)
    end
end

