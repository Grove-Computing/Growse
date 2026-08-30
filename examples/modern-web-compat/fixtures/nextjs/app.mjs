import { buildId, entrypoint, framework } from "./upstream-contract.mjs";

const root = document.getElementById("__next");
root.setAttribute("data-bootstrap", "loaded");
root.setAttribute("data-build-id", buildId);
root.setAttribute("data-framework-build", framework);
root.setAttribute("data-upstream-entrypoint", entrypoint);

const chunk = await import("./counter.chunk.mjs");
chunk.hydrate(root);
