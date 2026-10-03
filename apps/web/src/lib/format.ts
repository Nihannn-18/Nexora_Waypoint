import { OPERATING_TIMEZONE, type IsoDate } from '@waypoint/shared-types';

/**
 * Display formatting. Every date and time is rendered in the business timezone
 * (Asia/Colombo), never the browser's, and "now" always comes from the API
 * clock — the seeded demo day is in the past.
 *
 * Style guide: dates read "Sat 26 Sep", times are 24-hour HH:MM, weights in kg
 * and volumes in m³.
 */

const dayParts = new Intl.DateTimeFormat('en-GB', {
  timeZone: OPERATING_TIMEZONE,
  weekday: 'short',
  day: 'numeric',
  month: 'numeric',
});

// Three-letter months: some ICU builds print "Sept", the style guide says "Sep".
const MONTHS = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];

const dayFormat = {
  format(instant: Date): string {
    const parts = dayParts.formatToParts(instant);
    const get = (type: string) =>
      parts.find((p) => p.type === type)?.value ?? '';
    return `${get('weekday')} ${Number(get('day'))} ${MONTHS[Number(get('month')) - 1] ?? ''}`;
  },
};

const timeFormat = new Intl.DateTimeFormat('en-GB', {
  timeZone: OPERATING_TIMEZONE,
  hour: '2-digit',
  minute: '2-digit',
  hourCycle: 'h23',
});

const isoDateFormat = new Intl.DateTimeFormat('en-CA', {
  timeZone: OPERATING_TIMEZONE,
  year: 'numeric',
  month: '2-digit',
  day: '2-digit',
});

/** The business date of an instant, as `YYYY-MM-DD`. */
export function businessDate(instant: Date): IsoDate {
  return isoDateFormat.format(instant);
}

/** Calendar arithmetic on an ISO date, independent of any timezone. */
export function addDays(date: IsoDate, days: number): IsoDate {
  const [y, m, d] = date.split('-').map(Number);
  const shifted = new Date(Date.UTC(y ?? 1970, (m ?? 1) - 1, (d ?? 1) + days));
  return shifted.toISOString().slice(0, 10);
}

/** "Sat 26 Sep" for an ISO date. */
export function formatDay(date: IsoDate): string {
  // Noon UTC keeps the calendar day stable in every timezone.
  return dayFormat.format(new Date(`${date}T12:00:00Z`)).replace(',', '');
}

/** "Sat 26 Sep" for an instant, in the business timezone. */
export function formatInstantDay(instant: string | Date): string {
  return dayFormat.format(new Date(instant)).replace(',', '');
}

/** "07:42" for an instant, in the business timezone. */
export function formatTime(instant: string | Date): string {
  return timeFormat.format(new Date(instant));
}

/** "Sat 26 Sep · 07:42". */
export function formatDateTime(instant: string | Date): string {
  return `${formatInstantDay(instant)} · ${formatTime(instant)}`;
}

const kgFormat = new Intl.NumberFormat('en-GB', { maximumFractionDigits: 1 });
const m3Format = new Intl.NumberFormat('en-GB', {
  minimumFractionDigits: 1,
  maximumFractionDigits: 2,
});
const litreFormat = new Intl.NumberFormat('en-GB', {
  maximumFractionDigits: 1,
});

export const formatKg = (kg: number): string => `${kgFormat.format(kg)} kg`;
export const formatM3 = (m3: number): string => `${m3Format.format(m3)} m³`;
export const formatLitres = (l: number): string => `${litreFormat.format(l)} L`;
export const formatKm = (km: number): string => `${kgFormat.format(km)} km`;
export const formatMinutes = (min: number): string => `${Math.round(min)} min`;

/** Whole-number percentage of a limit, for meters. */
export function percentOf(value: number, limit: number): number {
  if (limit <= 0) return 0;
  return Math.round((value / limit) * 100);
}

/** "REAR_DOCK" → "Rear dock". */
export function humanize(code: string): string {
  const words = code.toLowerCase().split('_');
  const first = words[0] ?? '';
  return [first.charAt(0).toUpperCase() + first.slice(1), ...words.slice(1)]
    .join(' ')
    .trim();
}

/** "1 order" / "3 orders". */
export function plural(n: number, singular: string, pluralForm?: string) {
  return `${n} ${n === 1 ? singular : (pluralForm ?? `${singular}s`)}`;
}

/** "hh:mm:ss" for a non-negative duration. */
export function formatCountdown(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  return [h, m, s].map((n) => String(n).padStart(2, '0')).join(':');
}
