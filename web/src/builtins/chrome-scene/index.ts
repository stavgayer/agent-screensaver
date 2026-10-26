import type { BackgroundExt, Ctx, ViewState } from "../../api.ts";
import { numberOption } from "../matrix/logic.ts";
import { loadFont } from "../shared/fonts.ts";
import {
  MAX_SIGNS,
  approvalCount,
  easeToward,
  flashAmount,
  glitchAmount,
  pinkTarget,
  renderSize,
  signsFrom,
  type Sign,
  type SignState,
} from "./logic.ts";
import { ATLAS_CELL_PX, FRAGMENT_SHADER, VERTEX_SHADER } from "./shader.ts";

const FONT_FAMILY = "Chrome Scene Anton";
const LETTER_FONT = `${Math.round(ATLAS_CELL_PX * 0.9)}px "${FONT_FAMILY}", "Helvetica Neue", Impact, sans-serif`;
const FALLBACK_LETTER_FONT = `bold ${Math.round(ATLAS_CELL_PX * 0.8)}px "Helvetica Neue", Impact, sans-serif`;
const PINK_TAU_MS = 500;
const MAX_FRAME_MS = 250; // a long pause must not make the scene jump
const WARP_START_S = 40;
const STATE_CODES: Record<SignState, number> = { idle: 0, running: 1, finished: 2, approval: 3 };
const UNIFORMS = ["uResolution", "uTime", "uWarpTime", "uPink", "uGlitch", "uGrain", "uCount", "uSigns", "uAtlas"] as const;
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
 * A liquid chrome shape in a black studio, with one chrome sign per agent
 * card. Drawn by one fragment shader at a reduced size that CSS scales up.
 */
function createChromeScene(): BackgroundExt {
  let root: HTMLElement | undefined;
  let canvas: HTMLCanvasElement | undefined;
  let fallback: HTMLElement | undefined;
  let gl: WebGL2RenderingContext | undefined;
  let program: WebGLProgram | undefined;
  let texture: WebGLTexture | undefined;
  let atlas: HTMLCanvasElement | undefined;
  let atlasKey = "";
  let fontReady = false;
  let uniforms: Partial<Record<UniformName, WebGLUniformLocation | null>> = {};
  let cancelFrame: (() => void) | undefined;
  let onResize: (() => void) | undefined;

  let signs: Sign[] = [];
  let pinkGoal = 0;
  let pink = 0;
  let glitchStartMs: number | undefined;
  const signData = new Float32Array(MAX_SIGNS * 4);

  function showFallback(): void {
    if (root === undefined || fallback !== undefined) return;
    fallback = fallbackLayer();
    root.appendChild(fallback);
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
    showFallback();
  }

  /** Draws the signs' letters into the atlas when they change or the font arrives. */
  function refreshAtlas(context: WebGL2RenderingContext): void {
    const key = `${fontReady}:${signs.map((sign) => sign.letter).join("")}`;
    if (key === atlasKey || texture === undefined) return;
    atlasKey = key;
    atlas ??= document.createElement("canvas");
    atlas.width = ATLAS_CELL_PX * MAX_SIGNS;
    atlas.height = ATLAS_CELL_PX;
    const paint = atlas.getContext("2d");
    if (paint === null) return;
    paint.clearRect(0, 0, atlas.width, atlas.height);
    paint.fillStyle = "#fff";
    paint.textAlign = "center";
    paint.textBaseline = "middle";
    paint.font = fontReady ? LETTER_FONT : FALLBACK_LETTER_FONT;
    signs.forEach((sign, index) => paint.fillText(sign.letter, (index + 0.5) * ATLAS_CELL_PX, ATLAS_CELL_PX * 0.54));
    context.bindTexture(context.TEXTURE_2D, texture);
    context.texImage2D(context.TEXTURE_2D, 0, context.RGBA, context.RGBA, context.UNSIGNED_BYTE, atlas);
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
    const scale = numberOption(options.scale, 0.5, 0.25, 1);
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
    };
    fit();
    onResize = fit;
    window.addEventListener("resize", onResize);

    if (scriptSrc !== "") {
      void loadFont(FONT_FAMILY, new URL("fonts/Anton-Regular.woff2", scriptSrc).href).then((ok) => {
        fontReady = ok;
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

      signData.fill(0);
      signs.forEach((sign, index) => {
        const sway = reducedMotion ? 0 : Math.sin(seconds * 0.4 + index * 1.7) * 0.16;
        const pulse = reducedMotion ? 1 : 0.5 + 0.5 * Math.sin(seconds * 1.6 + index * 1.1);
        signData.set([STATE_CODES[sign.state], flashAmount(sign, wallMs), sway, pulse], index * 4);
      });

      refreshAtlas(gl);
      gl.viewport(0, 0, surface.width, surface.height);
      gl.useProgram(program ?? null);
      gl.uniform2f(uniforms.uResolution ?? null, surface.width, surface.height);
      gl.uniform1f(uniforms.uTime ?? null, reducedMotion ? 0 : (wallMs - startMs) / 1000);
      // Start well into the warp, so the first frame is already a liquid shape, not a ball.
      gl.uniform1f(uniforms.uWarpTime ?? null, seconds + WARP_START_S);
      gl.uniform1f(uniforms.uPink ?? null, pink);
      gl.uniform1f(uniforms.uGlitch ?? null, glitch);
      gl.uniform1f(uniforms.uGrain ?? null, grain);
      gl.uniform1i(uniforms.uCount ?? null, signs.length);
      gl.uniform4fv(uniforms.uSigns ?? null, signData);
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
  }

  function stop(): void {
    canvas?.removeEventListener("webglcontextlost", onContextLost);
    releaseGl(true);
    if (onResize !== undefined) window.removeEventListener("resize", onResize);
    onResize = undefined;
    fallback?.remove();
    fallback = undefined;
    root = undefined;
    atlas = undefined;
    atlasKey = "";
  }

  return { start, update, stop };
}

window.agentScreensaver.register(createChromeScene);
