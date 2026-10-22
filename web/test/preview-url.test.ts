import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { PREVIEW_DATA_URL, dataFolderUrl, previewState, serviceRunning, toPreviewUrl } from "../settings/preview-url.ts";

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
  const state = JSON.stringify({
    view: {
      background: { entry_url: `${DATA}extensions/builtin.aerials/index.js`, options: { files: [`${DATA}media/aerials/a.mov`] } },
      components: [{ entry_url: `${DATA}extensions/builtin.clock/index.js`, options: {} }],
    },
  });
  const rewritten = JSON.parse(previewState(state, DATA));

  it("loads extension entries from the preview scheme", () => {
    assert.equal(rewritten.view.background.entry_url, `${PREVIEW_DATA_URL}extensions/builtin.aerials/index.js`);
    assert.equal(rewritten.view.components[0].entry_url, `${PREVIEW_DATA_URL}extensions/builtin.clock/index.js`);
  });

  it("leaves option values as the extension expects them", () => {
    // The aerials background keeps only file:// URLs; the frame maps media src instead.
    assert.deepEqual(rewritten.view.background.options.files, [`${DATA}media/aerials/a.mov`]);
  });
});

describe("serviceRunning", () => {
  const cases: [string, string | null, boolean][] = [
    ["a state from the daemon", '{"service_ok":true}', true],
    ["the idle state without the daemon", '{"service_ok":false}', false],
    ["no state", null, false],
    ["text that is not JSON", "{", false],
  ];
  for (const [name, state, want] of cases) {
    it(name, () => assert.equal(serviceRunning(state), want));
  }
});
