/**
 * The ignored-projects list editor. The service stores folder names and has
 * the final word (Go's config.ProjectName); this mirrors its rule so the page
 * can say "already in the list" before it calls Go.
 */

/** A folder name or a path, as the folder name events carry. "" when nothing usable is left. */
export function projectName(entry: string): string {
  const trimmed = entry.trim().replace(/\/+$/, "");
  const name = trimmed.split("/").pop() ?? "";
  return name === "." ? "" : name;
}

/** The list with `entry` added at the end, or why it cannot be added. */
export function withProject(projects: readonly string[], entry: string): { projects: string[] } | { error: string } {
  const name = projectName(entry);
  if (name === "") return { error: "Type a folder name, for example observer-sessions." };
  if (projects.some((project) => sameProject(project, name))) return { error: `${name} is already in the list.` };
  return { projects: [...projects, name] };
}

/**
 * Whether two folder names match. Upper and lower case do not count, like
 * Go's strings.EqualFold in config.IgnoresProject.
 */
export function sameProject(a: string, b: string): boolean {
  return a.toLowerCase() === b.toLowerCase() || a.toUpperCase() === b.toUpperCase();
}

/** The list without `name`. */
export function withoutProject(projects: readonly string[], name: string): string[] {
  return projects.filter((project) => project !== name);
}
