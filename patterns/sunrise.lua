-- sunrise.lua
local function clamp(v, lo, hi) return math.max(lo, math.min(hi, v)) end
local function to_min(t) if not t then return 0 end; return (tonumber(t.hour) or 0) * 60 + (tonumber(t.min) or 0) end
local function now()
  local h = tonumber(os.date("%H")) or 0
  local m = tonumber(os.date("%M")) or 0
  return h * 60 + m, h, m
end
local function in_range(cur, a, b)
  if a == b then return false end
  if a < b then return cur >= a and cur < b end
  return cur >= a or cur < b
end
local function safe_set_color(color)
  if not color then return end
  set_color(tonumber(color[1]) or 0, tonumber(color[2]) or 0, tonumber(color[3]) or 0)
end
local function safe_set_brightness(br)
  if not br then return end
  br = math.floor(clamp(tonumber(br) or 0, 0, 100))
  set_brightness(br)
  return br
end
local function apply_state(color, brightness, label, h, m)
  safe_set_color(color)
  local br = safe_set_brightness(brightness) or 0
  local cname = "Custom"
  if color and color[1]==0 and color[2]==255 then cname = "Green"
  elseif color and color[1]==255 and color[2]==0 then cname = "Red" end
  print(string.format("%s | %02d:%02d | color=%s | br=%d%%", label, h, m, cname, br))
end

-- CONFIG
local BR_MIN = 100
local BR_DUSK = 100
local BR_MAX = 100
local DEFAULT_TRANS_MS = 60 * 1000

local transitions = {
  { name="morning",
    at = { hour = 9 },
    from = { color={255,0,0}, brightness = BR_MIN },
    to   = { color={0,255,0}, brightness = BR_MAX },
    duration_ms = DEFAULT_TRANS_MS },
  { name="dusk",
    at = { hour = 21, min = 0 },
    from = { color={0,255,0}, brightness = BR_MAX },
    to   = { color={255, 17, 0}, brightness = BR_DUSK },
    duration_ms = DEFAULT_TRANS_MS },
  { name="evening",
    at = { hour = 0, min = 0 },
    from = { color={255, 17, 0}, brightness = BR_DUSK },
    to   = { color={255,0,0}, brightness = BR_MIN },
    duration_ms = DEFAULT_TRANS_MS },

  { name="day_period",
    range = { start = { hour = 9 }, finish = { hour = 21 } },
    target = { color = {0,255,0}, brightness = BR_MAX } },
  { name="dusk_period",
    range = { start = { hour = 21 }, finish = { hour = 0 } },
    target = { color = {255, 17, 0}, brightness = BR_DUSK } },
  { name="night_period",
    range = { start = { hour = 0 }, finish = { hour = 9 } },
    target = { color = {255,0,0}, brightness = BR_MIN } }
}

print("Starting sunrise pattern...")
set_power(true)
local cur_min, hh, mm = now()

-- exact triggers (cron runs at minute 0; matches when t.at.min is nil or 0)
for _, t in ipairs(transitions) do
  if t.at and cur_min == to_min(t.at) then
    if t.from and t.from.color then safe_set_color(t.from.color) end
    if t.from and t.from.brightness then safe_set_brightness(t.from.brightness) end

    local dur = tonumber(t.duration_ms) or DEFAULT_TRANS_MS

    if t.from and t.from.brightness and t.to and t.to.brightness and type(fade_brightness) == "function" then
      fade_brightness(t.from.brightness, t.to.brightness, dur)
    elseif t.to and t.to.brightness then
      safe_set_brightness(t.to.brightness)
    end

    if t.from and t.from.color and t.to and t.to.color and type(fade) == "function" then
      fade(
        tonumber(t.from.color[1]) or 0, tonumber(t.from.color[2]) or 0, tonumber(t.from.color[3]) or 0,
        tonumber(t.to.color[1])   or 0, tonumber(t.to.color[2])   or 0, tonumber(t.to.color[3])   or 0,
        dur
      )
    elseif t.to and t.to.color then
      safe_set_color(t.to.color)
    end

    print("Triggered exact transition: "..t.name)
    return
  end
end

-- apply steady state for current range
for _, t in ipairs(transitions) do
  if t.range and t.target then
    local a = to_min(t.range.start)
    local b = to_min(t.range.finish)
    if in_range(cur_min, a, b) then
      apply_state(t.target.color, t.target.brightness, t.name, hh, mm)
      return
    end
  end
end

-- fallback
apply_state({255,0,0}, BR_MIN, "fallback_night", hh, mm)
