import { createRouter, type RouterHistory } from "@solidjs/router";
import type { JSX } from "@solidjs/web";
import { messages as m } from "./messages";
import { Home, NotFound } from "./pages/Home";
import { Settings } from "./pages/Settings";

const routes = [
  { path: "/", component: Home },
  { path: "/settings", component: Settings },
  { path: "*404", component: NotFound },
] as const;

/** Builds the router. Tests pass a memory history; the browser uses the default. */
export function makeRouter(history?: RouterHistory) {
  return createRouter({ routes, history });
}

const BrowserRouter = makeRouter();

export function App(props: { router?: ReturnType<typeof makeRouter> }) {
  const Router = props.router ?? BrowserRouter;
  return <Router>{(route) => <Layout>{route.children}</Layout>}</Router>;
}

function Layout(props: { children?: JSX.Element }) {
  return (
    <>
      <header class="masthead">
        <a class="masthead__name" href="/">
          {m.appName}
        </a>
        <p class="masthead__tagline">{m.tagline}</p>
        <nav class="masthead__nav" aria-label={m.nav.label}>
          <a href="/settings">{m.nav.settings}</a>
        </nav>
      </header>
      <main class="page">{props.children}</main>
    </>
  );
}
