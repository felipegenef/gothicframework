(function() {
    window.__gothic_dev = true;

    let src = window.gothicframework_reloadSrc
        || new EventSource("/_gothicframework/reload/events");

    src.onmessage = async function(event) {
        if (!event || event.data !== "reload") return;

        // Close SSE before navigating so the server-side slot is freed immediately.
        src.close();
        window.gothicframework_reloadSrc = null;

        try {
            const res = await fetch(window.location.href, { cache: "no-store" });
            const html = await res.text();

            // Clear WASM globals so new modules start from a clean registry.
            delete window.__gothic_registry;
            delete window.__gothic_proxied;
            delete window.__gothicCurrentModule;

            // Replace the entire document with the freshly fetched HTML.
            // document.open/write/close forces the browser to re-request all
            // linked resources (CSS, JS) as if it were a brand-new page load.
            document.open("text/html", "replace");
            document.write(html);
            document.close();
        } catch(e) {
            // Fallback if fetch fails (server still restarting).
            window.location.reload();
        }
    };

    window.gothicframework_reloadSrc = src;
    window.onbeforeunload = function() { src.close(); };
})();
