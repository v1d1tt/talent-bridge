import { createMemo, createSignal, Errored, isPending, Loading, Show } from "solid-js";
import { fetchHealth, type HealthReport } from "../health";
import { messages as m } from "../messages";

/** Settings page. For now it holds only the system status sheet. */
export function Settings() {
  return (
    <>
      <h1 class="page__title">{m.settings.heading}</h1>
      <StatusSheet />
    </>
  );
}

/** The health check, drawn as one ruled sheet of the scorer's book. */
function StatusSheet() {
  const [attempt, setAttempt] = createSignal(0);
  // An async memo (Solid 2): it re-runs fetchHealth whenever `attempt` changes.
  const report = createMemo(() => {
    attempt();
    return fetchHealth();
  });

  return (
    <section class="sheet" aria-labelledby="status-heading">
      <h2 id="status-heading" class="sheet__title">
        {m.status.heading}
      </h2>
      <Errored
        fallback={(_error, reset) => (
          <>
            <Rows serverDown />
            <p class="summary summary--fault" role="status">
              {m.status.serverDownHelp}
            </p>
            <button type="button" class="action" onClick={() => reset()}>
              {m.status.checkAgain}
            </button>
          </>
        )}
      >
        <Loading
          fallback={
            <>
              <Rows />
              <p class="summary" role="status">
                {m.status.checking} {m.status.coldStart}
              </p>
            </>
          }
        >
          <Rows report={report()} />
          <p class={{ summary: true, "summary--fault": report().database.status === "down" }} role="status">
            {report().database.status === "up" ? m.status.allGood : m.status.dbDownHelp}
          </p>
          <button
            type="button"
            class="action"
            aria-busy={isPending(report) ? "true" : "false"}
            onClick={() => setAttempt((n) => n + 1)}
          >
            {isPending(report) ? m.status.checking : m.status.checkAgain}
          </button>
        </Loading>
      </Errored>
    </section>
  );
}

/**
 * The ruled rows. Marks follow the book: an empty cell = not measured yet,
 * a struck-through cell = could not be measured, ✗ = fault.
 */
function Rows(props: { report?: HealthReport; serverDown?: boolean }) {
  const dbUp = () => props.report?.database.status === "up";
  return (
    <table class="ruled">
      <tbody>
        <tr>
          <th scope="row">{m.status.server}</th>
          <td>
            <Show when={props.report}>
              <Mark kind="ok" />
              <span>{m.status.serverUp}</span>
            </Show>
            <Show when={props.serverDown}>
              <Mark kind="fault" />
              <span>{m.status.serverDown}</span>
            </Show>
          </td>
          <td />
        </tr>
        <tr>
          <th scope="row">{m.status.database}</th>
          <td>
            <Show when={props.report}>
              <Mark kind={dbUp() ? "ok" : "fault"} />
              <span>{dbUp() ? m.status.dbUp : m.status.dbDown}</span>
            </Show>
            <Show when={props.serverDown}>
              <span class="struck">{m.status.notMeasured}</span>
            </Show>
          </td>
          <td class="num">
            <Show when={dbUp()}>{m.status.latency(props.report!.database.latency_ms)}</Show>
          </td>
        </tr>
        <tr>
          <th scope="row">{m.status.schema}</th>
          <td>
            <Show when={dbUp()}>
              <span>{m.status.migration(props.report!.database.schema_version)}</span>
            </Show>
            <Show when={(props.report && !dbUp()) || props.serverDown}>
              <span class="struck">{m.status.notMeasured}</span>
            </Show>
          </td>
          <td />
        </tr>
        <tr>
          <th scope="row">{m.status.build}</th>
          <td>
            <Show when={props.report}>
              <code>{props.report!.version}</code>
            </Show>
          </td>
          <td />
        </tr>
      </tbody>
    </table>
  );
}

function Mark(props: { kind: "ok" | "fault" }) {
  return (
    <span class={`mark mark--${props.kind}`} aria-hidden="true">
      {props.kind === "ok" ? "●" : "✗"}
    </span>
  );
}
