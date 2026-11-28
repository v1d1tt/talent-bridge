import type { JSX } from "@solidjs/web";
import { createMemo, createSignal, Errored, For, Loading, Match, onCleanup, Show, Switch } from "solid-js";
import { AnalysisError } from "../analyses";
import { type Me } from "../me";
import { messages as m } from "../messages";
import { Plate } from "../pose/Plate";
import { drawSkeleton, inkFrom } from "../pose/skeleton";
import type { PoseSequence } from "../pose/types";
import {
  createReference,
  getShotGates,
  listReferences,
  sourceLink,
  type Gate,
  type Handedness,
  type Reference,
  type View,
} from "../references";
import { useSession } from "../session";

const t = m.references;

/** Shows its children only to the admin; everyone else gets a plain reason. */
export function AdminOnly(props: { children: (me: Me) => JSX.Element }) {
  const session = useSession();
  return (
    <Loading fallback={<p class="summary" role="status" />}>
      <Show
        when={session.me()}
        fallback={
          <p class="intro">
            {t.signedOut} <a href="/signin">{m.nav.signIn}</a>
          </p>
        }
      >
        {(me) => (
          <Show when={me().user.role === "admin"} fallback={<p class="intro">{t.notAdmin}</p>}>
            {props.children(me())}
          </Show>
        )}
      </Show>
    </Loading>
  );
}

/** /admin/references: the reference register, as a page of the scorer's book. */
export function References() {
  return (
    <section aria-labelledby="references-heading">
      <h1 id="references-heading" class="page__title">
        {t.heading}
      </h1>
      <AdminOnly>{(me) => <Register me={me} />}</AdminOnly>
    </section>
  );
}

function Register(props: { me: Me }) {
  // Saving a reference bumps the version, so the gates and the register load again.
  const [version, setVersion] = createSignal(0);
  const gates = createMemo(() => {
    version();
    return getShotGates();
  });
  const refs = createMemo(() => {
    version();
    return listReferences();
  });
  const retry = (reset: () => void) => () => (setVersion((n) => n + 1), reset());

  return (
    <>
      <p class="intro">{t.intro}</p>
      <div class="register">
        <section class="register__add" aria-labelledby="add-heading">
          <h2 id="add-heading" class="sheet__title">
            {t.addHeading}
          </h2>
          <AddReference
            me={props.me}
            gates={() => gates().gates}
            onSaved={() => setVersion((n) => n + 1)}
          />
        </section>

        <section class="register__gates" aria-labelledby="gates-heading">
          <h2 id="gates-heading" class="sheet__title">
            {t.gatesHeading}
          </h2>
          <Errored fallback={(_, reset) => <Failed retry={retry(reset)} />}>
            <Loading fallback={<p class="summary" role="status">{t.loading}</p>}>
              <p class="note">{t.gatesIntro(gates().min_references)}</p>
              <GateTable gates={gates().gates} min={gates().min_references} />
            </Loading>
          </Errored>
        </section>
      </div>

      <section aria-labelledby="register-heading">
        <h2 id="register-heading" class="sheet__title sheet__title--next">
          {t.registerHeading}
        </h2>
        <Errored fallback={(_, reset) => <Failed retry={retry(reset)} />}>
          <Loading fallback={<p class="summary" role="status">{t.loading}</p>}>
            <RegisterTable refs={refs()} />
          </Loading>
        </Errored>
      </section>
    </>
  );
}

function Failed(props: { retry: () => void }) {
  return (
    <>
      <p class="summary summary--fault" role="alert">
        {t.failed}
      </p>
      <button type="button" class="action" onClick={() => props.retry()}>
        {t.retry}
      </button>
    </>
  );
}

type GateRow = { shot: string; name: string; discipline: Gate["discipline"]; views: Record<View, Gate | undefined> };

/** Groups the gates into one row per shot, in the server's order. */
function rows(gates: Gate[]): GateRow[] {
  const out = new Map<string, GateRow>();
  for (const g of gates) {
    const row = out.get(g.shot) ?? { shot: g.shot, name: g.shot_name, discipline: g.discipline, views: { side_on: undefined, front_on: undefined } };
    row.views[g.view] = g;
    out.set(g.shot, row);
  }
  return [...out.values()];
}

function GateTable(props: { gates: Gate[]; min: number }) {
  const groups = () => (["bowling", "batting"] as const).map((d) => ({ d, rows: rows(props.gates).filter((r) => r.discipline === d) }));
  return (
    <div class="scroll-x">
      <table class="ruled gates">
        <thead>
          <tr>
            <th scope="col">{t.shot}</th>
            <th scope="col">{t.views.side_on}</th>
            <th scope="col">{t.views.front_on}</th>
          </tr>
        </thead>
        <For each={groups()}>
          {(g) => (
            <tbody>
              <tr>
                <th scope="rowgroup" colspan={3} class="gates__group">
                  {t.disciplines[g.d]}
                </th>
              </tr>
              <For each={g.rows}>
                {(r) => (
                  <tr>
                    <th scope="row">{r.name}</th>
                    <GateCell gate={r.views.side_on} min={props.min} />
                    <GateCell gate={r.views.front_on} min={props.min} />
                  </tr>
                )}
              </For>
            </tbody>
          )}
        </For>
      </table>
    </div>
  );
}

