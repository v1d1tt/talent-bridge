import type { JSX } from "@solidjs/web";
import { createContext, createMemo, createSignal, useContext, type Accessor } from "solid-js";
import { fetchMe, type Me, type SessionState } from "./me";

/** Who is signed in, shared by the header and the pages. */
export type Session = {
  /** An async memo: read it under <Loading>. */
  state: Accessor<SessionState>;
  /** The signed-in user, or undefined when signed out. Async like `state`. */
  me: Accessor<Me | undefined>;
  /** Asks the server again, after sign-out or a role choice. */
  refresh: () => void;
};

const SessionContext = createContext<Session>();

export function SessionProvider(props: { children?: JSX.Element }) {
  const [attempt, setAttempt] = createSignal(0);
  // Same pattern as the status sheet: the memo re-runs fetchMe when `attempt` changes.
  const state = createMemo(() => {
    attempt();
    return fetchMe();
  });
  const me = () => {
    const s = state();
    return s.kind === "signed-in" ? s.me : undefined;
  };
  return (
    <SessionContext value={{ state, me, refresh: () => setAttempt((n) => n + 1) }}>{props.children}</SessionContext>
  );
}

export function useSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error("useSession needs a <SessionProvider>");
  return s;
}
