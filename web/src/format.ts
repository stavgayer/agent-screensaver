import type { AgentInfo } from "./api.ts";

const MS_PER_MINUTE = 60_000;
const MINUTES_PER_HOUR = 60;
const MS_PER_DAY = 24 * MINUTES_PER_HOUR * MS_PER_MINUTE;

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

/** Minutes east of UTC at one instant in the local time zone, so a summer or winter date uses its own offset. */
export function localUtcOffsetMinutes(atMs: number): number {
  return -new Date(atMs).getTimezoneOffset();
}

/**
 * Time left to a reset, to the minute and rounded down: "4h 12m", "47m", "under 1m".
 * At or past the reset it is "Reset".
 */
export function formatCountdown(remainingMs: number): string {
  if (remainingMs <= 0) return "Reset";
  const minutes = Math.floor(remainingMs / MS_PER_MINUTE);
  if (minutes < 1) return "under 1m";
  if (minutes < MINUTES_PER_HOUR) return `${minutes}m`;
  return `${Math.floor(minutes / MINUTES_PER_HOUR)}h ${minutes % MINUTES_PER_HOUR}m`;
}

/**
 * English short weekday and 24-hour time, "Mon 09:00", at a given UTC offset.
 * Built by hand so it never depends on locale data; the offset is passed in so the
 * result does not depend on the machine's time zone.
 */
export function formatDayTime(atMs: number, utcOffsetMinutes: number): string {
  const shifted = new Date(atMs + utcOffsetMinutes * MS_PER_MINUTE);
  return `${WEEKDAYS[shifted.getUTCDay()].slice(0, 3)} ${pad2(shifted.getUTCHours())}:${pad2(shifted.getUTCMinutes())}`;
}

/**
 * Reset label for a ring. Within 24 hours, or at 24 hours exactly, it is the countdown.
 * Further away it is the day and time in local time. `resetsAtS` is in seconds;
 * `offsetAt` gives the local UTC offset at the reset instant.
 */
export function formatResetLabel(
  resetsAtS: number | undefined,
  nowMs: number,
  offsetAt: (atMs: number) => number = localUtcOffsetMinutes,
): string {
  if (resetsAtS === undefined) return "";
  const resetsAtMs = resetsAtS * 1000;
  const remainingMs = resetsAtMs - nowMs;
  return remainingMs > MS_PER_DAY ? formatDayTime(resetsAtMs, offsetAt(resetsAtMs)) : formatCountdown(remainingMs);
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

/** The name of a card's agent from its extension, else the source with a capital letter. */
export function agentName(source: string, agents: Readonly<Record<string, AgentInfo>>): string {
  return (Object.hasOwn(agents, source) ? agents[source]?.name : undefined) || displayName(source);
}
