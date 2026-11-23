const dateFmt = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", year: "numeric" });
const monthDayFmt = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric" });
const dateTimeFmt = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

/** Parses YYYY-MM-DD as a local calendar date, or an RFC 3339 timestamp as-is. */
function parse(value: string): Date {
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    const [y, m, d] = value.split("-").map(Number);
    return new Date(y, m - 1, d);
  }
  return new Date(value);
}

/** Full date, e.g. "Nov 20, 2026". Accepts YYYY-MM-DD or a timestamp. */
export function formatDate(value: string): string {
  return dateFmt.format(parse(value));
}

/** "Nov 20" in the current year, "Nov 20, 2025" otherwise. */
export function shortDate(value: string): string {
  const d = parse(value);
  return d.getFullYear() === new Date().getFullYear() ? monthDayFmt.format(d) : dateFmt.format(d);
}

/** Date and time in local time. */
export function formatDateTime(iso: string): string {
  return dateTimeFmt.format(new Date(iso));
}
