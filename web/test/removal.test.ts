import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { removalStatus } from "../settings/removal.ts";

describe("removalStatus", () => {
  it("names one extension removed alone", () => {
    assert.equal(removalStatus("Rain", { removed: ["acme.rain"], kept: null, theme_reset: false }), "Removed Rain");
  });

  it("counts the parts a theme brought", () => {
    const removal = { removed: ["stavgayer.kida", "stavgayer.kida-layout", "stavgayer.kida-ground", "stavgayer.kida-board"], kept: null, theme_reset: false };
    assert.equal(removalStatus("Kid A", removal), "Removed Kid A and 3 parts it brought");
    assert.equal(removalStatus("Dusk", { removed: ["acme.dusk", "acme.dusk-layout"], kept: [], theme_reset: false }), "Removed Dusk and 1 part it brought");
  });

  it("says when the default theme took over", () => {
    assert.equal(removalStatus("Dusk", { removed: ["acme.dusk"], kept: null, theme_reset: true }), "Removed Dusk. The Default theme is in use now");
  });

  it("copes with nothing removed", () => {
    assert.equal(removalStatus("Rain", { removed: null, kept: null, theme_reset: false }), "Removed Rain");
  });
});
