-- Sunrise / Sunset Effect (3 transitions per day)
-- Night = red @ 5% brightness
-- Day   = green @ 100% brightness
-- Dusk  = green @ 25% brightness
-- Runs only once, fade depends on current time

set_power(true)

local hour = tonumber(os.date("%H"))
local min = tonumber(os.date("%M"))
local current_time_in_minutes = hour * 60 + min

-- Define transition times in minutes from midnight
local morning_transition = 7 * 60   -- 7:00
local dusk_transition = 21 * 60     -- 21:00
local evening_transition = 23 * 60  -- 23:00

if current_time_in_minutes == morning_transition then
    -- Morning: fade from Red 5% -> Green 100%
    print("Morning transition: Red → Green")
    set_color(255, 0, 0)
    set_brightness(5)
    fade(255, 0, 0, 0, 255, 0, 300000) -- 5 min fade
    set_brightness(100)

elseif current_time_in_minutes == dusk_transition then
    -- Dusk: fade from Green 100% -> Green 25%
    print("Dusk transition: Green 100% → Green 25%")
    set_color(0, 255, 0)
    set_brightness(100)
    fade(0, 255, 0, 0, 255, 0, 300000) -- 5 min fade
    set_brightness(25)

elseif current_time_in_minutes == evening_transition then
    -- Evening: fade from Green 25% -> Red 5%
    print("Evening transition: Green → Red")
    set_color(0, 255, 0)
    set_brightness(25)
    fade(0, 255, 0, 255, 0, 0, 300000) -- 5 min fade
    set_brightness(5)

else
    -- Idle: just set correct state if launched at another time
    if current_time_in_minutes > morning_transition and current_time_in_minutes < dusk_transition then
        set_color(0, 255, 0)
        set_brightness(100)
        print(string.format("Daytime | Time: %02d:%02d | Green 100%%", hour, min))
    elseif current_time_in_minutes > dusk_transition and current_time_in_minutes < evening_transition then
        set_color(0, 255, 0)
        set_brightness(25)
        print(string.format("Dusk | Time: %02d:%02d | Green 25%%", hour, min))
    else
        set_color(255, 0, 0)
        set_brightness(5)
        print(string.format("Nighttime | Time: %02d:%02d | Red 5%%", hour, min))
    end
end