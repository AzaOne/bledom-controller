-- police.lua: Rapidly strobes between red and blue.

print("Starting police pattern...")

set_power(true)
set_brightness(100)

-- Loop forever until the user stops the pattern
while true do
  
  -- Flash RED
  set_color(255, 0, 0)
  sleep(100) -- Wait 100 milliseconds
  
  set_color(0, 0, 0) -- Turn off briefly for a better strobe effect
  sleep(50)

  -- Check if we should stop between flashes 
  if should_stop() then return end

  -- Flash BLUE
  set_color(0, 0, 255)
  sleep(100)

  set_color(0, 0, 0)
  sleep(50)

  -- Another check to make the pattern very responsive
  if should_stop() then return end
end
