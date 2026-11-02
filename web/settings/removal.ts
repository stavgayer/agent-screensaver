/** The `removeExtension` answer (Go's settings.Removal). */
export interface Removal {
  removed: string[] | null;
  kept: { id: string; used_by: string }[] | null;
  theme_reset: boolean;
}

/** Says what a remove did: a theme goes with the parts it brought, and a theme in use falls back to the default. */
export function removalStatus(name: string, removal: Removal): string {
  const parts = (removal.removed ?? []).length - 1;
  let text = parts > 0 ? `Removed ${name} and ${parts} ${parts === 1 ? "part" : "parts"} it brought` : `Removed ${name}`;
  if (removal.theme_reset) text += ". The Default theme is in use now";
  return text;
}