function GateCell(props: { gate: Gate | undefined; min: number }) {
  const n = () => props.gate?.references ?? 0;
  return (
    <td class="gate">
      <span class="gate__count">{t.count(n())}</span>
      <Show
        when={n() >= props.min}
        fallback={<span class="gate__needs">{t.needs(props.min)}</span>}
      >
        <span class="gate__ready">
          <span class="mark mark--ok" aria-hidden="true">
            ·
          </span>
          {props.gate?.enabled ? t.on : t.ready}
        </span>
      </Show>
      <Show when={props.gate && props.gate.legacy_references > 0}>
        <span class="gate__legacy">{t.legacy(props.gate!.legacy_references)}</span>
      </Show>
    </td>
  );
}

function RegisterTable(props: { refs: Reference[] }) {
  const c = t.columns;
  return (
    <Show when={props.refs.length > 0} fallback={<p class="note">{t.empty}</p>}>
      <div class="scroll-x">
        <table class="ruled register-table">
          <thead>
            <tr>
              <th scope="col">{c.player}</th>
              <th scope="col">{c.shot}</th>
              <th scope="col">{c.view}</th>
              <th scope="col">{c.hand}</th>
              <th scope="col" class="num">
                {c.frames}
              </th>
              <th scope="col">{c.source}</th>
              <th scope="col">
                <span class="sr-only">{t.figure}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            <For each={props.refs}>
              {(r) => (
                <tr>
                  <th scope="row">{r.player}</th>
                  <td>{r.shot_name}</td>
                  <td>{t.views[r.view]}</td>
                  <td>{t.hands[r.handedness]}</td>
                  <td class="num">{r.frame_count}</td>
                  <td class="register-table__source">
                    <Show when={r.origin === "legacy_csv"}>
                      <span class="tag">{t.oldCsv}</span>{" "}
                    </Show>
                    <Source text={r.source} />
                  </td>
                  <td>
                    <a href={`/admin/references/${r.id}`} aria-label={t.figureFor(r.player, r.shot_name)}>
                      {t.figure}
                    </a>
                  </td>
                </tr>
              )}
            </For>
          </tbody>
        </table>
      </div>
    </Show>
  );
}

/** A source is a link only when it is an http(s) URL; any other text stays text. */
export function Source(props: { text: string }) {
  return (
    <Show when={sourceLink(props.text)} fallback={<span>{props.text}</span>}>
      {(href) => (
        <a href={href()} rel="noopener noreferrer nofollow" target="_blank">
          {props.text}
        </a>
      )}
    </Show>
  );
}

/** The steps of adding one reference. The page shows exactly what the step says. */
type Step =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "reading"; done: number; total: number }
  | { kind: "read"; seq: PoseSequence }
  | { kind: "saving"; seq: PoseSequence }
  | { kind: "saved"; id: string; player: string }
  | { kind: "failed"; message: string; seq?: PoseSequence };

// The pose model is loaded once per page visit and reused.
let model: Promise<import("../pose/landmarker").Loaded> | undefined;

