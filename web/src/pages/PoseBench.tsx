import { createSignal, For, Show } from "solid-js";
import { messages as m } from "../messages";
import type { Landmark } from "../pose/types";

type Row = {
  model: string;
  delegate: string;
  loadMs: number;
  meanMs: number;
  p95Ms: number;
  seen: number;
  frames: number;
  jitter: number;
};

/**
 * Mean change in landmark speed between frames (a second difference), in
 * thousandths of the frame size, over landmarks seen in all three frames.
 * A model that tracks steadily has low jitter on the same clip.
 */
export function jitter(frames: Landmark[][]): number {
  let sum = 0;
  let n = 0;
  for (let f = 1; f + 1 < frames.length; f++) {
    for (let l = 0; l < frames[f].length; l++) {
      const [a, b, c] = [frames[f - 1][l], frames[f][l], frames[f + 1][l]];
      if (a[2] < 0.5 || b[2] < 0.5 || c[2] < 0.5) continue;
      sum += Math.hypot(c[0] - 2 * b[0] + a[0], c[1] - 2 * b[1] + a[1]);
      n++;
    }
  }
  return n ? (sum / n) * 1000 : NaN;
}

function percentile(xs: number[], p: number): number {
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.min(s.length - 1, Math.floor((p / 100) * s.length))] ?? NaN;
}

/** /lab/pose-benchmark: lite vs full on one clip, on this device. */
export function PoseBench() {
  const [rows, setRows] = createSignal<Row[]>([]);
  const [status, setStatus] = createSignal("");
  const [running, setRunning] = createSignal(false);
  const [copied, setCopied] = createSignal(false);

  async function run(file: File) {
    setRunning(true);
    setRows([]);
    setCopied(false);
    const { closeClip, extractPose, openClip } = await import("../pose/extract");
    const { loadLandmarker } = await import("../pose/landmarker");
    const out: Row[] = [];
    let video: HTMLVideoElement | undefined;
    try {
      video = await openClip(file);
      for (const model of ["lite", "full"] as const) {
        const t0 = performance.now();
        const loaded = await loadLandmarker(model);
        const loadMs = performance.now() - t0;
        // One warm-up frame: the first GPU call compiles shaders.
        loaded.landmarker.detectForVideo(video, loaded.nextTimestampMs);
        loaded.nextTimestampMs += 1000;
        const { sequence, msPerFrame } = await extractPose(video, loaded, (p) =>
          setStatus(m.bench.running(model, p.done, p.total)),
        ).catch(async (err: unknown) => {
          // A clip with no person still gives timing numbers.
          if (err instanceof Error && err.message === "no_person") return { sequence: { frames: [] as Landmark[][] }, msPerFrame: [] };
          throw err;
        });
        loaded.landmarker.close();
        out.push({
          model,
          delegate: loaded.delegate,
          loadMs,
          meanMs: msPerFrame.reduce((s, x) => s + x, 0) / (msPerFrame.length || 1),
          p95Ms: percentile(msPerFrame, 95),
          seen: sequence.frames.filter((f) => f.some((l) => l[2] > 0)).length,
          frames: sequence.frames.length,
          jitter: jitter(sequence.frames),
        });
        setRows([...out]);
      }
      setStatus("");
    } catch (err) {
      setStatus(String(err));
    } finally {
      if (video) closeClip(video);
      setRunning(false);
    }
  }

  const report = () =>
    JSON.stringify(
      { device: navigator.userAgent, rows: rows().map((r) => ({ ...r, loadMs: Math.round(r.loadMs) })) },
      null,
      1,
    );

  return (
    <section class="sheet" aria-labelledby="bench-heading">
      <h1 id="bench-heading" class="page__title">
        {m.bench.heading}
      </h1>
      <p class="intro">{m.bench.intro}</p>
      <Show when={!running()}>
        <div class="pick">
          <label class="action action--file">
            <input
              type="file"
              accept="video/*"
              class="sr-only"
              onChange={(e) => {
                const f = e.currentTarget.files?.[0];
                e.currentTarget.value = "";
                if (f) void run(f);
              }}
            />
            {m.bench.choose}
          </label>
        </div>
      </Show>
      <p class="summary" role="status">
        {status()}
      </p>
      <Show when={rows().length > 0}>
        <table class="ruled bench">
          <thead>
            <tr>
              <th scope="col">{m.bench.model}</th>
              <th scope="col">{m.bench.delegate}</th>
              <th scope="col" class="num">{m.bench.load}</th>
              <th scope="col" class="num">{m.bench.mean}</th>
              <th scope="col" class="num">{m.bench.p95}</th>
              <th scope="col" class="num">{m.bench.seen}</th>
              <th scope="col" class="num">{m.bench.jitter}</th>
            </tr>
          </thead>
          <tbody>
            <For each={rows()}>
              {(r) => (
                <tr>
                  <td>{r.model}</td>
                  <td>{r.delegate}</td>
                  <td class="num">{Math.round(r.loadMs)} ms</td>
                  <td class="num">{r.meanMs.toFixed(1)} ms</td>
                  <td class="num">{r.p95Ms.toFixed(1)} ms</td>
                  <td class="num">
                    {r.seen} / {r.frames}
                  </td>
                  <td class="num">{r.jitter.toFixed(2)}</td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
        <p class="note">{m.bench.jitterHint}</p>
        <p class="note">
          {m.bench.device}: {navigator.userAgent}
        </p>
        <button
          type="button"
          class="action"
          onClick={() => void navigator.clipboard.writeText(report()).then(() => setCopied(true))}
        >
          {copied() ? m.bench.copied : m.bench.copy}
        </button>
      </Show>
    </section>
  );
}
