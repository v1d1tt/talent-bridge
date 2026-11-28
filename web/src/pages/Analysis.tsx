import { useParams } from "@solidjs/router";
import { createMemo, createSignal, Errored, For, Loading, Match, Show, Switch } from "solid-js";
import { AnalysisError, getAnalysis, type Analysis as AnalysisData, type KeyFrame } from "../analyses";
import { messages as m } from "../messages";
import { Plate } from "../pose/Plate";

const seconds = (frame: number, fps: number) => (frame / fps).toFixed(2);

/** /analyses/:id: one saved analysis, as a page of the scorer's book. */
export function Analysis() {
  const params = useParams<{ id: string }>();
  const [attempt, setAttempt] = createSignal(0);
  // An async memo: it re-runs when the id or `attempt` changes.
  const analysis = createMemo(() => {
    attempt();
    return getAnalysis(params.id ?? "");
  });

  return (
    <section aria-labelledby="analysis-heading">
      <h1 id="analysis-heading" class="page__title">
        {m.analysis.heading}
      </h1>
      <Errored
        fallback={(err, reset) => {
          const e = err();
          const status = e instanceof AnalysisError ? e.status : 0;
          return (
            <Switch fallback={<Failed retry={() => (setAttempt((n) => n + 1), reset())} />}>
              <Match when={status === 404}>
                <p class="intro">{m.analysis.notFound}</p>
              </Match>
              <Match when={status === 401}>
                <p class="intro">
                  {m.analysis.signedOut} <a href="/signin">{m.nav.signIn}</a>
                </p>
              </Match>
            </Switch>
          );
        }}
      >
        <Loading fallback={<p class="summary" role="status">{m.analysis.loading}</p>}>
          <Result a={analysis()} />
        </Loading>
      </Errored>
    </section>
  );
}

function Failed(props: { retry: () => void }) {
  return (
    <>
      <p class="summary summary--fault" role="alert">
        {m.analysis.failed}
      </p>
      <button type="button" class="action" onClick={() => props.retry()}>
        {m.analysis.retry}
      </button>
    </>
  );
}

function Result(props: { a: AnalysisData }) {
  const seq = () => props.a.pose_sequence;
  return (
    <div class="result">
      <div class="result__left">
        <div class="total">
          <span class="total__label">{m.analysis.score}</span>
          <span class="total__value" aria-label={m.analysis.scoreLater}>
            —
          </span>
          <p class="note">{m.analysis.scoreLater}</p>
        </div>
        <Plate
          seq={props.a.pose_sequence}
          start={props.a.key_frames[0]?.frame_index ?? Math.floor(props.a.pose_sequence.frames.length / 2)}
          caption={m.analysis.plate}
        />
      </div>

      <div class="result__right">
        <h2 class="sheet__title">{m.analysis.sheet}</h2>
        <table class="ruled">
          <tbody>
            <tr>
              <th scope="row">{m.analysis.frames}</th>
              <td class="num">{props.a.frame_count}</td>
            </tr>
            <tr>
              <th scope="row">{m.analysis.fps}</th>
              <td class="num">{m.analysis.fpsValue(props.a.fps)}</td>
            </tr>
            <tr>
              <th scope="row">{m.analysis.length}</th>
              <td class="num">{m.analysis.seconds(props.a.duration_s)}</td>
            </tr>
            <tr>
              <th scope="row">{m.analysis.size}</th>
              <td class="num">{m.analysis.sizeValue(seq().width, seq().height)}</td>
            </tr>
            <tr>
              <th scope="row">{m.analysis.saved}</th>
              <td class="num">
                <time datetime={props.a.created_at}>{new Date(props.a.created_at).toLocaleString("en-GB")}</time>
              </td>
            </tr>
          </tbody>
        </table>

        <h2 class="sheet__title sheet__title--next">{m.analysis.keyFrames}</h2>
        <Show when={props.a.key_frames_on} fallback={<p class="note">{m.analysis.keyFramesOff}</p>}>
          <ol class="stills">
            <For each={props.a.key_frames}>{(k) => <Still k={k} fps={props.a.fps} />}</For>
          </ol>
          <p class="note">{m.analysis.keyFramesHow}</p>
        </Show>
        <p>
          <a href="/analyses/new">{m.analysis.newOne}</a>
        </p>
      </div>
    </div>
  );
}

function Still(props: { k: KeyFrame; fps: number }) {
  const caption = () => m.analysis.keyFrameCaption(props.k.frame_index + 1, seconds(props.k.frame_index, props.fps));
  return (
    <li class="still">
      <Switch>
        <Match when={props.k.url}>
          {(url) => <img src={url()} alt={m.analysis.keyFrameAlt(props.k.frame_index + 1)} loading="lazy" />}
        </Match>
        <Match when={props.k.state === "rejected"}>
          <p class="still__empty struck">{m.analysis.keyFrameRejected}</p>
        </Match>
        <Match when={true}>
          <p class="still__empty struck">{m.analysis.keyFramePending}</p>
        </Match>
      </Switch>
      <span class="still__caption">{caption()}</span>
    </li>
  );
}
