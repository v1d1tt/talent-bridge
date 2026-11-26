import { For } from "solid-js";
import { messages as m } from "../messages";

/** Home page: what Talent-Bridge does, and what this build already does. */
export function Home() {
  return (
    <>
      <h1 class="page__title">{m.home.heading}</h1>
      <p class="intro">{m.home.intro}</p>
      <h2 class="sheet__title">{m.home.stepsHeading}</h2>
      <ol class="ruled-list">
        <For each={m.home.steps}>{(step) => <li>{step}</li>}</For>
      </ol>
      <p class="note">{m.home.buildNote}</p>
    </>
  );
}

export function NotFound() {
  return (
    <>
      <h1 class="page__title">{m.notFound.heading}</h1>
      <p>
        <a href="/">{m.notFound.back}</a>
      </p>
    </>
  );
}
