import type { BackgroundExt, Ctx, ViewState } from "../../api.ts";
import { loadFont } from "../shared/fonts.ts";
import { numberOption } from "../shared/options.ts";
import {
  MAX_SIGNS,
  approvalCount,
  easeToward,
  flashAmount,
  glitchAmount,
  layoutRow,
  liftShift,
  pinkTarget,
  renderSize,
  rowY,
  signsFrom,
  type Sign,
  type SignState,
} from "./logic.ts";
import {
  ATLAS_ROW_PX,
  ATLAS_WIDTH_PX,
  CAMERA_Z,
  FOCAL,
  FRAGMENT_SHADER,
  ROW_Z,
  SHAPE_LIFT_Z,
  SIGN_HALF_HEIGHT,
  VERTEX_SHADER,
} from "./shader.ts";

const FONT_FAMILY = "Chrome Scene Anton";
const WORD_PX = Math.round(ATLAS_ROW_PX * 0.9);
const WORD_FONT = `${WORD_PX}px "${FONT_FAMILY}", "Helvetica Neue", Impact, sans-serif`;
const FALLBACK_WORD_FONT = `bold ${WORD_PX}px "Helvetica Neue", Impact, sans-serif`;
/** Space on each side of a word, in atlas pixels. */
const WORD_PAD_PX = 26;
/** Guess for a word's width when no 2D context can measure it. */
const GUESS_PX_PER_LETTER = 52;
const SIGN_GAP = 0.18;
/** Share of the visible width the row may use. */
const ROW_WIDTH_SHARE = 0.86;
const PINK_TAU_MS = 500;
const MAX_FRAME_MS = 250; // a long pause must not make the scene jump
const WARP_START_S = 40;
const STATE_CODES: Record<SignState, number> = { idle: 0, running: 1, finished: 2, approval: 3 };
const UNIFORMS = [
  "uResolution", "uTime", "uWarpTime", "uPink", "uLift", "uLiftX", "uGlitch", "uGrain", "uCount", "uSigns", "uSignBox", "uRowY", "uRowHalf", "uAtlas",
] as const;
type UniformName = (typeof UNIFORMS)[number];

// A classic script knows its own URL only while it first runs.
const scriptSrc = (document.currentScript as HTMLScriptElement | null)?.src ?? "";

/** Grain and scan lines over black, for a Mac with no WebGL2 or a lost context. */
function fallbackLayer(): HTMLElement {
  const layer = document.createElement("div");
  const lines = "repeating-linear-gradient(to bottom, rgba(255,255,255,0.035) 0 1px, transparent 1px 3px)";
  const noise = noiseTile();
  layer.style.cssText = `display:block;width:100%;height:100%;background:${noise === "" ? "" : `url(${noise}), `}${lines}, #000`;
  return layer;
}

function noiseTile(): string {
  const tile = document.createElement("canvas");
  tile.width = 96;
  tile.height = 96;
  const context = tile.getContext("2d");
  if (context === null) return "";
  const image = context.createImageData(96, 96);
  for (let index = 0; index < image.data.length; index += 4) {
    image.data.fill(255, index, index + 3);
    image.data[index + 3] = Math.random() * 22;
  }
  context.putImageData(image, 0, 0);
  return tile.toDataURL();
}

function compile(gl: WebGL2RenderingContext, type: number, source: string): WebGLShader {
  const shader = gl.createShader(type);
  if (shader === null) throw new Error("no shader");
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) throw new Error(gl.getShaderInfoLog(shader) ?? "shader did not compile");
  return shader;
}

function link(gl: WebGL2RenderingContext): WebGLProgram {
  const program = gl.createProgram();
  if (program === null) throw new Error("no program");
  const vertex = compile(gl, gl.VERTEX_SHADER, VERTEX_SHADER);
  const fragment = compile(gl, gl.FRAGMENT_SHADER, FRAGMENT_SHADER);
  gl.attachShader(program, vertex);
  gl.attachShader(program, fragment);
  gl.linkProgram(program);
  gl.deleteShader(vertex);
  gl.deleteShader(fragment);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) throw new Error(gl.getProgramInfoLog(program) ?? "program did not link");
  return program;
}

