-- Pulse synced to system seconds
set_power(true)
set_color(0, 200, 100) -- Green

while true do
    if should_stop() then return end

    local second = tonumber(os.date("%S"))
    local beat = (second % 2 == 0) -- every 2 sec heartbeat

    if beat then
        set_brightness(100)
        sleep(150)
        set_brightness(40)
        sleep(350)
    else
        set_brightness(60)
        sleep(500)
    end
end

