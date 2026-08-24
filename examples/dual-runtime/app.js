(function () {
  "use strict";
  var count = 0;
  function text(id, value) {
    var element = document.getElementById(id);
    if (element) { element.textContent = value; }
  }
  text("engine", "javascript");
  console.info("dual-runtime engine=javascript");
  document.getElementById("increment").addEventListener("click", function () {
    count += 1;
    text("count", String(count));
  });
  var note = document.getElementById("note");
  var saved = localStorage.getItem("dual-note");
  if (saved !== null) {
    note.value = saved;
    text("storage", saved);
  }
  note.addEventListener("input", function (event) {
    localStorage.setItem("dual-note", event.value);
    text("storage", event.value);
  });
  setTimeout(function () { text("timer", "javascript timer fired"); }, 10);
  fetch("/api/message").then(function (response) { return response.text(); }).then(function (message) {
    text("fetch-success", message);
  }, function (error) { text("fetch-success", error.message); });
  fetch("/api/failure").then(function (response) {
    text("fetch-failure", "HTTP " + response.status);
  }, function (error) { text("fetch-failure", error.message); });
  document.getElementById("route").addEventListener("click", function () {
    history.pushState({engine: "javascript"}, "", "?view=javascript#history");
    text("location", location.href);
  });
  document.getElementById("runtime-error").addEventListener("click", function () {
    throw new Error("intentional JavaScript showcase error");
  });
}());
