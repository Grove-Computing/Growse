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
}
