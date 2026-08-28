const root = document.getElementById("__next");
root.setAttribute("data-bootstrap", "loaded");

const chunk = await import("./counter.chunk.mjs");
chunk.hydrate(root);
