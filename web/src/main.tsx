import "./styles.css";

import { render } from "@solidjs/web";
import { App } from "./App";

const root = document.getElementById("app");
if (!root) throw new Error("Missing #app");

render(() => <App />, root);
