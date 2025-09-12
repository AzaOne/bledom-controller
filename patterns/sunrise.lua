-- Sunrise / Sunset Effect (2 transitions per day)
-- Night = red @ 5% brightness
-- Day   = green @ 100% brightness
-- Runs only once, fade depends on current time

set_power(true)

local hour = tonumber(os.date("%H"))

if hour == 7 then
    -- Morning: fade from Red 5% -> Green 100%
    print("Morning transition: Red → Green")
    set_color(255, 0, 0)
    set_brightness(5)
    fade(255, 0, 0, 0, 255, 0, 300000) -- 5 min fade
    set_brightness(100)

elseif hour == 22 then
    -- Evening: fade from Green 100% -> Red 5%
    print("Evening transition: Green → Red")
    set_color(0, 255, 0)
    set_brightness(100)
    fade(0, 255, 0, 255, 0, 0, 300000) -- 5 min fade
    set_brightness(5)

else
    -- Idle: just set correct state if launched at another time
    if hour > 7 and hour < 22 then
        set_color(0, 255, 0)
        set_brightness(100)
        print(string.format("Daytime | Hour: %d | Green 100%%", hour))
    else
        set_color(255, 0, 0)
        set_brightness(5)
        print(string.format("Nighttime | Hour: %d | Red 5%%", hour))
    end
end
