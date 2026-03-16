// InfoKeep Service Worker — network-first, no offline caching needed
// (app is server-rendered and requires an active connection)

const CACHE_NAME = 'infokeep-v2';

// Cache only static assets on install
const STATIC_ASSETS = [
    '/static/css/bulma.min.css',
];

self.addEventListener('install', (event) => {
    event.waitUntil(
        caches.open(CACHE_NAME).then((cache) => cache.addAll(STATIC_ASSETS)).catch(() => { })
    );
    self.skipWaiting();
});

self.addEventListener('activate', (event) => {
    event.waitUntil(
        caches.keys().then((keys) =>
            Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
        )
    );
    self.clients.claim();
});

// Network-first: try the network, fall back to cache for static assets only
self.addEventListener('fetch', (event) => {
    if (event.request.method !== 'GET') return;

    event.respondWith(
        fetch(event.request)
            .then((response) => {
                // Cache successful static asset responses
                if (response.ok && event.request.url.includes('/static/')) {
                    const clone = response.clone();
                    caches.open(CACHE_NAME).then((cache) => cache.put(event.request, clone));
                }
                return response;
            })
            .catch(() => caches.match(event.request))
    );
});

// Listen for Web Push events from the server and display a native notification
self.addEventListener('push', function (event) {
    if (!event.data) {
        return;
    }

    try {
        const data = event.data.json();
        const title = data.title || "InfoKeep Reminder";
        const options = {
            body: data.body || "You have a new reminder.",
            vibrate: [200, 100, 200]
        };

        event.waitUntil(
            self.registration.showNotification(title, options)
        );
    } catch (e) {
        console.error("Failed to parse push data", e);
    }
});

// Handle clicking the notification (focus/open app)
self.addEventListener('notificationclick', function (event) {
    event.notification.close();
    event.waitUntil(
        clients.matchAll({ type: 'window', includeUncontrolled: true }).then(function (clientList) {
            for (let client of clientList) {
                if (client.url.includes('/reminders') && 'focus' in client) {
                    return client.focus();
                }
            }
            if (clients.openWindow) {
                return clients.openWindow('/reminders');
            }
        })
    );
});
