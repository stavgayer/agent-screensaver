import type { BackgroundExt, Ctx } from "../../api.ts";
import { numberOption } from "../shared/options.ts";
import { ROWS_PER_SECOND, advanceDrop, charsetFor, fadeAlpha } from "./logic.ts";

const BASE_CELL_PX = 16;
const MAX_FRAME_MS = 250; // a long pause (sleep, hidden tab) must not make the rain jump
const GLYPH_OPACITY = 0.55;
const START_SPREAD_ROWS = 50;

/** Falling glyphs on a canvas that fills its root. */
function createMatrix(): BackgroundExt {
  let canvas: HTMLCanvasElement | undefined;
  let cancelFrame: (() => void) | undefined;
  let onResize: (() => void) | undefined;

  function start(root: HTMLElement, ctx: Ctx): void {
    const { options } = ctx;
    const color = typeof options.color === "string" ? options.color : "#00ff66";
    const speed = numberOption(options.speed, 1, 0.25, 4);
    const fps = numberOption(options.fps, 24, 1, 240);
    const density = numberOption(options.density, 1, 0.25, 2);
    const glyphs = charsetFor(options.charset);
    const cell = BASE_CELL_PX / density;

    const element = document.createElement("canvas");
    element.style.cssText = "display:block;width:100%;height:100%";
    root.appendChild(element);
    canvas = element;
    const context = element.getContext("2d");
    if (context === null) return;

    let width = 0;
    let height = 0;
    let drops: number[] = [];

    const fit = (): void => {
      const ratio = window.devicePixelRatio || 1;
      width = root.clientWidth;
      height = root.clientHeight;
      element.width = Math.max(1, Math.round(width * ratio));
      element.height = Math.max(1, Math.round(height * ratio));
      context.setTransform(ratio, 0, 0, ratio, 0, 0);
      context.fillStyle = "#000";
      context.fillRect(0, 0, width, height);
      context.font = `${cell}px monospace`;
      const columns = Math.ceil(width / cell);
      // Keep the columns that already fall; give new ones a random head start above the screen.
      drops = Array.from({ length: columns }, (_, index) => drops[index] ?? Math.random() * -START_SPREAD_ROWS);
    };
    fit();
    onResize = fit;
    window.addEventListener("resize", onResize);

    let lastMs: number | undefined;
    const draw = (nowMs: number): void => {
      const frameMs = Math.min(MAX_FRAME_MS, nowMs - (lastMs ?? nowMs - 1000 / fps));
      lastMs = nowMs;
      const rows = (ROWS_PER_SECOND * speed * frameMs) / 1000;
      const screenRows = height / cell;
      context.globalAlpha = 1;
      context.fillStyle = `rgba(0,0,0,${fadeAlpha(frameMs)})`;
      context.fillRect(0, 0, width, height);
      context.fillStyle = color;
      context.globalAlpha = GLYPH_OPACITY;
      drops.forEach((row, column) => {
        const next = advanceDrop(row, screenRows, rows, Math.random);
        // Draw a glyph only when the head enters a new row, so slow drops do not smear one cell.
        if (Math.floor(next) !== Math.floor(row) && next >= 0) {
          context.fillText(glyphs[Math.floor(Math.random() * glyphs.length)], column * cell, Math.floor(next) * cell);
        }
        drops[column] = next;
      });
      context.globalAlpha = 1;
    };
    cancelFrame = ctx.requestFrame(draw, fps);
  }

  function stop(): void {
    cancelFrame?.();
    cancelFrame = undefined;
    if (onResize !== undefined) window.removeEventListener("resize", onResize);
    onResize = undefined;
    canvas?.remove();
    canvas = undefined;
  }

  return { start, stop };
}

window.agentScreensaver.register(createMatrix);
