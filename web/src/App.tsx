import { createRouter, useNavigate, type RouterHistory } from "@solidjs/router";
import type { JSX } from "@solidjs/web";
import { createSignal, Loading, Show } from "solid-js";
import { signOut, type Me } from "./me";
import { messages as m } from "./messages";
import { Home, NotFound } from "./pages/Home";
import { Settings } from "./pages/Settings";
import { SignIn } from "./pages/SignIn";
import { Welcome } from "./pages/Welcome";
import { SessionProvider, useSession } from "./session";

const routes = [
  { path: "/", component: Home },
  { path: "/settings", component: Settings },
  { path: "/signin", component: SignIn },
  { path: "/welcome", component: Welcome },
  { path: "*404", component: NotFound },
] as const;

/** Builds the router. Tests pass a memory history; the browser uses the default. */
export function makeRouter(history?: RouterHistory) {
  return createRouter({ routes, history });
}

const BrowserRouter = makeRouter();

export function App(props: { router?: ReturnType<typeof makeRouter> }) {
  const Router = props.router ?? BrowserRouter;
  return (
    <SessionProvider>
      <Router>{(route) => <Layout>{route.children}</Layout>}</Router>
    </SessionProvider>
  );
}

function Layout(props: { children?: JSX.Element }) {
  const session = useSession();
  return (
    <>
      <header class="masthead">
        <a class="masthead__name" href="/">
          {m.appName}
        </a>
        <p class="masthead__tagline">{m.tagline}</p>
        <nav class="masthead__nav" aria-label={m.nav.label}>
          {/* The links that exist for every role today; later slices add per-role links. */}
          <Loading fallback={<a href="/settings">{m.nav.settings}</a>}>
            <Show
              when={session.me()}
              fallback={
                <>
                  <a href="/settings">{m.nav.settings}</a>
                  <a href="/signin">{m.nav.signIn}</a>
                </>
              }
            >
              {(me) => <SignedInNav me={me()} />}
            </Show>
          </Loading>
        </nav>
      </header>
      <main class="page">{props.children}</main>
    </>
  );
}

function SignedInNav(props: { me: Me }) {
  const session = useSession();
  const navigate = useNavigate();
  const [busy, setBusy] = createSignal(false);
  const [failed, setFailed] = createSignal(false);

  async function onSignOut() {
    setBusy(true);
    setFailed(false);
    try {
      await signOut(props.me.csrf_token);
    } catch {
      setFailed(true);
      setBusy(false);
      return;
    }
    session.refresh();
    navigate("/");
  }

  return (
    <>
      <span class="masthead__user">
        {props.me.user.display_name}
        <Show when={props.me.user.role}>{(role) => <span class="masthead__role">{m.roles[role()]}</span>}</Show>
      </span>
      <a href="/settings">{m.nav.settings}</a>
      <button type="button" class="link-button" aria-busy={busy() ? "true" : "false"} onClick={onSignOut}>
        {busy() ? m.nav.signingOut : m.nav.signOut}
      </button>
      <Show when={failed()}>
        <span class="masthead__error" role="alert">
          {m.nav.signOutFailed}
        </span>
      </Show>
    </>
  );
}
