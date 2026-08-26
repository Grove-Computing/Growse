import { moduleMessage } from "./dependency.mjs";

document.getElementById("module-state").textContent = moduleMessage;

import("./dynamic.mjs").then(function (module) {
  document.getElementById("dynamic-state").textContent = module.dynamicMessage;
});

WebAssembly.instantiateStreaming(fetch("/answer.wasm")).then(function (result) {
  document.getElementById("wasm-state").textContent = "WASM answer=" + result.instance.exports.answer();
});

navigator.serviceWorker.register("/app/sw.js").then(function () {
  document.getElementById("service-worker-state").textContent = "registered and active";
  return fetch("/app/offline/message");
}).then(function (response) {
  return response.text();
}).then(function (message) {
  document.getElementById("offline-state").textContent = message;
});
