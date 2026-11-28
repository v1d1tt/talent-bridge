import { createEffect, createMemo, createSignal } from "solid-js";
import { messages as m } from "../messages";
import { drawSkeleton, fitFrame, fitView, inkFrom } from "./skeleton";
import type { PoseSequence } from "./types";

const seconds = (frame: number, fps: number) => (frame / fps).toFixed(2);

/**
 * The skeleton plate: the stick figure of one frame of a pose sequence,
 * with a frame slider. Used for an athlete's analysis and for a pro
 * reference (only the figure is shown; no picture of the clip). The
 * figure is zoomed so the whole movement fills the plate (fitView).
 */
export function Plate(props: { seq: PoseSequence; start: number; caption: string }) {
  const [frame, setFrame] = createSignal<number>();
  const [canvas, setCanvas] = createSignal<HTMLCanvasElement>();
  const shown = () => frame() ?? props.start;
  const label = () => m.plate.frame(shown() + 1, props.seq.frames.length, seconds(shown(), props.seq.fps));
  const view = createMemo(() => fitView(props.seq.frames));

  createEffect(
    () => ({ c: canvas(), f: shown(), s: props.seq, v: view() }),
    ({ c, f, s, v }) => {
      const ctx = c?.getContext("2d");
      if (!c || !ctx) return;
      c.width = 720;
      c.height = Math.round((720 * s.height) / s.width);
      const shownFrame = s.frames[f];
      drawSkeleton(ctx, shownFrame && fitFrame(shownFrame, v), inkFrom(c));
    },
  );

  return (
    <figure class="plate-figure">
      <figcaption class="sheet__title">{props.caption}</figcaption>
      <div class="plate" style={{ "aspect-ratio": `${props.seq.width} / ${props.seq.height}` }}>
        <canvas ref={setCanvas} role="img" aria-label={label()} />
      </div>
      <label class="slider">
        <span>{label()}</span>
        <input
          type="range"
          min="0"
          max={props.seq.frames.length - 1}
          value={shown()}
          aria-label={m.plate.frameLabel}
          onInput={(e) => setFrame(Number(e.currentTarget.value))}
        />
      </label>
    </figure>
  );
}
