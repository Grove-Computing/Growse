import { hydrate } from "../nodes/app.mjs";
import { application, entrypoint, framework } from "../upstream-contract.mjs";

const root = document.getElementById("svelte");
root.setAttribute("data-module-bootstrap", "loaded");
root.setAttribute("data-framework-build", framework);
root.setAttribute("data-upstream-entrypoint", entrypoint);
root.setAttribute("data-upstream-application", application);
hydrate(root);
