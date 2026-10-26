import { MAX_SIGNS } from "./logic.ts";

/** The atlas holds one row per sign slot; each row is one word. */
export const ATLAS_WIDTH_PX = 512;
export const ATLAS_ROW_PX = 128;

/** Scene layout, shared by the shader and the code that lays out the signs. */
export const CAMERA_Z = -5.4;
export const FOCAL = 2.0;
export const ROW_Z = -0.6;
/** Half the height of a sign before the row is shrunk to fit. */
export const SIGN_HALF_HEIGHT = 0.21;

/** One triangle that covers the screen, made from the vertex index: no buffers. */
export const VERTEX_SHADER = `#version 300 es
void main() {
  vec2 corner = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;

const glsl = (value: number): string => (Number.isInteger(value) ? value.toFixed(1) : String(value));

/**
 * Ray marches one liquid chrome shape and a centred row of up to MAX_SIGNS
 * chrome signs, lit by a bright studio made in code, then adds film grain,
 * scan lines and a light chromatic offset in the same pass. Kept cheap: a
 * bounded number of steps, an early exit, and the signs measured only while
 * the ray is inside the box around their row.
 */
export const FRAGMENT_SHADER = `#version 300 es
precision highp float;

#define MAX_SIGNS ${MAX_SIGNS}
#define MAX_STEPS 60
#define FAR 12.0

uniform vec2 uResolution;
uniform float uTime;      // seconds; grain and glitch noise
uniform float uWarpTime;  // seconds; stands still under reduced motion
uniform float uPink;      // 0 calm, 1 full pink
uniform float uGlitch;    // 0 none, 1 full burst
uniform float uGrain;
uniform int uCount;
uniform vec4 uSigns[MAX_SIGNS];   // state code, lime flash, sway angle, pulse
uniform vec4 uSignBox[MAX_SIGNS]; // centre x, half width, half height, half width of the word in the atlas (0..0.5)
uniform float uRowY;               // height of the sign row, see rowY in logic.ts
uniform vec2 uRowHalf;            // half width and half height of the box around the row
uniform sampler2D uAtlas;

out vec4 outColor;

const vec3 LIME = vec3(0.784, 1.0, 0.18);
const vec3 PINK = vec3(1.0, 0.16, 0.56);
const vec3 SHAPE_CENTER = vec3(0.0, 0.62, 0.0);
const float CAMERA_Z = ${glsl(CAMERA_Z)};
const float FOCAL = ${glsl(FOCAL)};
const float ROW_Z = ${glsl(ROW_Z)};

float hash(vec2 p) {
  p = fract(p * vec2(123.34, 456.21));
  p += dot(p, p + 45.32);
  return fract(p.x * p.y);
}

mat2 rot(float a) {
  float c = cos(a), s = sin(a);
  return mat2(c, -s, s, c);
}

float roundBox(vec3 p, vec3 size, float radius) {
  vec3 q = abs(p) - size + radius;
  return length(max(q, 0.0)) + min(max(q.x, max(q.y, q.z)), 0.0) - radius;
}

float shape(vec3 p) {
  float t = uWarpTime;
  p -= SHAPE_CENTER;
  p.xz *= rot(t * 0.06);
  p.xy *= rot(sin(t * 0.05) * 0.5);
  p.x *= 0.86;
  float d = length(p) - 1.04;
  d += 0.19 * sin(1.9 * p.x + t * 0.5) * sin(2.2 * p.y - t * 0.4) * sin(1.7 * p.z + t * 0.35);
  d += 0.02 * sin(4.3 * p.x - t * 0.8) * sin(3.9 * p.y + t * 0.6) * sin(4.1 * p.z - t * 0.5);
  // The warp bends distances; a smaller step keeps the march from passing through.
  return d * 0.65;
}

vec3 signLocal(vec3 p, int index) {
  vec3 q = p - vec3(uSignBox[index].x, uRowY, ROW_Z);
  q.xz *= rot(uSigns[index].z);
  // Leaned back a little, so the face catches the top light.
  q.yz *= rot(0.18);
  return q;
}

