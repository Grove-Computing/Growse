const toggle = document.getElementById("table-toggle");
const table = document.getElementById("layout-table");
const status = document.getElementById("table-status");
let compact = false;

toggle.addEventListener("click", () => {
  compact = !compact;
  table.setAttribute("class", compact ? "showcase-table collapsed auto" : "showcase-table");
  status.textContent = compact ? "COLLAPSED · AUTO" : "SEPARATE · FIXED";
});
