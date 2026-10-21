// Builds the screen saver's web page into ../build/runtime and the built-in
// extensions into ../build/extensions. Run from web/ with `npm run build`.
import { build } from "esbuild";
import { execFileSync } from "node:child_process";
import { copyFileSync, existsSync, mkdirSync, readdirSync } from "node:fs";
import { basename, dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(fileURLToPath(import.meta.url));
const outRuntime = join(root, "..", "build", "runtime");
const outExtensions = join(root, "..", "build", "extensions");
const builtinsDir = join(root, "src", "builtins");

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

/** `src/builtins/<name>.ts` becomes `extensions/builtin.<name>/index.js`, next to `<name>.extension.json`. */
async function buildBuiltins() {
  if (!existsSync(builtinsDir)) return; // none yet
  for (const file of readdirSync(builtinsDir).filter((name) => name.endsWith(".ts"))) {
    const name = basename(file, ".ts");
    const outDir = join(outExtensions, `builtin.${name}`);
    mkdirSync(outDir, { recursive: true });
    await build({ ...common, entryPoints: [join(builtinsDir, file)], outfile: join(outDir, "index.js") });
    const manifest = join(builtinsDir, `${name}.extension.json`);
    if (existsSync(manifest)) copyFileSync(manifest, join(outDir, "extension.json"));
  }
}

await buildRuntime();
await buildBuiltins();
