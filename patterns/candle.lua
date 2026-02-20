-- candle.lua: Simulates a warm, flickering candle flame.

print("Starting candle flicker pattern...")
set_power(true)

-- Base candle color (a warm orange-yellow)
local base_r, base_g, base_b = 255, 140, 20
local base_brightness = 80

-- Loop until cancelled
while true do
  if should_stop() then return end
  -- Every loop, calculate a slightly random brightness and color shift
  -- Fluctuate brightness between 50% and 100% of the base
  local brightness_flicker = math.random(50, 100)
  local current_brightness = base_brightness * (brightness_flicker / 100)
  set_brightness(current_brightness)

  -- Slightly shift the color to be more red or more yellow
  local r_flicker = math.random(-30, 10) -- Allow it to become slightly more red
  local g_flicker = math.random(-10, 20) -- or slightly more yellow/white
  
  -- Clamp values to be within the 0-255 range
  local r = math.max(0, math.min(255, base_r + r_flicker))
  local g = math.max(0, math.min(128, base_g + g_flicker))
  local b = base_b

  set_color(r, g, b)

  -- The delay itself is also random, which is key to a realistic effect
  local delay = math.random(10, 100) -- Sleep for 50-200ms
  sleep(delay)

  if should_stop() then return end
end
