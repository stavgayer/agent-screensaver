/** One file an upgrade changes (Go's install.FileChange). */
export interface FileChange {
  path: string;
  kind: "changed" | "added" | "removed";
  group: "manifest" | "runnable" | "other";
  /** The whole unified diff of a text file. */
  diff?: string;
  binary?: boolean;
  /** A diff too long to show: the two copies to read by hand. */
  too_large?: boolean;
  old_path?: string;
  new_path?: string;
  old_size: number;
  new_size: number;
  old_sha256?: string;
  new_sha256?: string;
}

/** An upgrade review, as Go's `installExtension` call answers it (settings.InstallReview). */
export interface InstallReview {
  manifest: { id: string; name?: string; version?: string };
  upgrade: boolean;
  from_version: string;
  changes: FileChange[];
  hash: string;
}

/** A diff line longer than this many characters is cut, with the rest one click away. */
export const MAX_LINE_CHARACTERS = 500;
const SHORT_HASH = 12;

/** One diff line, escaped. `rest` is the cut part, shown only on request. */
export interface DiffLine {
  text: string;
  rest: string | null;
  cutNote: string | null;
}

export interface UpgradeFile {
  /** "Changed: bin/convert", with "(binary)" for a file shown by its hash. */
  heading: string;
  /** Every line of the diff of a text file, or null. */
  lines: DiffLine[] | null;
  /** Size and hash lines of a binary file, or the paths of a file too large to show. */
  notes: string[];
}

export interface UpgradeGroup {
  heading: string;
  files: UpgradeFile[];
}

export interface UpgradeDialog {
  title: string;
  intro: string;
  groups: UpgradeGroup[];
  /** A file too large to show must be read by hand; the button says so. */
  confirmLabel: string;
}

const KIND_NAMES = { changed: "Changed", added: "Added", removed: "Removed" };
const GROUP_HEADINGS = {
  manifest: "Manifest",
  runnable: "Programs and copied files: these run on your Mac or go into the agent's folder",
  other: "Other files: a program of the agent can read or run these too",
};

const short = (hash: string | undefined): string => (hash ?? "").slice(0, SHORT_HASH);

/**
 * Shows every character that could hide or reorder text as a visible
 * escape, as the CLI does: C0 controls but tab and DEL as \xNN; C1
 * controls and the bidi and invisible format characters as <U+NNNN>.
 * Text nodes already keep markup out; this keeps the reading honest.
 */
export function escapeControls(text: string): string {
  let out = "";
  for (const character of text) {
    const code = character.codePointAt(0) as number;
    if (character === "\t") out += character;
    else if (code < 0x20 || code === 0x7f) out += `\\x${code.toString(16).padStart(2, "0")}`;
    else if (isHiddenFormat(code)) out += `<U+${code.toString(16).toUpperCase().padStart(4, "0")}>`;
    else out += character;
  }
  return out;
}

function isHiddenFormat(code: number): boolean {
  return (code >= 0x80 && code <= 0x9f) || code === 0x061c || (code >= 0x200b && code <= 0x200f) ||
    (code >= 0x2028 && code <= 0x202e) || (code >= 0x2060 && code <= 0x2069) || code === 0xfeff;
}

/** Every line of a diff, escaped, with lines over MAX_LINE_CHARACTERS cut. */
export function diffLines(diff: string): DiffLine[] {
  const lines = diff.endsWith("\n") ? diff.slice(0, -1).split("\n") : diff.split("\n");
  return lines.map((line) => {
    const characters = Array.from(escapeControls(line));
    if (characters.length <= MAX_LINE_CHARACTERS) return { text: characters.join(""), rest: null, cutNote: null };
    const more = characters.length - MAX_LINE_CHARACTERS;
    return {
      text: characters.slice(0, MAX_LINE_CHARACTERS).join(""),
      rest: characters.slice(MAX_LINE_CHARACTERS).join(""),
      cutNote: `(line cut, ${more} more characters)`,
    };
  });
}

function fileOf(change: FileChange): UpgradeFile {
  const notes: string[] = [];
  if (change.binary) {
    if (change.kind !== "added") notes.push(`Old: ${change.old_size} bytes, sha256 ${short(change.old_sha256)}`);
    if (change.kind !== "removed") notes.push(`New: ${change.new_size} bytes, sha256 ${short(change.new_sha256)}`);
  }
  if (change.too_large) {
    notes.push("Too large to show: review the files by hand.");
    if (change.old_path) notes.push(`Old: ${escapeControls(change.old_path)}`);
    if (change.new_path) notes.push(`New: ${escapeControls(change.new_path)}`);
  }
  return {
    heading: `${KIND_NAMES[change.kind]}: ${escapeControls(change.path)}${change.binary ? " (binary)" : ""}`,
    lines: change.binary || change.too_large || !change.diff ? null : diffLines(change.diff),
    notes,
  };
}

/** The dialog for an upgrade that changes any file of an outside agent. */
export function upgradeDialog(review: InstallReview): UpgradeDialog {
  const name = escapeControls(review.manifest.name || review.manifest.id);
  const from = escapeControls(review.from_version || "an unknown version");
  const to = escapeControls(review.manifest.version || "an unknown version");
  const groups: UpgradeGroup[] = [];
  for (const change of review.changes) {
    const heading = GROUP_HEADINGS[change.group];
    if (groups.at(-1)?.heading !== heading) groups.push({ heading, files: [] });
    (groups.at(-1) as UpgradeGroup).files.push(fileOf(change));
  }
  return {
    title: `Upgrade ${name}?`,
    intro: `${escapeControls(review.manifest.id)} goes from ${from} to ${to}. These files of the extension change. The installed version stays until you install.`,
    groups,
    confirmLabel: review.changes.some((change) => change.too_large) ? "I reviewed the files by hand, install" : "Install",
  };
}
