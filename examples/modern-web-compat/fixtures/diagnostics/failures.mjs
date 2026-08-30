import("./missing.chunk.mjs").then(function () {
  document.getElementById("chunk-state").textContent = "unexpected chunk success";
}, function () {
  document.getElementById("chunk-state").textContent = "chunk failure isolated";
  console.error("dynamic chunk load failed");
});

document.getElementById("hydration-error").addEventListener("click", function () {
  throw new Error("[component:diagnostic-root] hydration mismatch isolated");
});

document.getElementById("unsupported-global-error").addEventListener("click", function () {
  try {
    navigator.frameworkUnsupportedAPI();
  } catch (_) {
    console.error("[component:diagnostic-root] unsupported global navigator.frameworkUnsupportedAPI");
  }
});

document.getElementById("event-error").addEventListener("click", function () {
  throw new Error("[component:diagnostic-root] Event dispatch failure isolated");
});

document.getElementById("observer-error").addEventListener("click", function () {
  console.error("observer loop limit isolated");
});
