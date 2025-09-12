-- party-strobe.lua: Fast strobe through a set of vibrant colors.

print("Starting party strobe...")
set_power(true)
set_brightness(100)

-- Define a table of colors to cycle through
local colors = {
  {255, 0, 0},   -- Red
  {0, 255, 0},   -- Green
  {0, 0, 255},   -- Blue
  {255, 255, 0}, -- Yellow
  {0, 255, 255}, -- Cyan
  {255, 0, 255}, -- Magenta
  {255, 255, 255} -- White
}

while true do
  -- ipairs iterates through the table with an index and the value
  for i, color in ipairs(colors) do
    
    -- color[1] is Red, color[2] is Green, color[3] is Blue
    set_color(color[1], color[2], color[3])
    
    -- A very short sleep for a fast strobe effect
    sleep(120)

    if should_stop() then return end
  end
end