/**
 * A liquid chrome shape in a bright studio, with a row of chrome signs under
 * it, one per agent card, each carrying the agent's name. Drawn by one
 * fragment shader at a reduced size that CSS scales up.
 */
function createChromeScene(): BackgroundExt {
  let root: HTMLElement | undefined;
  let canvas: HTMLCanvasElement | undefined;
  let fallback: HTMLElement | undefined;
  let gl: WebGL2RenderingContext | undefined;
  let program: WebGLProgram | undefined;
  let texture: WebGLTexture | undefined;
  let atlas: HTMLCanvasElement | undefined;
  let uniforms: Partial<Record<UniformName, WebGLUniformLocation | null>> = {};
  let cancelFrame: (() => void) | undefined;
  let onResize: (() => void) | undefined;

  let signs: Sign[] = [];
  let labels = "";
  let fontReady = false;
  // Set when the words or the font change; the next frame redraws the atlas and the row.
  let dirty = true;
  let pinkGoal = 0;
  let pink = 0;
  let glitchStartMs: number | undefined;
  /** Half widths of the words in the atlas, as a share of its width. */
  const wordHalf = new Float32Array(MAX_SIGNS);
  const signData = new Float32Array(MAX_SIGNS * 4);
  const signBox = new Float32Array(MAX_SIGNS * 4);
  let rowHalfWidth = 0;
  let rowHalfHeight = 0;

  function showFallback(): void {
    if (root === undefined || fallback !== undefined) return;
    fallback = fallbackLayer();
    root.appendChild(fallback);
  }

  function removeResize(): void {
    if (onResize !== undefined) window.removeEventListener("resize", onResize);
    onResize = undefined;
  }

  function releaseGl(contextAlive: boolean): void {
    cancelFrame?.();
    cancelFrame = undefined;
    if (gl !== undefined && contextAlive) {
      if (program !== undefined) gl.deleteProgram(program);
      if (texture !== undefined) gl.deleteTexture(texture);
      (gl.getExtension("WEBGL_lose_context") as WEBGL_lose_context | null)?.loseContext?.();
    }
    gl = undefined;
    program = undefined;
    texture = undefined;
    canvas?.remove();
    canvas = undefined;
  }

  function onContextLost(): void {
    releaseGl(false);
    removeResize();
    showFallback();
  }

  /** Draws each sign's word into its atlas row and measures it. */
  function drawAtlas(context: WebGL2RenderingContext): void {
    atlas ??= document.createElement("canvas");
    atlas.width = ATLAS_WIDTH_PX;
    atlas.height = ATLAS_ROW_PX * MAX_SIGNS;
    const paint = atlas.getContext("2d");
    const maxWordPx = ATLAS_WIDTH_PX - WORD_PAD_PX * 2;
    if (paint !== null) {
      paint.clearRect(0, 0, atlas.width, atlas.height);
      paint.fillStyle = "#fff";
      paint.textAlign = "center";
      paint.textBaseline = "middle";
      paint.font = fontReady ? WORD_FONT : FALLBACK_WORD_FONT;
    }
    signs.forEach((sign, index) => {
      const measured = paint?.measureText(sign.label).width ?? sign.label.length * GUESS_PX_PER_LETTER;
      const wordPx = Math.min(maxWordPx, measured);
      paint?.fillText(sign.label, ATLAS_WIDTH_PX / 2, (index + 0.55) * ATLAS_ROW_PX, maxWordPx);
      wordHalf[index] = (wordPx / 2 + WORD_PAD_PX) / ATLAS_WIDTH_PX;
    });
    if (paint === null || texture === undefined) return;
    context.bindTexture(context.TEXTURE_2D, texture);
    context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, context.RGBA, context.UNSIGNED_BYTE, atlas);
  }

  /** Lays the signs out in one centred row that fits the visible width. */
  function layOut(aspect: number): void {
    // A word's sign is as wide, relative to its height, as the word is in its atlas row.
    const halfWidths = signs.map((_, index) => SIGN_HALF_HEIGHT * ((wordHalf[index] * ATLAS_WIDTH_PX * 2) / ATLAS_ROW_PX));
    const visibleWidth = ((2 * (ROW_Z - CAMERA_Z)) / FOCAL) * aspect;
    const { centers, scale } = layoutRow(halfWidths, SIGN_GAP, visibleWidth * ROW_WIDTH_SHARE);
    signBox.fill(0);
    centers.forEach((center, index) => {
      signBox[index * 4] = center;
      signBox[index * 4 + 1] = halfWidths[index] * scale;
      signBox[index * 4 + 2] = SIGN_HALF_HEIGHT * scale;
      signBox[index * 4 + 3] = wordHalf[index];
    });
    const last = centers.length - 1;
    rowHalfWidth = last < 0 ? 0 : centers[last] + halfWidths[last] * scale + 0.1;
    rowHalfHeight = SIGN_HALF_HEIGHT * scale + 0.15;
  }

  function setUp(context: WebGL2RenderingContext): void {
    program = link(context);
    uniforms = Object.fromEntries(UNIFORMS.map((name) => [name, context.getUniformLocation(program as WebGLProgram, name)]));
    texture = context.createTexture() ?? undefined;
    context.bindTexture(context.TEXTURE_2D, texture ?? null);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MIN_FILTER, context.LINEAR);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_MAG_FILTER, context.LINEAR);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_S, context.CLAMP_TO_EDGE);
    context.texParameteri(context.TEXTURE_2D, context.TEXTURE_WRAP_T, context.CLAMP_TO_EDGE);
    context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, 1, 1, 0, context.RGBA, context.UNSIGNED_BYTE, new Uint8Array(4));
  }

  function start(element: HTMLElement, ctx: Ctx): void {
    root = element;
    const { options } = ctx;
    const fps = numberOption(options.fps, 30, 10, 60);
    const scale = numberOption(options.scale, 0.5, 0.25, 0.5);
    const grain = numberOption(options.grain, 0.6, 0, 1);
    const reducedMotion = window.matchMedia?.("(prefers-reduced-motion: reduce)").matches ?? false;

    const surface = document.createElement("canvas");
    surface.style.cssText = "display:block;width:100%;height:100%;background:#000";
    element.appendChild(surface);
    canvas = surface;
    const context = surface.getContext("webgl2", { alpha: false, antialias: false, depth: false, stencil: false, powerPreference: "low-power" });
    if (context === null) {
      releaseGl(false);
      showFallback();
      return;
    }
    gl = context;
    try {
      setUp(context);
    } catch (err) {
      console.error("[builtin.chrome-scene] WebGL setup failed", err);
      releaseGl(true);
      showFallback();
      return;
    }
    surface.addEventListener("webglcontextlost", onContextLost);

    const fit = (): void => {
      const size = renderSize(element.clientWidth, element.clientHeight, scale);
      surface.width = size.width;
      surface.height = size.height;
      dirty = true;
    };
    fit();
    onResize = fit;
    window.addEventListener("resize", onResize);

    if (scriptSrc !== "") {
      void loadFont(FONT_FAMILY, new URL("fonts/Anton-Regular.woff2", scriptSrc).href).then((ok) => {
        fontReady = ok;
        dirty = true;
      });
    }

    const startMs = ctx.now();
    let lastMs: number | undefined;
    let seconds = 0;
    const draw = (nowMs: number): void => {
      if (gl === undefined) return;
      const frameMs = Math.min(MAX_FRAME_MS, nowMs - (lastMs ?? nowMs - 1000 / fps));
      lastMs = nowMs;
      // Under reduced motion every clock stands still; only colors change.
      if (!reducedMotion) seconds += frameMs / 1000;
      const wallMs = ctx.now();
      pink = easeToward(pink, pinkGoal, frameMs, PINK_TAU_MS);
      const glitch = reducedMotion ? 0 : glitchAmount(glitchStartMs === undefined ? undefined : wallMs - glitchStartMs);

      if (dirty) {
        dirty = false;
        drawAtlas(gl);
        layOut(surface.width / Math.max(1, surface.height));
      }
      for (let index = 0; index < MAX_SIGNS; index++) {
        const sign = signs[index];
        const at = index * 4;
        signData[at] = sign === undefined ? 0 : STATE_CODES[sign.state];
        signData[at + 1] = sign === undefined ? 0 : flashAmount(sign, wallMs);
        signData[at + 2] = reducedMotion ? 0 : Math.sin(seconds * 0.35 + index * 1.7) * 0.07;
        signData[at + 3] = reducedMotion ? 1 : 0.5 + 0.5 * Math.sin(seconds * 1.6 + index * 1.1);
      }

      gl.viewport(0, 0, surface.width, surface.height);
      gl.useProgram(program ?? null);
      gl.uniform2f(uniforms.uResolution ?? null, surface.width, surface.height);
      gl.uniform1f(uniforms.uTime ?? null, reducedMotion ? 0 : (wallMs - startMs) / 1000);
      // Start well into the warp, so the first frame is already a liquid shape, not a ball.
      gl.uniform1f(uniforms.uWarpTime ?? null, seconds + WARP_START_S);
      gl.uniform1f(uniforms.uPink ?? null, pink);
      // The shape rises with the pink ease and settles back; under reduced motion it jumps.
      gl.uniform1f(uniforms.uLift ?? null, reducedMotion ? pinkGoal : pink);
      // From screen units to scene units at the lifted shape's depth.
      gl.uniform1f(uniforms.uLiftX ?? null, (liftShift(surface.width / Math.max(1, surface.height)) * (SHAPE_LIFT_Z - CAMERA_Z)) / FOCAL);
      gl.uniform1f(uniforms.uGlitch ?? null, glitch);
      gl.uniform1f(uniforms.uGrain ?? null, grain);
      gl.uniform1i(uniforms.uCount ?? null, signs.length);
      gl.uniform4fv(uniforms.uSigns ?? null, signData);
      gl.uniform4fv(uniforms.uSignBox ?? null, signBox);
      // The row follows the pink ease down and up; under reduced motion it jumps.
      gl.uniform1f(uniforms.uRowY ?? null, rowY(reducedMotion ? pinkGoal : pink));
      gl.uniform2f(uniforms.uRowHalf ?? null, rowHalfWidth, rowHalfHeight);
      gl.uniform1i(uniforms.uAtlas ?? null, 0);
      gl.activeTexture(gl.TEXTURE0);
      gl.bindTexture(gl.TEXTURE_2D, texture ?? null);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    };
    cancelFrame = ctx.requestFrame(draw, fps);
  }

  function update(state: ViewState): void {
    const before = approvalCount(signs);
    signs = signsFrom(state, signs, state.nowMs);
    pinkGoal = pinkTarget(state);
    if (approvalCount(signs) > before) glitchStartMs = state.nowMs;
    const next = signs.map((sign) => sign.label).join("\n");
    if (next !== labels) {
      labels = next;
      dirty = true;
    }
  }

  function stop(): void {
    canvas?.removeEventListener("webglcontextlost", onContextLost);
    releaseGl(true);
    removeResize();
    fallback?.remove();
    fallback = undefined;
    root = undefined;
    atlas = undefined;
    dirty = true;
  }

  return { start, update, stop };
}

window.agentScreensaver.register(createChromeScene);
