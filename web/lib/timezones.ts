/** IANA timezones for department settings. "UTC" is not always included by Intl, so add it. */
export function timezoneOptions(): { value: string; label: string }[] {
  const zones = new Set<string>(["UTC", ...Intl.supportedValuesOf("timeZone")]);
  return [...zones].map((z) => ({ value: z, label: z.replaceAll("_", " ") }));
}
