const MS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;

const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const MONTHS = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];

const RING_WARN_FROM = 50;
const RING_HIGH_ABOVE = 80;

const pad2 = (value: number): string => String(value).padStart(2, "0");

/** Elapsed time, for example "3m" or "1h 12m". Under one minute is "<1m". */
export function formatDuration(ms: number): string {
  const minutes = Math.floor(ms / MS_PER_MINUTE);
  if (minutes < 1) return "<1m";
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  const rest = minutes % MINUTES_PER_HOUR;
  return rest === 0 ? `${hours}h` : `${hours}h ${rest}m`;
}

/**
 * Time until a limit resets, for example "1h40m". `resetsAtS` is in seconds.
 * Rounds up so a ring never shows "0m" before the reset.
 */
export function formatResetIn(resetsAtS: number | undefined, nowMs: number): string {
  if (resetsAtS === undefined) return "";
  const remainingMs = resetsAtS * 1000 - nowMs;
  if (remainingMs <= 0) return "reset, waiting for new data";
  const minutes = Math.ceil(remainingMs / MS_PER_MINUTE);
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  const hours = Math.floor(minutes / MINUTES_PER_HOUR);
  const rest = minutes % MINUTES_PER_HOUR;
  return rest === 0 ? `${hours}h` : `${hours}h${rest}m`;
}

export function ringLevel(pct: number): "ok" | "warn" | "high" {
  if (pct > RING_HIGH_ABOVE) return "high";
  if (pct >= RING_WARN_FROM) return "warn";
  return "ok";
}

/** 24-hour "HH:MM" in the local time zone. Built by hand so it never depends on locale data. */
export function formatClock(date: Date): string {
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
}

/** English "Tuesday 20 October" in the local time zone. Built by hand so it never depends on locale data. */
export function formatDate(date: Date): string {
  return `${WEEKDAYS[date.getDay()]} ${date.getDate()} ${MONTHS[date.getMonth()]}`;
}

/** "claude" is "Claude", "codex" is "Codex"; any other source gets a capital first letter. */
export function displayName(source: string): string {
  return source.charAt(0).toUpperCase() + source.slice(1);
}