function AddReference(props: { me: Me; gates: () => Gate[]; onSaved: () => void }) {
  const [step, setStep] = createSignal<Step>({ kind: "idle" });
  let form!: HTMLFormElement;
  let plate!: HTMLDivElement;
  let canvas!: HTMLCanvasElement;
  let abort: AbortController | undefined;
  onCleanup(() => abort?.abort());

  const seqOf = (s: Step) => ("seq" in s ? s.seq : undefined);
  const busy = () => ["loading", "reading", "saving"].includes(step().kind);

  async function read(file: File) {
    abort?.abort();
    const controller = new AbortController();
    abort = controller;
    setStep({ kind: "loading" });
    const { ClipError, closeClip, extractPose, openClip } = await import("../pose/extract");
    let video: HTMLVideoElement | undefined;
    try {
      video = await openClip(file);
      const { loadLandmarker } = await import("../pose/landmarker");
      model ??= loadLandmarker();
      const loaded = await model.catch((err: unknown) => {
        model = undefined;
        throw err;
      });
      // Only the stick figure is drawn; the pro clip itself is never shown.
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
      setStep({ kind: "read", seq: sequence });
    } catch (err) {
      if (controller.signal.aborted) {
        setStep({ kind: "idle" });
        return;
      }
      setStep({ kind: "failed", message: clipMessage(err, ClipError) });
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

  async function save(e: SubmitEvent & { currentTarget: HTMLFormElement }) {
    e.preventDefault();
    const seq = seqOf(step());
    if (!seq) {
      setStep({ kind: "failed", message: t.needClip });
      return;
    }
    // Read the tags from the form itself at submit time (Solid 2 applies
    // signal writes on the next tick; the form holds the current values).
    const f = new FormData(e.currentTarget);
    const tags = {
      player: String(f.get("player") ?? ""),
      shot: String(f.get("shot") ?? ""),
      view: String(f.get("view") ?? "") as View,
      handedness: String(f.get("handedness") ?? "") as Handedness,
      source: String(f.get("source") ?? ""),
    };
    if (!tags.shot) {
      setStep({ kind: "failed", message: t.needShot, seq });
      return;
    }
    setStep({ kind: "saving", seq });
    try {
      const { id } = await createReference({ ...tags, pose_sequence: seq }, props.me.csrf_token);
      form.reset();
      setStep({ kind: "saved", id, player: tags.player.trim() });
      props.onSaved();
    } catch (err) {
      const message =
        err instanceof AnalysisError && err.status === 422 && err.reason ? err.reason : t.failedSave;
      setStep({ kind: "failed", message, seq });
    }
  }

  const shots = () => {
    const seen = new Map<string, Gate>();
    for (const g of props.gates()) if (!seen.has(g.shot)) seen.set(g.shot, g);
    return [...seen.values()];
  };

  return (
    <form ref={form} class="entry" onSubmit={save}>
      <label class="entry__field">
        <span class="entry__label">{t.player}</span>
        <input name="player" type="text" required maxlength={80} autocomplete="off" />
        <span class="entry__hint">{t.playerHint}</span>
      </label>

      <label class="entry__field">
        <span class="entry__label">{t.shot}</span>
        <Errored fallback={<select name="shot" required disabled />}>
          <Loading fallback={<select name="shot" required disabled />}>
          <select name="shot" required>
            <option value="">{t.shotPick}</option>
            <For each={["bowling", "batting"] as const}>
              {(d) => (
                <optgroup label={t.disciplines[d]}>
                  <For each={shots().filter((x) => x.discipline === d)}>
                    {(x) => <option value={x.shot}>{x.shot_name}</option>}
                  </For>
                </optgroup>
              )}
            </For>
          </select>
          </Loading>
        </Errored>
      </label>

      <fieldset class="entry__field entry__choices">
        <legend class="entry__label">{t.view}</legend>
        <For each={["side_on", "front_on"] as const}>
          {(v) => (
            <label class="entry__choice">
              <input type="radio" name="view" value={v} required />
              {t.views[v]}
            </label>
          )}
        </For>
      </fieldset>

      <fieldset class="entry__field entry__choices">
        <legend class="entry__label">{t.handedness}</legend>
        <For each={["right", "left"] as const}>
          {(h) => (
            <label class="entry__choice">
              <input type="radio" name="handedness" value={h} required />
              {t.hands[h]}
            </label>
          )}
        </For>
        <span class="entry__hint">{t.handHint}</span>
      </fieldset>

      <label class="entry__field">
        <span class="entry__label">{t.source}</span>
        <input name="source" type="text" required maxlength={500} autocomplete="off" />
        <span class="entry__hint">{t.sourceHint}</span>
      </label>

      <div class="entry__field">
        <span class="entry__label" id="clip-label">
          {t.clip}
        </span>
        <Show when={!busy()}>
          <label class="action action--file">
            <input type="file" accept="video/*" class="sr-only" aria-labelledby="clip-label" onChange={picked} />
            {t.clip}
          </label>
        </Show>
        <span class="entry__hint">{t.clipHint}</span>
      </div>

      <div class="plate plate--live" ref={plate} hidden={step().kind !== "reading"}>
        <canvas ref={canvas} aria-hidden="true" />
      </div>
      <Show when={seqOf(step())}>
        {(seq) => <Plate seq={seq()} start={Math.floor(seq().frames.length / 2)} caption={m.reference.plate} />}
      </Show>

      <Switch>
        <Match when={step().kind === "loading"}>
          <p class="summary" role="status">
            {t.loadingModel}
          </p>
        </Match>
        <Match when={step().kind === "reading" && (step() as Extract<Step, { kind: "reading" }>)}>
          {(s) => (
            <div class="progress">
              <progress max={s().total} value={s().done} aria-label={t.readingLabel} />
              <p class="summary" role="status">
                {t.reading(s().done, s().total)}
              </p>
              <button type="button" class="action" onClick={() => abort?.abort()}>
                {t.cancel}
              </button>
            </div>
          )}
        </Match>
        <Match when={step().kind === "read" && (step() as Extract<Step, { kind: "read" }>)}>
          {(s) => (
            <p class="summary" role="status">
              {t.read(s().seq.frames.length, (s().seq.frames.length / s().seq.fps).toFixed(2))}
            </p>
          )}
        </Match>
        <Match when={step().kind === "saving"}>
          <p class="summary" role="status">
            {t.saving}
          </p>
        </Match>
        <Match when={step().kind === "saved" && (step() as Extract<Step, { kind: "saved" }>)}>
          {(s) => (
            <p class="summary" role="status">
              {t.saved(s().player)} <a href={`/admin/references/${s().id}`}>{t.showIt}</a>
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

      <button type="submit" class="action" disabled={busy()} aria-busy={step().kind === "saving" ? "true" : "false"}>
        {step().kind === "saving" ? t.saving : t.save}
      </button>
    </form>
  );
}

function clipMessage(err: unknown, ClipError: typeof import("../pose/extract").ClipError): string {
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
  return e.model;
}
