self.addEventListener('install', function (event) {
  event.waitUntil(
    caches.open('puretty-v2').then(function (cache) {
      return cache.addAll([
        '/xterm.js',
        '/xterm.css',
        '/addon-fit.js',
        '/app.js',
        '/manifest.json'
      ]);
    })
  );
});

self.addEventListener('activate', function (event) {
  event.waitUntil(
    caches.keys().then(function (names) {
      return Promise.all(
        names.filter(function (n) { return n !== 'puretty-v2'; })
             .map(function (n) { return caches.delete(n); })
      );
    })
  );
});

self.addEventListener('fetch', function (event) {
  var url = new URL(event.request.url);

  // Never cache API endpoints or the root path.
  if (url.pathname === '/' || url.pathname === '/input' || url.pathname === '/output' || url.pathname === '/resize') {
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
