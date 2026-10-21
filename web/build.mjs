// Builds the screen saver's web page into ../build/runtime and the built-in
// extensions into ../build/extensions. Run from web/ with `npm run build`.
import { build } from "esbuild";
import { execFileSync } from "node:child_process";
import { copyFileSync, cpSync, existsSync, mkdirSync, readdirSync, rmSync } from "node:fs";
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
await buildBuiltins();
