import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { PREVIEW_DATA_URL, dataFolderUrl, previewState, toPreviewUrl } from "../settings/preview-url.ts";

const DATA = "file:///Users/me/Library/Application%20Support/agent-screensaver/";

describe("dataFolderUrl", () => {
  it("is two folders up from the settings page", () => {
    assert.equal(dataFolderUrl(`${DATA}runtime/settings/index.html`), DATA);
  });
});

describe("toPreviewUrl", () => {
  const cases: [string, string, string][] = [
    ["a file in the data folder", `${DATA}media/aerials/a.mov`, `${PREVIEW_DATA_URL}media/aerials/a.mov`],
    ["a file outside stays as it is", "file:///Users/me/Movies/a.mov", "file:///Users/me/Movies/a.mov"],
    ["a sibling folder with the same prefix stays", "file:///Users/me/Library/Application%20Support/agent-screensaver-old/a.mov", "file:///Users/me/Library/Application%20Support/agent-screensaver-old/a.mov"],
    ["another scheme stays", "data:image/png;base64,AA", "data:image/png;base64,AA"],
  ];
  for (const [name, url, want] of cases) {
    it(name, () => assert.equal(toPreviewUrl(url, DATA), want));
  }
});

describe("previewState", () => {
  it("rewrites every data folder URL in the state text", () => {
    const state = JSON.stringify({
      view: { background: { entry_url: `${DATA}extensions/builtin.matrix/index.js`, options: { files: [`${DATA}media/a.mov`] } } },
    });
    const rewritten = JSON.parse(previewState(state, DATA));
    assert.equal(rewritten.view.background.entry_url, `${PREVIEW_DATA_URL}extensions/builtin.matrix/index.js`);
    assert.deepEqual(rewritten.view.background.options.files, [`${PREVIEW_DATA_URL}media/a.mov`]);
  });
});