float signDistance(vec3 p, int index) {
  vec4 box = uSignBox[index];
  return roundBox(signLocal(p, index), vec3(box.y, box.z, box.z * 0.16), box.z * 0.14);
}

// x: distance, y: what was hit (-1 shape, 0.. sign index), z: nearest sign distance, w: its index.
vec4 scene(vec3 p) {
  vec4 hit = vec4(shape(p), -1.0, FAR, -1.0);
  if (uCount == 0) return hit;
  float row = roundBox(p - vec3(0.0, uRowY, ROW_Z), vec3(uRowHalf, 0.5), 0.0);
  if (row > hit.x) {
    hit.z = row;
    return hit;
  }
  for (int i = 0; i < MAX_SIGNS; i++) {
    if (i >= uCount) break;
    float d = signDistance(p, i);
    if (d < hit.z) hit.zw = vec2(d, float(i));
    if (d < hit.x) hit.xy = vec2(d, float(i));
  }
  return hit;
}

vec3 normalAt(vec3 p) {
  const vec2 e = vec2(0.0015, -0.0015);
  return normalize(
    e.xyy * scene(p + e.xyy).x + e.yyx * scene(p + e.yyx).x +
    e.yxy * scene(p + e.yxy).x + e.xxx * scene(p + e.xxx).x);
}

float band(float x, float center, float width, float softness) {
  return smoothstep(width + softness, width, abs(x - center));
}

// The studio the chrome reflects: a bright ceiling, tall soft boxes all
// round, a pale floor with a soft horizon glow, and one acid strip light
// that turns pink with uPink. Values above 1 are highlights.
vec3 studio(vec3 d) {
  float a = atan(d.x, d.z);
  float y = d.y;
  vec3 c = mix(vec3(0.07), vec3(0.32), smoothstep(0.05, 0.9, y));
  // A pale floor sweep with soft light pools, darker only straight down.
  vec3 floorColor = vec3(0.16) + vec3(0.9) * smoothstep(-0.05, -0.5, y) * (0.5 + 0.5 * cos(a * 2.0 + 0.8));
  floorColor *= smoothstep(-1.0, -0.7, y) * 0.8 + 0.2;
  c = mix(floorColor, c, smoothstep(-0.12, 0.0, y));
  c += vec3(1.1) * exp(-abs(y) * 7.0);
  c += vec3(3.2) * smoothstep(0.62, 0.8, y);
  float tall = smoothstep(-0.35, 0.15, y) * smoothstep(0.95, 0.6, y);
  c += vec3(2.6) * band(a, 2.75, 0.12, 0.1) * tall;
  c += vec3(2.2) * band(a, -2.65, 0.08, 0.08) * tall;
  c += vec3(1.4) * band(a, 1.35, 0.1, 0.12) * tall;
  c += vec3(1.1) * band(a, -1.2, 0.06, 0.1) * tall;
  vec3 accent = mix(LIME, PINK, uPink);
  c += accent * 3.0 * band(a, -2.2, 0.035, 0.06) * smoothstep(-0.5, 0.0, y) * smoothstep(0.85, 0.4, y);
  return mix(c, c * vec3(1.0, 0.62, 0.86), uPink * 0.55);
}

vec3 chrome(vec3 reflected, float spread) {
  vec3 o = vec3(spread, 0.0, -spread * 0.5);
  return vec3(
    studio(normalize(reflected + o)).r,
    studio(reflected).g,
    studio(normalize(reflected - o)).b);
}

// Thin oil-film colours that show at grazing angles.
vec3 film(float x) {
  return 0.5 + 0.5 * cos(6.2832 * (x + vec3(0.0, 0.33, 0.67)));
}

vec3 signGlow(int index) {
  vec4 s = uSigns[index];
  if (s.x > 2.5) return PINK * (1.6 + uGlitch * hash(vec2(uTime, float(index))) * 1.5);
  // Finished: a white-hot lime flash that settles to a calm pale lime.
  if (s.x > 1.5) return mix(mix(vec3(1.0), LIME, 0.45) * 0.95, vec3(2.4, 2.8, 1.6), s.y);
  if (s.x > 0.5) return LIME * 1.5;
  // Idle: dark ink, as if cut into the plate.
  return vec3(0.025);
}

