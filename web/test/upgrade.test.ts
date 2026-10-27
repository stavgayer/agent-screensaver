import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { upgradeDialog, type FileChange, type InstallReview } from "../settings/upgrade.ts";

function review(changes: FileChange[]): InstallReview {
  return { manifest: { id: "acme.agent", name: "Acme", version: "1.1.0" }, upgrade: true, from_version: "1.0.0", changes, hash: "h" };
}

describe("upgradeDialog", () => {
  it("shows a changed text file with its diff", () => {
    const diff = "--- a/bin/convert\n+++ b/bin/convert\n@@ -1 +1 @@\n-cat\n+curl\n";
    const dialog = upgradeDialog(review([{ path: "bin/convert", kind: "changed", diff, old_size: 4, new_size: 5 }]));
    assert.equal(dialog.title, "Upgrade Acme?");
    assert.match(dialog.intro, /acme\.agent goes from 1\.0\.0 to 1\.1\.0/);
    assert.deepEqual(dialog.files, [{ heading: "Changed: bin/convert", diff, notes: [] }]);
    assert.equal(dialog.confirmLabel, "Install");
  });

  it("shows a binary file by its sizes and short hashes", () => {
    const dialog = upgradeDialog(review([{
      path: "bin/tool", kind: "changed", binary: true, old_size: 8, new_size: 10,
      old_sha256: "a".repeat(64), new_sha256: "b".repeat(64),
    }]));
    assert.deepEqual(dialog.files[0], {
      heading: "Changed: bin/tool (binary)",
      diff: null,
      notes: [`Old: 8 bytes, sha256 ${"a".repeat(12)}`, `New: 10 bytes, sha256 ${"b".repeat(12)}`],
    });
  });

  it("names added and removed files and a cut diff", () => {
    const dialog = upgradeDialog(review([
      { path: "plugin.js", kind: "added", diff: "+x\n", truncated: true, old_size: 0, new_size: 2 },
      { path: "bin/old", kind: "removed", binary: true, old_size: 3, new_size: 0, old_sha256: "c".repeat(64) },
    ]));
    assert.equal(dialog.files[0]!.heading, "Added: plugin.js");
    assert.deepEqual(dialog.files[0]!.notes, ["Diff truncated after 200 lines."]);
    assert.deepEqual(dialog.files[1]!.notes, [`Old: 3 bytes, sha256 ${"c".repeat(12)}`]);
  });
});
