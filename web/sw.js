self.addEventListener('install', function (event) {
  event.waitUntil(
    caches.open('puretty-v1').then(function (cache) {
      return cache.addAll([
        '/',
        '/xterm.js',
        '/xterm.css',
        '/addon-fit.js',
        '/app.js',
        '/manifest.json'
      ]);
    })
  );
});

self.addEventListener('fetch', function (event) {
  var url = new URL(event.request.url);

  // Never cache API endpoints.
  if (url.pathname === '/input' || url.pathname === '/output' || url.pathname === '/resize') {
    event.respondWith(fetch(event.request));
    return;
  }

  // Cache-first with network fallback for static assets.
  event.respondWith(
    caches.match(event.request).then(function (cached) {
      return cached || fetch(event.request);
    })
  );
});
