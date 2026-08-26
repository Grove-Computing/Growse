self.addEventListener("install", function () { self.skipWaiting(); });
self.addEventListener("activate", function () { clients.claim(); });
self.addEventListener("fetch", function (event) {
  if (event.request.url.endsWith("/app/offline/message")) {
    event.respondWith(new Response("offline response from Service Worker"));
  }
});
