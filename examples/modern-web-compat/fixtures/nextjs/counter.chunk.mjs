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

  marker.textContent = "hydrated";
  root.setAttribute("data-hydrated", "true");

  counter.addEventListener("click", function () {
    count += 1;
    output.textContent = String(count);
    root.setAttribute("data-state-revision", String(count));
  });

  navigation.addEventListener("click", function (event) {
    event.preventDefault();
    history.pushState({ framework: "nextjs", route: "about" }, "", "/next/about");
    document.getElementById("next-route").textContent = "/next/about";
  });
}
