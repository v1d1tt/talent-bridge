import { useNavigate } from "@solidjs/router";
import { createSignal, Loading, Show } from "solid-js";
import { chooseRole, RequestError, type Me } from "../me";
import { messages as m } from "../messages";
import { useSession } from "../session";

/** /welcome: the one-time role choice after the first sign-in. */
export function Welcome() {
  const session = useSession();
  return (
    <section class="sheet" aria-labelledby="welcome-heading">
      <h1 id="welcome-heading" class="page__title">
        {m.welcome.heading}
      </h1>
      <Loading fallback={<p class="summary" role="status" />}>
        <Show
          when={session.me()}
          fallback={
            <>
              <p class="intro">{m.welcome.signedOut}</p>
              <p>
                <a href="/signin">{m.nav.signIn}</a>
              </p>
            </>
          }
        >
          {(me) => (
            <Show when={me().user.role} fallback={<RoleForm me={me()} />}>
              {(role) => (
                <>
                  <p class="intro">{m.welcome.alreadyChosen(m.roles[role()])}</p>
                  <p>
                    <a href="/">{m.welcome.home}</a>
                  </p>
                </>
              )}
            </Show>
          )}
        </Show>
      </Loading>
    </section>
  );
}

function RoleForm(props: { me: Me }) {
  const session = useSession();
  const navigate = useNavigate();
  const [saving, setSaving] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  async function submit(e: SubmitEvent & { currentTarget: HTMLFormElement }) {
    e.preventDefault();
    // Read the checked radio from the form itself: plain HTML state, no signal.
    const picked = new FormData(e.currentTarget).get("role");
    const role = picked === "athlete" || picked === "coach" ? picked : null;
    if (!role) {
      setError(m.welcome.pickOne);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await chooseRole(role, props.me.csrf_token);
    } catch (err) {
      // 409: a role was set already (another tab); the refresh below shows it.
      if (!(err instanceof RequestError && err.status === 409)) {
        setError(m.welcome.failed);
        setSaving(false);
        return;
      }
    }
    session.refresh();
    navigate("/");
  }

  return (
    <form onSubmit={submit} novalidate>
      <p class="intro">{m.welcome.intro(props.me.user.display_name)}</p>
      <fieldset class="choices">
        <legend class="sr-only">{m.welcome.heading}</legend>
        <RoleOption value="athlete" label={m.welcome.athlete} hint={m.welcome.athleteHint} />
        <RoleOption value="coach" label={m.welcome.coach} hint={m.welcome.coachHint} />
      </fieldset>
      <p class="note">{m.welcome.final}</p>
      <p class={{ summary: true, "summary--fault": !!error() }} role="status">
        {error()}
      </p>
      <button type="submit" class="action" aria-busy={saving() ? "true" : "false"} disabled={saving()}>
        {saving() ? m.welcome.saving : m.welcome.continue}
      </button>
    </form>
  );
}

function RoleOption(props: { value: "athlete" | "coach"; label: string; hint: string }) {
  return (
    <label class="choice">
      <input type="radio" name="role" value={props.value} />
      <span class="choice__label">{props.label}</span>
      <span class="choice__hint">{props.hint}</span>
    </label>
  );
}
