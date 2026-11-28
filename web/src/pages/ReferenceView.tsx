import { useParams } from "@solidjs/router";
import { createMemo, createSignal, Errored, Loading, Match, Show, Switch } from "solid-js";
import { AnalysisError } from "../analyses";
import { messages as m } from "../messages";
import { Plate } from "../pose/Plate";
import { getReference, type ReferenceWithPose } from "../references";
import { AdminOnly, Source } from "./References";

const t = m.reference;

/** /admin/references/:id: one pro reference, drawn only as a stick figure. */
export function ReferenceView() {
  return (
    <section aria-labelledby="reference-heading">
      <AdminOnly>{() => <Loaded />}</AdminOnly>
    </section>
  );
}

function Loaded() {
  const params = useParams<{ id: string }>();
  const [attempt, setAttempt] = createSignal(0);
  const ref = createMemo(() => {
    attempt();
    return getReference(params.id ?? "");
  });
  return (
    <Errored
      fallback={(err, reset) => (
        <Switch
          fallback={
            <>
              <p class="summary summary--fault" role="alert">
                {m.references.failed}
              </p>
              <button type="button" class="action" onClick={() => (setAttempt((n) => n + 1), reset())}>
                {m.references.retry}
              </button>
            </>
          }
        >
          <Match when={err() instanceof AnalysisError && (err() as AnalysisError).status === 404}>
            <h1 id="reference-heading" class="page__title">
              {m.references.heading}
            </h1>
            <p class="intro">{t.notFound}</p>
            <p>
              <a href="/admin/references">{t.back}</a>
            </p>
          </Match>
        </Switch>
      )}
    >
      <Loading fallback={<p class="summary" role="status">{t.loading}</p>}>
        <Sheet r={ref()} />
      </Loading>
    </Errored>
  );
}

function Sheet(props: { r: ReferenceWithPose }) {
  const r = () => props.r;
  return (
    <>
      <h1 id="reference-heading" class="page__title">
        {r().player}
      </h1>
      <div class="result">
        <div class="result__left">
          <Plate seq={r().pose_sequence} start={Math.floor(r().pose_sequence.frames.length / 2)} caption={t.plate} />
        </div>
        <div class="result__right">
          <table class="ruled">
            <tbody>
              <tr>
                <th scope="row">{t.shot}</th>
                <td>{r().shot_name}</td>
              </tr>
              <tr>
                <th scope="row">{t.view}</th>
                <td>{m.references.views[r().view]}</td>
              </tr>
              <tr>
                <th scope="row">{t.hand}</th>
                <td>{m.references.hands[r().handedness]}</td>
              </tr>
              <tr>
                <th scope="row">{t.source}</th>
                <td class="register-table__source">
                  <Source text={r().source} />
                </td>
              </tr>
              <tr>
                <th scope="row">{t.frames}</th>
                <td class="num">
                  {r().frame_count} · {m.analysis.fpsValue(r().fps)}
                </td>
              </tr>
              <tr>
                <th scope="row">{t.length}</th>
                <td class="num">{m.analysis.seconds(r().duration_s)}</td>
              </tr>
              <tr>
                <th scope="row">{t.added}</th>
                <td class="num">
                  <time datetime={r().created_at}>{new Date(r().created_at).toLocaleDateString("en-GB")}</time>
                </td>
              </tr>
            </tbody>
          </table>
          <Show when={r().origin === "legacy_csv"}>
            <p class="note">
              <span class="tag">{m.references.oldCsv}</span> {t.legacyNote}
            </p>
          </Show>
          <p>
            <a href="/admin/references">{t.back}</a>
          </p>
        </div>
      </div>
    </>
  );
}
