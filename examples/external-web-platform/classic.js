(function () {
  "use strict";
  document.getElementById("classic-state").textContent = "external classic loaded";
  var target = document.getElementById("mutation-target");
  target.classList.add("mutated");
  document.getElementById("mutation-state").innerHTML = "<strong>mutated by external JavaScript</strong>";
}());
