export function hydrate(root) {
  if (root !== document.getElementById("svelte")) {
    throw new Error("SvelteKit hydration replaced the SSR root");
  }
  if (root.getAttribute("data-ssr-token") !== "svelte-ssr-root-v1") {
    throw new Error("SvelteKit hydration lost SSR state");
  }

  root.setAttribute("data-svelte-h", "growse-v015");
  root.setAttribute("data-hydrated", "true");
  document.getElementById("svelte-hydration-marker").textContent = "hydrated";

  let revision = 0;
  document.getElementById("svelte-reactive").addEventListener("click", function () {
    revision += 1;
    document.getElementById("svelte-state").textContent = "reactive:" + String(revision);
    root.classList.add("interactive");
    document.getElementById("svelte-state").style.setProperty("color", "rgb(124, 58, 237)");
  });

  document.getElementById("svelte-form").addEventListener("submit", function (event) {
    event.preventDefault();
    const name = document.getElementById("svelte-name").value;
    document.getElementById("svelte-form-result").textContent = "enhanced:" + name;
  });

  document.getElementById("svelte-navigation").addEventListener("click", function (event) {
    event.preventDefault();
    history.pushState({ framework: "sveltekit", route: "about" }, "", "/svelte/about");
    document.getElementById("svelte-route").textContent = "/svelte/about";
  });

  const dialog = document.getElementById("svelte-dialog");
  document.getElementById("svelte-dialog-toggle").addEventListener("click", function () {
    dialog.removeAttribute("hidden");
    dialog.classList.add("open");
    dialog.style.setProperty("opacity", "1");
    dialog.focus();
    document.getElementById("svelte-dialog-state").textContent = document.activeElement === dialog ? "open:focused" : "open";
  });

  const menu = document.getElementById("svelte-menu");
  const menuToggle = document.getElementById("svelte-menu-toggle");
  menuToggle.addEventListener("click", function () {
    const expanded = menu.hasAttribute("hidden");
    menu.toggleAttribute("hidden", !expanded);
    menuToggle.setAttribute("aria-expanded", String(expanded));
  });

  addEventListener("popstate", function () {
    document.getElementById("svelte-route").textContent = location.pathname;
  });
}
