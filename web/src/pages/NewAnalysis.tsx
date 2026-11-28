import { useNavigate } from "@solidjs/router";
import { createSignal, Loading, Match, onCleanup, Show, Switch } from "solid-js";
import { AnalysisError, checkKeyFrames, createAnalysis, uploadKeyFrame } from "../analyses";
import { type Me } from "../me";
import { messages as m } from "../messages";
import { pickKeyFrames } from "../pose/keyframes";
import { drawSkeleton, inkFrom } from "../pose/skeleton";
import { useSession } from "../session";

/** /analyses/new: pick or record a clip, read it on the device, save it. */
export function NewAnalysis() {
  const session = useSession();
  return (
    <section class="sheet" aria-labelledby="new-heading">
      <h1 id="new-heading" class="page__title">
        {m.newAnalysis.heading}
      </h1>
      <Loading fallback={<p class="summary" role="status" />}>
        <Show
          when={session.me()}
          fallback={
            <p class="intro">
              {m.newAnalysis.signedOut} <a href="/signin">{m.nav.signIn}</a>
            </p>
          }
        >
          {(me) => (
            <Show when={me().user.role === "athlete"} fallback={<p class="intro">{m.newAnalysis.notAthlete}</p>}>
              <ClipReader me={me()} />
            </Show>
          )}
        </Show>
      </Loading>
    </section>
  );
}

/**
 * The steps of one clip. Each step is one state; the page shows exactly
 * what the current state says.
 */
type Step =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "reading"; done: number; total: number }
  | { kind: "saving"; what: "numbers" | "pictures" | "checking" }
  | { kind: "failed"; message: string };

// The pose model is loaded once per page visit and reused for every clip.
let model: Promise<import("../pose/landmarker").Loaded> | undefined;

function ClipReader(props: { me: Me }) {
  const navigate = useNavigate();
  const [step, setStep] = createSignal<Step>({ kind: "idle" });
  let plate!: HTMLDivElement;
  let canvas!: HTMLCanvasElement;
  let abort: AbortController | undefined;
  onCleanup(() => abort?.abort());

  async function read(file: File) {
    abort?.abort();
    const controller = new AbortController();
    abort = controller;
    setStep({ kind: "loading" });
    // The MediaPipe code (about 400 KB) and the reader load only now.
    const { ClipError, closeClip, cutJpeg, extractPose, openClip } = await import("../pose/extract");
    let video: HTMLVideoElement | undefined;
    try {
      video = await openClip(file);
      const { loadLandmarker } = await import("../pose/landmarker");
      model ??= loadLandmarker();
      const loaded = await model.catch((err: unknown) => {
        model = undefined; // try again with the next clip
        throw err;
      });

      // The live preview: the video frame being read, with the stick figure over it.
      plate.replaceChildren(video, canvas);
      plate.style.aspectRatio = `${video.videoWidth} / ${video.videoHeight}`;
      canvas.width = Math.min(960, video.videoWidth);
      canvas.height = Math.round((canvas.width * video.videoHeight) / video.videoWidth);
      const ctx = canvas.getContext("2d");
      const ink = inkFrom(plate);

      const { sequence } = await extractPose(
        video,
        loaded,
        (p) => {
          setStep({ kind: "reading", done: p.done, total: p.total });
          if (ctx) drawSkeleton(ctx, p.frame, ink);
        },
        controller.signal,
      );

      setStep({ kind: "saving", what: "numbers" });
      const keys = pickKeyFrames(sequence.frames, 3);
      const saved = await createAnalysis(sequence, keys, props.me.csrf_token);
      if (saved.key_frames.length > 0) {
        setStep({ kind: "saving", what: "pictures" });
        for (const upload of saved.key_frames) {
          // A failed picture does not stop the others; the result page shows it as not uploaded.
          await cutJpeg(video, upload.frame_index)
            .then((jpeg) => uploadKeyFrame(upload, jpeg))
            .catch(() => undefined);
        }
        setStep({ kind: "saving", what: "checking" });
        await checkKeyFrames(saved.id, props.me.csrf_token);
      }
      navigate(`/analyses/${saved.id}`);
    } catch (err) {
      if (controller.signal.aborted) {
        setStep({ kind: "idle" });
        return;
      }
      setStep({ kind: "failed", message: messageFor(err, ClipError, step().kind === "saving") });
    } finally {
      if (video) closeClip(video);
      if (abort === controller) abort = undefined;
    }
  }

  function picked(e: Event & { currentTarget: HTMLInputElement }) {
    const file = e.currentTarget.files?.[0];
    e.currentTarget.value = ""; // the same file can be picked again
    if (file) void read(file);
  }

  const busy = () => step().kind === "loading" || step().kind === "reading" || step().kind === "saving";

  return (
    <>
      <p class="intro">{m.newAnalysis.intro}</p>
      <Show when={!busy()}>
        <div class="pick">
          <label class="action action--file">
            <input type="file" accept="video/*" capture="environment" class="sr-only" onChange={picked} />
            {m.newAnalysis.record}
          </label>
          <label class="action action--file">
            <input type="file" accept="video/*" class="sr-only" onChange={picked} />
            {m.newAnalysis.choose}
          </label>
        </div>
      </Show>
      <p class="note">{m.newAnalysis.privacy}</p>

      <div class="plate plate--live" ref={plate} hidden={step().kind !== "reading" && step().kind !== "saving"}>
        <canvas ref={canvas} aria-hidden="true" />
      </div>

      <Switch>
        <Match when={step().kind === "loading"}>
          <p class="summary" role="status">
            {m.newAnalysis.loadingModel}
          </p>
        </Match>
        <Match when={step().kind === "reading" && (step() as Extract<Step, { kind: "reading" }>)}>
          {(s) => (
            <div class="progress">
              <progress max={s().total} value={s().done} aria-label={m.newAnalysis.readingLabel} />
              <p class="summary" role="status">
                {m.newAnalysis.reading(s().done, s().total)}
              </p>
            </div>
          )}
        </Match>
        <Match when={step().kind === "saving" && (step() as Extract<Step, { kind: "saving" }>)}>
          {(s) => (
            <p class="summary" role="status">
              {s().what === "numbers"
                ? m.newAnalysis.saving
                : s().what === "pictures"
                  ? m.newAnalysis.uploading
                  : m.newAnalysis.checking}
            </p>
          )}
        </Match>
        <Match when={step().kind === "failed" && (step() as Extract<Step, { kind: "failed" }>)}>
          {(s) => (
            <p class="summary summary--fault" role="alert">
              {s().message}
            </p>
          )}
        </Match>
      </Switch>
      <Show when={step().kind === "reading"}>
        <button type="button" class="action" onClick={() => abort?.abort()}>
          {m.newAnalysis.cancel}
        </button>
      </Show>
    </>
  );
}

function messageFor(err: unknown, ClipError: typeof import("../pose/extract").ClipError, saving: boolean): string {
  const e = m.newAnalysis.errors;
  if (err instanceof ClipError) {
    switch (err.code) {
      case "too_short":
        return e.too_short(err.seconds);
      case "too_long":
        return e.too_long(err.seconds);
      default:
        return e[err.code];
    }
  }
  if (err instanceof AnalysisError) {
    if (err.status === 429) return e.daily;
    if (err.status === 422 && err.reason) return err.reason; // the server's plain-English reason
    return e.save;
  }
  return saving ? e.save : e.model;
}
