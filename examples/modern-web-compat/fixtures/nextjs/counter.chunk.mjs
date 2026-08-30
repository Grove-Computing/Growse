export function hydrate(root) {
  if (root !== document.getElementById("__next")) {
    throw new Error("Next.js hydration replaced the SSR root");
  }
  if (root.getAttribute("data-ssr-token") !== "next-ssr-root-v1") {
    throw new Error("Next.js hydration lost SSR state");
  }

  let count = 0;
  const marker = document.getElementById("next-hydration-marker");
  const output = document.getElementById("next-count");
  const counter = document.getElementById("next-counter");
  const navigation = document.getElementById("next-navigation");
  const dialogToggle = document.getElementById("next-dialog-toggle");
  const dialog = document.getElementById("next-dialog");
  const dialogState = document.getElementById("next-dialog-state");
  const menuToggle = document.getElementById("next-menu-toggle");
  const menu = document.getElementById("next-menu");

  marker.textContent = "hydrated";
  root.setAttribute("data-hydrated", "true");

  counter.addEventListener("click", function () {
    count += 1;
    output.textContent = String(count);
    root.setAttribute("data-state-revision", String(count));
    root.classList.add("interactive");
    output.style.setProperty("color", "rgb(37, 99, 235)");
  });

  dialogToggle.addEventListener("click", function () {
    dialog.removeAttribute("hidden");
    dialog.classList.add("open");
    dialog.style.setProperty("opacity", "1");
    dialog.focus();
    dialogState.textContent = document.activeElement === dialog ? "open:focused" : "open";
  });

  menuToggle.addEventListener("click", function () {
    const expanded = menu.hasAttribute("hidden");
    menu.toggleAttribute("hidden", !expanded);
    menuToggle.setAttribute("aria-expanded", String(expanded));
  });

  navigation.addEventListener("click", function (event) {
    event.preventDefault();
    history.pushState({ framework: "nextjs", route: "about" }, "", "/next/about");
    document.getElementById("next-route").textContent = "/next/about";
  });

  document.getElementById("next-history-back").addEventListener("click", function () {
    history.back();
  });
  addEventListener("popstate", function () {
    document.getElementById("next-route").textContent = location.pathname;
  });
}
