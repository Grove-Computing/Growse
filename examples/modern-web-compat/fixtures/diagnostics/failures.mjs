import("./missing.chunk.mjs").then(function () {
  document.getElementById("chunk-state").textContent = "unexpected chunk success";
}, function () {
  document.getElementById("chunk-state").textContent = "chunk failure isolated";
  console.error("dynamic chunk load failed");
});

document.getElementById("hydration-error").addEventListener("click", function () {
  throw new Error("hydration exception isolated");
});

document.getElementById("observer-error").addEventListener("click", function () {
  console.error("observer loop limit isolated");
});
