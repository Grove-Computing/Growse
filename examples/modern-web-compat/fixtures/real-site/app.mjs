const root = document.getElementById("real-site-root");
if (root && root.getAttribute("data-ssr-token") === "real-site-v1") {
  root.setAttribute("data-hydrated", "true");
  document.getElementById("real-site-hydration").textContent = "hydrated";
  const medium = document.getElementById("real-site-medium");
  const state = document.getElementById("real-site-state");
  document.getElementById("real-site-high").addEventListener("click", function () {
    medium.setAttribute("hidden", "");
    state.textContent = "1件";
  });
  document.getElementById("real-site-all").addEventListener("click", function () {
    medium.removeAttribute("hidden");
    state.textContent = "2件";
  });
}
