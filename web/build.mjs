// Builds the screen saver's web page into ../build/runtime and the built-in
// extensions into ../build/extensions. Run from web/ with `npm run build`.
import { build } from "esbuild";
import { execFileSync } from "node:child_process";
import { copyFileSync, cpSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const outRuntime = join(root, "..", "build", "runtime");
const outExtensions = join(root, "..", "build", "extensions");
const builtinsDir = join(root, "src", "builtins");
const manifestsDir = join(root, "..", "extensions");

// WebKit refuses ES modules on file://, so every bundle is a classic script.
const common = { bundle: true, format: "iife", target: "es2022", logLevel: "warning" };

async function buildRuntime() {
  mkdirSync(outRuntime, { recursive: true });
  await build({ ...common, entryPoints: [join(root, "src", "runtime.ts")], outfile: join(outRuntime, "runtime.js") });
  for (const file of ["index.html", "styles.css"]) copyFileSync(join(root, file), join(outRuntime, file));
  execFileSync(
    join(root, "node_modules", ".bin", "tsc"),
    [join(root, "src", "api.ts"), "--declaration", "--emitDeclarationOnly", "--skipLibCheck",
      "--target", "ES2022", "--lib", "ES2022,DOM", "--outDir", outRuntime],
    { stdio: "inherit" },
  );
}

/**
 * The settings window's page goes to `build/runtime/settings/`. Its live
 * preview is `build/runtime/preview.html`: the screen saver's own page plus
 * one script that takes the state from the settings page, so the preview is
 * the real runtime and its markup is never copied by hand.
 */
async function buildSettings() {
  const outSettings = join(outRuntime, "settings");
  mkdirSync(outSettings, { recursive: true });
  for (const name of ["settings", "preview"]) {
    await build({ ...common, entryPoints: [join(root, "settings", `${name}.ts`)], outfile: join(outSettings, `${name}.js`) });
  }
  for (const file of ["index.html", "settings.css"]) copyFileSync(join(root, "settings", file), join(outSettings, file));
  const runtimeTag = '<script src="runtime.js"></script>';
  const page = readFileSync(join(root, "index.html"), "utf8");
  if (!page.includes(runtimeTag)) throw new Error(`index.html has no ${runtimeTag}`);
  writeFileSync(join(outRuntime, "preview.html"), page.replace(runtimeTag, `${runtimeTag}\n  <script src="settings/preview.js"></script>`));
}

/**
 * Every folder under the repo's `extensions/` is copied to `build/extensions/<id>/`.
 * A built-in with code (`src/builtins/<name>/index.ts`) is bundled to
 * `build/extensions/builtin.<name>/index.js` next to its manifest.
 */
async function buildBuiltins() {
  rmSync(outExtensions, { recursive: true, force: true });
  mkdirSync(outExtensions, { recursive: true });
  cpSync(manifestsDir, outExtensions, { recursive: true });
  if (!existsSync(builtinsDir)) return;
  for (const entry of readdirSync(builtinsDir, { withFileTypes: true })) {
    const source = join(builtinsDir, entry.name, "index.ts");
    if (!entry.isDirectory() || !existsSync(source)) continue;
    await build({
      ...common,
      entryPoints: [source],
      outfile: join(outExtensions, `builtin.${entry.name}`, "index.js"),
    });
  }
}

await buildRuntime();
await buildSettings();
await buildBuiltins();
