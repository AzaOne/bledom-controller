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
local morning_transition = 8 * 60   -- 8:00
local dusk_transition = 21 * 60     -- 21:00
local evening_transition = 23 * 60  -- 23:00

if current_time_in_minutes == morning_transition then
    -- Morning: fade brightness first, then color
    print("Morning transition: Red → Green")
    set_color(255, 0, 0)
    set_brightness(5)
    fade_brightness(5, 100, 300000)             -- 5% → 100% (5 min)
    fade(255, 0, 0, 0, 255, 0, 300000)          -- Red → Green (5 min)

elseif current_time_in_minutes == dusk_transition then
    -- Dusk: brightness fade only
    print("Dusk transition: Green 100% → Green 25%")
    set_color(0, 255, 0)
    fade_brightness(100, 25, 300000)            -- 100% → 25% (5 min)

elseif current_time_in_minutes == evening_transition then
    -- Evening: fade brightness first, then color
    print("Evening transition: Green → Red")
    set_color(0, 255, 0)
    set_brightness(25)
    fade_brightness(25, 5, 300000)              -- 25% → 5% (5 min)
    fade(0, 255, 0, 255, 0, 0, 300000)          -- Green → Red (5 min)

else
    -- Idle: set correct state if launched at another time
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

