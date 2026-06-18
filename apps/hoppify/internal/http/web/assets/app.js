const { useCallback, useEffect, useMemo, useRef, useState } = React;
const h = React.createElement;
const PAGE_SIZE = 30;

function App() {
  const [captures, setCaptures] = useState([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [status, setStatus] = useState("idle");
  const [error, setError] = useState("");
  const loadingRef = useRef(false);
  const sentinelRef = useRef(null);

  const loadMore = useCallback(async () => {
    if (loadingRef.current || !hasMore) {
      return;
    }

    loadingRef.current = true;
    setStatus(captures.length === 0 ? "loading" : "loading-more");
    setError("");

    try {
      const response = await fetch(`/api/v1/captures?limit=${PAGE_SIZE}&offset=${offset}`, {
        headers: { Accept: "application/json" },
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const page = await response.json();
      const items = Array.isArray(page.captures) ? page.captures : [];
      setCaptures((current) => current.concat(items));
      setHasMore(Boolean(page.hasMore));
      setOffset(page.hasMore ? page.nextOffset : offset + items.length);
      setStatus("ready");
    } catch (err) {
      setStatus("error");
      setError(err instanceof Error ? err.message : "Failed to load captures");
    } finally {
      loadingRef.current = false;
    }
  }, [captures.length, hasMore, offset]);

  useEffect(() => {
    loadMore();
  }, []);

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel) {
      return undefined;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          loadMore();
        }
      },
      { rootMargin: "900px 0px" },
    );
    observer.observe(sentinel);

    return () => observer.disconnect();
  }, [loadMore]);

  const totalLabel = useMemo(() => {
    if (captures.length === 0) {
      return "No captures";
    }
    if (captures.length === 1) {
      return "1 capture";
    }

    return `${captures.length} captures`;
  }, [captures.length]);

  return h(
    "div",
    { className: "app-shell" },
    h(Sidebar),
    h(
      "div",
      { className: "workspace" },
      h(Header, { totalLabel }),
      h(
        "main",
        { className: "content", "aria-label": "Captures" },
        h(CaptureGallery, {
          captures,
          error,
          hasMore,
          onRetry: loadMore,
          sentinelRef,
          status,
        }),
      ),
      h(Footer),
    ),
  );
}

function Sidebar() {
  return h(
    "aside",
    { className: "sidebar", "aria-label": "Main navigation" },
    h("div", { className: "brand" }, h("span", { className: "brand-mark" }, "H"), h("span", null, "Hoppify")),
    h(
      "nav",
      { className: "nav-list" },
      h(
        "a",
        { className: "nav-item active", href: "/" },
        h("span", { className: "nav-icon", "aria-hidden": "true" }),
        h("span", null, "Captures"),
      ),
    ),
  );
}

function Header({ totalLabel }) {
  return h(
    "header",
    { className: "topbar" },
    h(
      "div",
      null,
      h("p", { className: "section-kicker" }, "Gallery"),
      h("h1", null, "Captures"),
    ),
    h("div", { className: "counter", "aria-label": "Loaded captures" }, totalLabel),
  );
}

function CaptureGallery({ captures, error, hasMore, onRetry, sentinelRef, status }) {
  if (status === "loading" && captures.length === 0) {
    return h("div", { className: "state" }, "Loading captures...");
  }

  if (status === "error" && captures.length === 0) {
    return h(
      "div",
      { className: "state error-state" },
      h("span", null, error || "Failed to load captures"),
      h("button", { type: "button", onClick: onRetry }, "Retry"),
    );
  }

  if (captures.length === 0) {
    return h("div", { className: "state" }, "No captures yet");
  }

  return h(
    React.Fragment,
    null,
    h(
      "section",
      { className: "gallery-grid" },
      captures.map((capture) => h(CaptureCard, { capture, key: capture.uuid })),
    ),
    h(
      "div",
      { className: "scroll-status", ref: sentinelRef },
      status === "loading-more" ? "Loading more..." : hasMore ? "" : "End of captures",
    ),
    status === "error"
      ? h(
          "div",
          { className: "inline-error" },
          h("span", null, error || "Failed to load captures"),
          h("button", { type: "button", onClick: onRetry }, "Retry"),
        )
      : null,
  );
}

function CaptureCard({ capture }) {
  const dimensions = capture.width && capture.height ? `${capture.width}x${capture.height}` : "";
  const size = formatBytes(capture.sizeBytes);
  const capturedAt = formatDate(capture.createdAt);
  const title = capturedAt === "Unknown" ? "Capture" : capturedAt;

  return h(
    "article",
    { className: "capture-card" },
    h(
      "div",
      { className: "thumb" },
      h("img", {
        alt: "Capture image",
        loading: "lazy",
        src: capture.imageUrl,
      }),
    ),
    h(
      "div",
      { className: "card-body" },
      h("h2", { title }, title),
      h(
        "dl",
        { className: "metadata" },
        dimensions ? h("div", null, h("dt", null, "Size"), h("dd", null, dimensions)) : null,
        size ? h("div", null, h("dt", null, "File"), h("dd", null, size)) : null,
      ),
    ),
  );
}

function Footer() {
  return h(
    "footer",
    { className: "footer" },
    h("span", null, "Hoppify"),
    h("a", { href: "/swagger" }, "API docs"),
  );
}

function formatDate(value) {
  if (!value) {
    return "Unknown";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "Unknown";
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(date);
}

function formatBytes(value) {
  if (!Number.isFinite(value) || value <= 0) {
    return "";
  }

  const units = ["B", "KB", "MB", "GB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }

  return `${size.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

ReactDOM.createRoot(document.getElementById("root")).render(h(App));
