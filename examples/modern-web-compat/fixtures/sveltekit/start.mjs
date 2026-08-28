import { hydrate } from "../nodes/app.mjs";

const root = document.getElementById("svelte");
root.setAttribute("data-module-bootstrap", "loaded");
hydrate(root);
