import type { BackgroundExt, Ctx } from "../../api.ts";
import { addMedia, createStage, dimOption, fileUrl, fitOption } from "../shared/media.ts";

/**
 * A video that plays by itself. It is always muted: a screen saver that makes
 * sound by surprise is worse than one without, so there is no option for it.
 * It pauses while the page is hidden.
 */
function createVideo(): BackgroundExt {
  let stage: HTMLElement | undefined;
  let video: HTMLVideoElement | undefined;
  let onVisibility: (() => void) | undefined;

  function start(root: HTMLElement, ctx: Ctx): void {
    stage = createStage(dimOption(ctx.options.dim));
    root.appendChild(stage);
    const url = fileUrl(ctx.options.file);
    if (url === undefined) return; // no file chosen: plain black

    const element = document.createElement("video");
    element.muted = true;
    element.defaultMuted = true; // the `muted` attribute, which autoplay policies read
    element.loop = ctx.options.loop !== false;
    element.autoplay = true;
    element.playsInline = true;
    element.preload = "auto";
    element.disablePictureInPicture = true;
    addMedia(stage, element, fitOption(ctx.options.fit), "loadeddata");
    element.src = url;
    video = element;

    // A play() that is refused or interrupted is not an error here: the next show tries again.
    onVisibility = () => {
      if (document.hidden) element.pause();
      else void element.play().catch(() => undefined);
    };
    document.addEventListener("visibilitychange", onVisibility);
  }

  function stop(): void {
    if (onVisibility !== undefined) document.removeEventListener("visibilitychange", onVisibility);
    onVisibility = undefined;
    if (video !== undefined) {
      video.pause();
      // Removing the source and calling load() is what frees the decoder.
      video.removeAttribute("src");
      video.load();
      video = undefined;
    }
    stage?.remove();
    stage = undefined;
  }

  return { start, stop };
}

window.agentScreensaver.register(createVideo);
