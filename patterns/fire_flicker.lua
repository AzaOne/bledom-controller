-- fire_flicker.lua: Simulates the warm, dancing glow of a fire.

print("Starting fire flicker pattern...")

math.randomseed(os.time())
set_power(true)

while true do
    if should_stop() then return end

    local r = 200 + math.random(55)  -- 200–255 red
    local g = math.random(100)       -- 0–100 green
    local b = 0                      -- no blue
    local bright = 60 + math.random(40) -- 60–100 brightness

    set_color(r, g, b)
    set_brightness(bright)
    sleep(80 + math.random(120)) -- flicker timing
end