float glowStrength(int index) {
  vec4 s = uSigns[index];
  if (s.x > 2.5) return 1.0;
  if (s.x > 1.5) return 0.35 + s.y * 1.6;
  if (s.x > 0.5) return 0.45 + 0.35 * s.w;
  return 0.0;
}

vec3 shadeSign(vec3 p, vec3 base, int index) {
  vec3 q = signLocal(p, index);
  vec4 box = uSignBox[index];
  vec3 color = base * 0.42;
  if (q.z < -box.z * 0.1) {
    vec2 uv = q.xy / box.yz;
    if (abs(uv.x) > 1.0 || abs(uv.y) > 1.0) return color;
    vec2 cell = vec2(0.5 + uv.x * box.w, (float(index) + 0.5 - uv.y * 0.5) / float(MAX_SIGNS));
    float word = texture(uAtlas, cell).a;
    color = mix(color, signGlow(index), word);
    color += LIME * uSigns[index].y * 0.35 * (1.0 - word);
  }
  return color;
}

void main() {
  vec2 frag = gl_FragCoord.xy;
  vec2 screen = (frag * 2.0 - uResolution) / uResolution.y;

  vec2 uv = screen;
  if (uGlitch > 0.0) {
    float rowBand = floor(frag.y / uResolution.y * 22.0);
    float h = hash(vec2(rowBand, floor(uTime * 24.0)));
    uv.x += step(0.72, h) * (h - 0.86) * 0.9 * uGlitch;
  }

  vec3 ro = vec3(0.0, 0.0, CAMERA_Z);
  vec3 rd = normalize(vec3(uv, FOCAL));

  float t = 0.0;
  vec4 hit = vec4(FAR, -2.0, FAR, -1.0);
  float nearest = FAR;
  float nearestIndex = -1.0;
  for (int i = 0; i < MAX_STEPS; i++) {
    vec4 h = scene(ro + rd * t);
    if (h.z < nearest) {
      nearest = h.z;
      nearestIndex = h.w;
    }
    if (h.x < 0.0012) {
      hit = h;
      break;
    }
    t += h.x;
    if (t > FAR) break;
  }

  vec3 color = vec3(0.0);
  if (hit.y > -1.5) {
    vec3 p = ro + rd * t;
    vec3 n = normalAt(p);
    vec3 r = reflect(rd, n);
    float facing = max(dot(n, -rd), 0.0);
    float fresnel = pow(1.0 - facing, 4.0);
    vec3 base = chrome(r, 0.03 + uGlitch * 0.08) * (0.72 + 0.5 * fresnel);
    base *= mix(vec3(1.0), film(facing * 1.6 + 0.15), 0.35 * smoothstep(0.55, 0.0, facing));
    // Acid rim light from the strip, pink while an approval is open.
    base += mix(LIME, PINK, uPink) * fresnel * 0.9 * smoothstep(0.1, 0.7, -n.x);
    color = hit.y < -0.5 ? base : shadeSign(p, base, int(hit.y));
  }
  if (nearestIndex > -0.5) {
    int index = int(nearestIndex);
    float halo = exp(-nearest * 14.0) * glowStrength(index) * (hit.y > -0.5 ? 0.12 : 0.3);
    color += signGlow(index) * halo;
  }

  // Tone map on the brightest channel, so lime stays acid instead of going olive.
  float peak = max(max(color.r, color.g), color.b);
  color *= (1.0 - exp(-peak * 1.25)) / max(peak, 1e-4);

  color *= 1.0 - 0.32 * dot(screen * 0.5, screen * 0.5);
  color *= 1.0 - 0.09 * mod(floor(frag.y), 2.0);
  color += (hash(frag + fract(uTime) * vec2(113.1, 71.7)) - 0.5) * uGrain * 0.11;
  outColor = vec4(clamp(color, 0.0, 1.0), 1.0);
}
`;
