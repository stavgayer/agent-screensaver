import { MAX_SIGNS } from "./logic.ts";

/** Letter cells in the atlas texture, side by side, one per sign slot. */
export const ATLAS_CELL_PX = 128;

/** One triangle that covers the screen, made from the vertex index: no buffers. */
export const VERTEX_SHADER = `#version 300 es
void main() {
  vec2 corner = vec2(float((gl_VertexID << 1) & 2), float(gl_VertexID & 2));
  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);
}
`;

/**
 * Ray marches one liquid chrome shape and up to MAX_SIGNS chrome slabs, lit
 * by an environment made in code, then adds film grain, scan lines and a
 * light chromatic offset in the same pass. Kept cheap: a bounded number of
 * steps, an early exit, and the slabs skipped while the ray is far from
 * their row.
 */
export const FRAGMENT_SHADER = `#version 300 es
precision highp float;

#define MAX_SIGNS ${MAX_SIGNS}
#define MAX_STEPS 56
#define FAR 12.0

uniform vec2 uResolution;
uniform float uTime;      // seconds; grain, pulse and glitch noise
uniform float uWarpTime;  // seconds; stands still under reduced motion
uniform float uPink;      // 0 calm, 1 full pink
uniform float uGlitch;    // 0 none, 1 full burst
uniform float uGrain;
uniform int uCount;
uniform vec4 uSigns[MAX_SIGNS]; // state code, lime flash, sway angle, pulse
uniform sampler2D uAtlas;

out vec4 outColor;

const vec3 LIME = vec3(0.776, 1.0, 0.0);
const vec3 PINK = vec3(1.0, 0.13, 0.52);
const vec3 SLAB = vec3(0.36, 0.48, 0.06);
const float SLAB_ROUND = 0.05;
// Signs stand in two short arcs left and right of the shape, newest nearest,
// leaving the middle and the bottom of the screen to the clock and the cards.
const float SIGN_INNER_X = 1.95;
const float SIGN_STEP_X = 0.95;
const float SIGN_Y = 0.2;
const float SIGN_Z = 0.3;
const float SIGN_TILT = 0.2;

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

float blob(vec3 p) {
  float t = uWarpTime;
  p -= vec3(0.0, 0.42, 0.0);
  p.xz *= rot(t * 0.07);
  p.xy *= rot(t * 0.045);
  p.x *= 0.82;
  float d = length(p) - 1.1;
  d += 0.2 * sin(2.1 * p.x + t * 0.55) * sin(2.4 * p.y - t * 0.45) * sin(1.9 * p.z + t * 0.4);
  d += 0.05 * sin(5.3 * p.x - t * 0.9) * sin(4.7 * p.z + t * 0.7);
  // The warp bends distances; a smaller step keeps the march from passing through.
  return d * 0.62;
}

// Narrow screens pull the arcs in so they stay in view.
float squeeze() {
  return clamp(uResolution.x / uResolution.y / 1.6, 0.7, 1.0);
}

float signSide(int index) {
  return index % 2 == 0 ? -1.0 : 1.0;
}

vec3 signCenter(int index) {
  float k = float(index / 2);
  return vec3(signSide(index) * (SIGN_INNER_X + k * SIGN_STEP_X) * squeeze(), SIGN_Y - k * 0.14, SIGN_Z + k * 0.25);
}

vec3 signLocal(vec3 p, int index) {
  vec3 q = p - signCenter(index);
  // Each sign turns a little toward the shape, then sways.
  float k = float(index / 2);
  q.xz *= rot(signSide(index) * (0.32 + k * 0.08) + uSigns[index].z);
  // Leaned back a little, so the face catches the top light.
  q.yz *= rot(SIGN_TILT);
  return q;
}

// x: distance, y: what was hit (-1 shape, 0.. sign index). z: nearest sign distance, w: its index.
vec4 scene(vec3 p) {
  vec4 hit = vec4(blob(p), -1.0, FAR, -1.0);
  if (uCount == 0) return hit;
  // One box around each arc; the slabs are only measured once the ray is close.
  float arcMid = (SIGN_INNER_X + SIGN_STEP_X) * squeeze();
  vec3 arcHalf = vec3(SIGN_STEP_X * squeeze() + 0.6, SLAB.y + 0.3, 0.75);
  float row = roundBox(vec3(abs(p.x) - arcMid, p.y - SIGN_Y + 0.14, p.z - SIGN_Z - 0.25), arcHalf, 0.0);
  if (row > hit.x) {
    hit.z = row;
    return hit;
  }
  for (int i = 0; i < MAX_SIGNS; i++) {
    if (i >= uCount) break;
    float d = roundBox(signLocal(p, i), SLAB, SLAB_ROUND);
    if (d < hit.z) hit.zw = vec2(d, float(i));
    if (d < hit.x) hit.xy = vec2(d, float(i));
  }
  return hit;
}

vec3 normalAt(vec3 p) {
  const vec2 e = vec2(0.002, -0.002);
  return normalize(
    e.xyy * scene(p + e.xyy).x + e.yyx * scene(p + e.yyx).x +
    e.yxy * scene(p + e.yxy).x + e.xxx * scene(p + e.xxx).x);
}

// The studio the chrome reflects: black, two tall soft boxes, a top strip
// and a thin acid line on the horizon that turns pink with uPink.
vec3 environment(vec3 d) {
  float a = atan(d.x, d.z);
  float y = d.y;
  float lit = smoothstep(-0.25, 0.3, y);
  vec3 c = vec3(0.015) + vec3(0.16) * smoothstep(0.0, 1.0, y);
  c += vec3(1.8) * smoothstep(0.9, 0.99, cos(a - 2.7)) * lit;
  c += vec3(1.3) * smoothstep(0.94, 0.995, cos(a + 2.45)) * lit;
  c += vec3(0.7) * smoothstep(0.96, 0.998, cos(a - 1.3)) * lit;
  c += vec3(0.5) * smoothstep(0.96, 0.998, cos(a + 0.9)) * lit;
  c += vec3(1.2) * smoothstep(0.07, 0.0, abs(y - 0.62));
  vec3 line = mix(LIME, PINK, uPink);
  c += line * (exp(-abs(y + 0.05) * 30.0) * 1.3 + exp(-abs(y + 0.05) * 5.0) * 0.1);
  c *= mix(0.12, 1.0, smoothstep(-0.6, -0.04, y));
  c += vec3(0.1) * smoothstep(0.12, 0.0, abs(y + 0.32)) * smoothstep(0.2, 0.9, cos(a - 2.9));
  return mix(c, c * vec3(1.0, 0.4, 0.72) + PINK * 0.06, uPink * 0.75);
}

vec3 chrome(vec3 reflected, float spread) {
  vec3 o = vec3(spread, 0.0, -spread * 0.5);
  return vec3(
    environment(normalize(reflected + o)).r,
    environment(reflected).g,
    environment(normalize(reflected - o)).b);
}

vec3 signGlow(int index) {
  vec4 s = uSigns[index];
  if (s.x > 2.5) return PINK * (1.25 + uGlitch * hash(vec2(uTime, float(index))) * 1.5);
  if (s.x > 1.5) return mix(LIME * 0.3 + 0.08, LIME * 2.4 + 0.4, s.y);
  if (s.x > 0.5) return LIME * (0.5 + 0.55 * s.w);
  return vec3(0.16);
}

vec3 shadeSign(vec3 p, vec3 n, vec3 rd, int index, vec3 base) {
  vec3 q = signLocal(p, index);
  vec3 glow = signGlow(index);
  vec3 color = base * 0.7;
  if (q.z < -SLAB.z + 0.01) {
    vec2 uv = q.xy / SLAB.xy;
    vec2 cell = vec2((float(index) + uv.x * 0.5 + 0.5) / float(MAX_SIGNS), 0.5 - uv.y * 0.5);
    float letter = texture(uAtlas, cell).a;
    color = mix(color, glow, letter) + glow * letter * 0.5;
  }
  return color;
}

void main() {
  vec2 frag = gl_FragCoord.xy;
  vec2 screen = (frag * 2.0 - uResolution) / uResolution.y;

  vec2 uv = screen;
  if (uGlitch > 0.0) {
    float band = floor(frag.y / uResolution.y * 22.0);
    float h = hash(vec2(band, floor(uTime * 24.0)));
    uv.x += step(0.72, h) * (h - 0.86) * 0.9 * uGlitch;
  }

  vec3 ro = vec3(0.0, 0.0, -5.4);
  vec3 rd = normalize(vec3(uv, 2.0));

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
    if (h.x < 0.0015) {
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
    float fresnel = pow(1.0 - max(dot(n, -rd), 0.0), 3.0);
    vec3 base = chrome(r, 0.035 + uGlitch * 0.08) * (0.8 + 0.6 * fresnel);
    base += PINK * fresnel * uPink * 0.7;
    color = hit.y < -0.5 ? base : shadeSign(p, n, rd, int(hit.y), base);
  }
  if (nearestIndex > -0.5) {
    float halo = exp(-nearest * 11.0) * (hit.y > -1.5 ? 0.06 : 0.22);
    color += signGlow(int(nearestIndex)) * halo;
  }

  color = 1.0 - exp(-color * 1.15);
  color *= 1.0 - 0.5 * dot(screen * 0.55, screen * 0.55);
  color *= 1.0 - 0.16 * mod(floor(frag.y), 2.0);
  color += (hash(frag + fract(uTime) * vec2(113.1, 71.7)) - 0.5) * uGrain * 0.13;
  outColor = vec4(max(color, 0.0), 1.0);
}
`;
