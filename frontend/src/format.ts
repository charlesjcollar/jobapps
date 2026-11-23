const dateFmt = new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", year: "numeric" });
const dateTimeFmt = new Intl.DateTimeFormat(undefined, {
  month: "short",
  day: "numeric",
  year: "numeric",
  hour: "numeric",
  minute: "2-digit",
});

/** Formats a YYYY-MM-DD calendar date without shifting it into another time zone. */
export function formatDate(ymd: string): string {
  const [y, m, d] = ymd.split("-").map(Number);
  return dateFmt.format(new Date(y, m - 1, d));
}

/** Formats the local calendar date of an RFC 3339 timestamp. */
export function formatTimestampDate(iso: string): string {
  return dateFmt.format(new Date(iso));
}

/** Formats an RFC 3339 timestamp in local time. */
export function formatDateTime(iso: string): string {
  return dateTimeFmt.format(new Date(iso));
}
