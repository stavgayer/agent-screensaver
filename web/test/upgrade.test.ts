import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { diffLines, escapeControls, upgradeDialog, type FileChange, type InstallReview } from "../settings/upgrade.ts";

function review(changes: FileChange[]): InstallReview {
  return { manifest: { id: "acme.agent", name: "Acme", version: "1.1.0" }, upgrade: true, from_version: "1.0.0", changes, hash: "h" };
}

const plain = (text: string) => ({ text, rest: null, cutNote: null });

describe("upgradeDialog", () => {
  it("shows a changed text file with every line of its diff, under its group", () => {
    const diff = "--- a/bin/convert\n+++ b/bin/convert\n@@ -1 +1 @@\n-cat\n+curl\n";
    const dialog = upgradeDialog(review([{ path: "bin/convert", kind: "changed", group: "runnable", diff, old_size: 4, new_size: 5 }]));
    assert.equal(dialog.title, "Upgrade Acme?");
    assert.match(dialog.intro, /acme\.agent goes from 1\.0\.0 to 1\.1\.0/);
    assert.equal(dialog.groups.length, 1);
    assert.match(dialog.groups[0]!.heading, /^Programs and copied files/);
    assert.deepEqual(dialog.groups[0]!.files, [{
      heading: "Changed: bin/convert",
      lines: ["--- a/bin/convert", "+++ b/bin/convert", "@@ -1 +1 @@", "-cat", "+curl"].map(plain),
      notes: [],
    }]);
    assert.equal(dialog.confirmLabel, "Install");
  });

  it("keeps a long diff whole", () => {
    const diff = Array.from({ length: 900 }, (_, index) => `+line ${index}`).join("\n") + "\n";
    const dialog = upgradeDialog(review([{ path: "a.sh", kind: "added", group: "other", diff, old_size: 0, new_size: 1 }]));
    assert.equal(dialog.groups[0]!.files[0]!.lines!.length, 900);
  });

  it("groups the manifest, programs and other files in order", () => {
    const dialog = upgradeDialog(review([
      { path: "extension.json", kind: "changed", group: "manifest", diff: "-a\n+b\n", old_size: 1, new_size: 1 },
      { path: "bin/convert", kind: "changed", group: "runnable", diff: "-a\n+b\n", old_size: 1, new_size: 1 },
      { path: "lib/helper.sh", kind: "changed", group: "other", diff: "-a\n+b\n", old_size: 1, new_size: 1 },
    ]));
    assert.deepEqual(dialog.groups.map((group) => group.heading.split(":")[0]), ["Manifest", "Programs and copied files", "Other files"]);
  });

  it("shows a binary file by its sizes and short hashes", () => {
    const dialog = upgradeDialog(review([{
      path: "bin/tool", kind: "changed", group: "runnable", binary: true, old_size: 8, new_size: 10,
      old_sha256: "a".repeat(64), new_sha256: "b".repeat(64),
    }]));
    assert.deepEqual(dialog.groups[0]!.files[0], {
      heading: "Changed: bin/tool (binary)",
      lines: null,
      notes: [`Old: 8 bytes, sha256 ${"a".repeat(12)}`, `New: 10 bytes, sha256 ${"b".repeat(12)}`],
    });
  });

  it("names the paths of a file too large to show, and the button asks for a review by hand", () => {
    const dialog = upgradeDialog(review([{
      path: "bin/convert", kind: "changed", group: "runnable", too_large: true, old_size: 1, new_size: 3_000_000,
      old_path: "/data/extensions/acme.agent/bin/convert", new_path: "/tmp/acme/bin/convert",
    }]));
    assert.deepEqual(dialog.groups[0]!.files[0]!.notes, [
      "Too large to show: review the files by hand.",
      "Old: /data/extensions/acme.agent/bin/convert",
      "New: /tmp/acme/bin/convert",
    ]);
    assert.equal(dialog.confirmLabel, "I reviewed the files by hand, install");
  });

  it("escapes control characters in names and paths", () => {
    const dialog = upgradeDialog(review([{ path: "bin/‮convert", kind: "added", group: "runnable", diff: "+x\n", old_size: 0, new_size: 2 }]));
    assert.equal(dialog.groups[0]!.files[0]!.heading, "Added: bin/<U+202E>convert");
  });
});

describe("escapeControls", () => {
  it("shows C0, DEL, C1 and bidi characters as escapes and keeps tabs", () => {
    assert.equal(escapeControls("a\x1b[2Kb\r\x07\x7f\tc"), "a\\x1b[2Kb\\x0d\\x07\\x7f\tc");
    assert.equal(escapeControls("‮evil⁦‏\u0085﻿"), "<U+202E>evil<U+2066><U+200F><U+0085><U+FEFF>");
    assert.equal(escapeControls("plain é 日本"), "plain é 日本");
  });

  it("shows every format character, variation selector and tag character as an escape", () => {
    assert.equal(escapeControls("soft\u00adhyphen"), "soft<U+00AD>hyphen");
    assert.equal(escapeControls("a\u180eb"), "a<U+180E>b");
    assert.equal(escapeControls("heart\ufe0f"), "heart<U+FE0F>");
    assert.equal(escapeControls("tag\u{E0041}"), "tag<U+E0041>");
    assert.equal(escapeControls("sel\u{E0100}"), "sel<U+E0100>");
  });
});

describe("diffLines", () => {
  it("cuts a line over 500 characters and keeps the rest for a click", () => {
    const [line] = diffLines(`+${"x".repeat(700)}\n`);
    assert.equal(line!.text, `+${"x".repeat(499)}`);
    assert.equal(line!.rest, "x".repeat(201));
    assert.equal(line!.cutNote, "(line cut, 201 more characters)");
  });
});
