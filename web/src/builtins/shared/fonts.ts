const loads = new Map<string, Promise<boolean>>();

async function load(family: string, url: string, descriptors?: FontFaceDescriptors): Promise<boolean> {
  try {
    const face = new FontFace(family, `url(${url})`, descriptors);
    await face.load();
    // The bundled DOM typings omit `add`, which every WebKit has.
    (document.fonts as unknown as { add(face: FontFace): void }).add(face);
    return true;
  } catch {
    // No FontFace, a missing file or a blocked load: the caller keeps its fallback font.
    return false;
  }
}

/**
 * Loads a font file with the standard `FontFace` API, once per family per page.
 * `url` is a `file://` URL next to the extension's entry file (the page's CSP
 * allows `font-src file:` only). Resolves `false`, never throws, when the
 * font cannot load, so a component falls back to the system font.
 */
export function loadFont(family: string, url: string, descriptors?: FontFaceDescriptors): Promise<boolean> {
  let result = loads.get(family);
  if (result === undefined) {
    result = load(family, url, descriptors);
    loads.set(family, result);
  }
  return result;
}
