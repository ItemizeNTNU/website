/*
 * Tombstone.
 *
 * The previous site registered a service worker at this path that cached its
 * application shell. Those registrations live on in returning visitors'
 * browsers and would keep serving them the old site indefinitely, so this
 * replacement tears itself down: it takes over immediately, empties every
 * cache, unregisters, and reloads any open tabs onto the real site.
 *
 * Keep serving this for as long as anyone might still have the old worker
 * installed. Deleting the route brings the stale shell back.
 */

self.addEventListener("install", function () {
  self.skipWaiting();
});

self.addEventListener("activate", function (event) {
  event.waitUntil(
    (async function () {
      const names = await caches.keys();
      await Promise.all(names.map((name) => caches.delete(name)));
      await self.registration.unregister();

      const clients = await self.clients.matchAll({ type: "window" });
      clients.forEach((client) => client.navigate(client.url));
    })(),
  );
});
