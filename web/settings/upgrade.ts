/** One runnable file an upgrade changes (Go's install.FileChange). */
export interface FileChange {
  path: string;
  kind: "changed" | "added" | "removed";
  diff?: string;
  truncated?: boolean;
  binary?: boolean;
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

/** Go caps each diff at this many lines (install.MaxDiffLines). */
export const MAX_DIFF_LINES = 200;
const SHORT_HASH = 12;

export interface UpgradeFile {
  /** "Changed: bin/convert", with "(binary)" for a file shown by its hash. */
  heading: string;
  /** The unified diff of a text file, or null. */
  diff: string | null;
  /** Size and hash lines of a binary file, and the note of a cut diff. */
  notes: string[];
}

export interface UpgradeDialog {
  title: string;
  intro: string;
  files: UpgradeFile[];
  confirmLabel: string;
}

const KIND_NAMES = { changed: "Changed", added: "Added", removed: "Removed" };

const short = (hash: string | undefined): string => (hash ?? "").slice(0, SHORT_HASH);

/** The dialog for an upgrade that changes an outside agent's runnable files. */
export function upgradeDialog(review: InstallReview): UpgradeDialog {
  const name = review.manifest.name || review.manifest.id;
  const from = review.from_version || "an unknown version";
  const to = review.manifest.version || "an unknown version";
  return {
    title: `Upgrade ${name}?`,
    intro: `${review.manifest.id} goes from ${from} to ${to}. These files run on your Mac or are copied into the agent's folder, and they change. The installed version stays until you press Install.`,
    files: review.changes.map((change) => {
      const notes: string[] = [];
      if (change.binary) {
        if (change.kind !== "added") notes.push(`Old: ${change.old_size} bytes, sha256 ${short(change.old_sha256)}`);
        if (change.kind !== "removed") notes.push(`New: ${change.new_size} bytes, sha256 ${short(change.new_sha256)}`);
      }
      if (change.truncated) notes.push(`Diff truncated after ${MAX_DIFF_LINES} lines.`);
      return {
        heading: `${KIND_NAMES[change.kind]}: ${change.path}${change.binary ? " (binary)" : ""}`,
        diff: change.binary || !change.diff ? null : change.diff,
        notes,
      };
    }),
    confirmLabel: "Install",
  };
}
