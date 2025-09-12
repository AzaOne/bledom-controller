-- Heartbeat Pulse
set_power(true)
set_color(255, 0, 50) -- Red-pink

while true do
    if should_stop() then return end

    set_brightness(100)
    sleep(120)
    set_brightness(30)
    sleep(120)

    set_brightness(80)
    sleep(100)
    set_brightness(20)
    sleep(500)

    sleep(800) -- pause between beats
end

