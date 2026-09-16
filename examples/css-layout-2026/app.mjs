const toggle = document.getElementById("table-toggle");
const table = document.getElementById("layout-table");
const status = document.getElementById("table-status");
let compact = false;

toggle.addEventListener("click", () => {
  compact = !compact;
  table.setAttribute("class", compact ? "showcase-table collapsed auto" : "showcase-table");
  status.textContent = compact ? "COLLAPSED · AUTO" : "SEPARATE · FIXED";
});

const layoutToggle = document.getElementById("layout-toggle");
const playground = document.getElementById("layout-playground");
const layoutStatus = document.getElementById("layout-status");
let alternateLayout = false;

layoutToggle.addEventListener("click", () => {
  alternateLayout = !alternateLayout;
  playground.setAttribute("class", alternateLayout ? "layout-playground alternate" : "layout-playground");
  layoutStatus.textContent = alternateLayout ? "VERTICAL-LR · RTL · UNSAFE" : "VERTICAL-RL · SAFE";
});

const writingToggle = document.getElementById("writing-toggle");
const writingPlayground = document.getElementById("writing-playground");
const writingStatus = document.getElementById("writing-status");
let alternateWriting = false;

writingToggle.addEventListener("click", () => {
  alternateWriting = !alternateWriting;
  writingPlayground.setAttribute("class", alternateWriting ? "writing-playground alternate" : "writing-playground");
  writingStatus.textContent = alternateWriting ? "VERTICAL-LR · RTL" : "VERTICAL-RL · LTR";
});

const positionToggle = document.getElementById("position-toggle");
const positionPlayground = document.getElementById("position-playground");
const positionStatus = document.getElementById("position-status");
let alternatePosition = false;

positionToggle.addEventListener("click", () => {
  alternatePosition = !alternatePosition;
  positionPlayground.setAttribute("class", alternatePosition ? "position-playground alternate" : "position-playground");
  positionStatus.textContent = alternatePosition ? "SHIFTED · ROTATED · WIDER" : "NESTED SCROLL · TRANSFORMED";
});
