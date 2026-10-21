import type { BackgroundExt, Ctx } from "../../api.ts";
import { addMedia, createStage, dimOption, fileUrl, fitOption } from "../shared/media.ts";

/** One still picture. It draws once, so it costs no CPU while it shows. */
function createImage(): BackgroundExt {
  let stage: HTMLElement | undefined;

  function start(root: HTMLElement, ctx: Ctx): void {
    stage = createStage(dimOption(ctx.options.dim));
    root.appendChild(stage);
    const url = fileUrl(ctx.options.file);
    if (url === undefined) return; // no file chosen: plain black

    const image = document.createElement("img");
    image.alt = "";
    image.decoding = "async";
    image.draggable = false;
    addMedia(stage, image, fitOption(ctx.options.fit), "load");
    image.src = url;
  }

  function stop(): void {
    stage?.remove();
    stage = undefined;
  }

  return { start, stop };
}

window.agentScreensaver.register(createImage);
