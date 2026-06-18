const { useCallback, useEffect, useMemo, useRef, useState } = React;
const h = React.createElement;
const PAGE_SIZE = 30;

const SECTIONS = {
  captures: {
    key: "captures",
    label: "Captures",
    title: "Captures",
    counterLabel: "Loaded captures",
    endpoint: "/api/v1/captures",
    responseKey: "captures",
    emptyText: "No captures yet",
    endText: "End of captures",
    itemAlt: "Capture image",
    loadingText: "Loading captures...",
    loadingMoreText: "Loading more captures...",
    singular: "capture",
    plural: "captures",
    errorText: "Failed to load captures",
    ariaLabel: "Captures",
  },
  crops: {
    key: "crops",
    label: "Crops",
    title: "Crops",
    counterLabel: "Loaded crops",
    endpoint: "/api/v1/crops",
    responseKey: "crops",
    emptyText: "No crops yet",
    endText: "End of crops",
    itemAlt: "Crop image",
    loadingText: "Loading crops...",
    loadingMoreText: "Loading more crops...",
    singular: "crop",
    plural: "crops",
    errorText: "Failed to load crops",
    ariaLabel: "Crops",
    gridClass: "crop-grid",
    cardClass: "crop-card",
  },
  recognitions: {
    key: "recognitions",
    label: "Recognitions",
    title: "Recognitions",
    kicker: "Model results",
    counterLabel: "Loaded recognitions",
    endpoint: "/api/v1/beer-labels/recognitions",
    responseKey: "recognitions",
    emptyText: "No recognitions yet",
    endText: "End of recognitions",
    loadingText: "Loading recognitions...",
    loadingMoreText: "Loading more recognitions...",
    singular: "recognition",
    plural: "recognitions",
    errorText: "Failed to load recognitions",
    ariaLabel: "Recognitions",
    view: "recognitions",
  },
};

function App() {
  const [activeSection, setActiveSection] = useState(sectionFromHash);
  const section = SECTIONS[activeSection] || SECTIONS.captures;

  useEffect(() => {
    const onHashChange = () => setActiveSection(sectionFromHash());
    window.addEventListener("hashchange", onHashChange);

    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  const handleNavigate = useCallback((sectionKey) => {
    if (!SECTIONS[sectionKey]) {
      return;
    }

    setActiveSection(sectionKey);
  }, []);

  return h(
    "div",
    { className: "app-shell" },
    h(Sidebar, { activeSection, onNavigate: handleNavigate }),
    h(GalleryWorkspace, { section }),
  );
}

function GalleryWorkspace({ section }) {
  const [items, setItems] = useState([]);
  const [offset, setOffset] = useState(0);
  const [hasMore, setHasMore] = useState(true);
  const [status, setStatus] = useState("idle");
  const [error, setError] = useState("");
  const [selectedItem, setSelectedItem] = useState(null);
  const loadingRef = useRef(false);
  const requestRef = useRef(0);
  const sentinelRef = useRef(null);

  const loadPage = useCallback(
    async (targetOffset, append) => {
      if (loadingRef.current) {
        return;
      }
      if (append && !hasMore) {
        return;
      }

      loadingRef.current = true;
      const requestID = requestRef.current + 1;
      requestRef.current = requestID;
      setStatus(append ? "loading-more" : "loading");
      setError("");

      try {
        const response = await fetch(`${section.endpoint}?limit=${PAGE_SIZE}&offset=${targetOffset}`, {
          headers: { Accept: "application/json" },
        });
        if (!response.ok) {
          throw new Error(`HTTP ${response.status}`);
        }

        const page = await response.json();
        const pageItems = Array.isArray(page[section.responseKey]) ? page[section.responseKey] : [];
        if (requestID !== requestRef.current) {
          return;
        }
        setItems((current) => (append ? current.concat(pageItems) : pageItems));
        setHasMore(Boolean(page.hasMore));
        const fallbackOffset = targetOffset + pageItems.length;
        const nextOffset = Number.isFinite(page.nextOffset) ? page.nextOffset : fallbackOffset;
        setOffset(page.hasMore ? nextOffset : fallbackOffset);
        setStatus("ready");
      } catch (err) {
        if (requestID !== requestRef.current) {
          return;
        }
        setStatus("error");
        setError(err instanceof Error ? err.message : section.errorText);
      } finally {
        if (requestID === requestRef.current) {
          loadingRef.current = false;
        }
      }
    },
    [hasMore, section],
  );

  const loadFirstPage = useCallback(() => {
    requestRef.current += 1;
    loadingRef.current = false;
    setItems([]);
    setOffset(0);
    setHasMore(true);
    setStatus("idle");
    setError("");
    loadPage(0, false);
  }, [loadPage]);

  const loadMore = useCallback(() => {
    loadPage(offset, true);
  }, [loadPage, offset]);

  useEffect(() => {
    loadFirstPage();
    setSelectedItem(null);
  }, [section.key]);

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

  const totalLabel = useMemo(() => formatTotal(items.length, section), [items.length, section]);
  const previewItem = section.key === "crops" ? selectedItem : null;
  const handlePreview = useCallback(
    (item) => {
      if (section.key === "crops") {
        setSelectedItem(item);
      }
    },
    [section.key],
  );
  const handleClosePreview = useCallback(() => setSelectedItem(null), []);

  return h(
    "div",
    { className: "workspace" },
    h(Header, { section, totalLabel }),
    h(
      "main",
      {
        className: previewItem ? "content content-with-preview" : "content",
        "aria-label": section.ariaLabel,
      },
      h(
        "div",
        { className: "content-primary" },
        section.view === "recognitions"
          ? h(RecognitionTable, {
              error,
              hasMore,
              items,
              onLoadMore: loadMore,
              onRetry: items.length === 0 ? loadFirstPage : loadMore,
              section,
              sentinelRef,
              status,
            })
          : h(Gallery, {
              error,
              hasMore,
              items,
              onLoadMore: loadMore,
              onPreview: handlePreview,
              onRetry: items.length === 0 ? loadFirstPage : loadMore,
              previewItem,
              section,
              sentinelRef,
              status,
            }),
      ),
      previewItem ? h(CropPreviewPanel, { item: previewItem, onClose: handleClosePreview }) : null,
    ),
    h(Footer),
  );
}

function Sidebar({ activeSection, onNavigate }) {
  return h(
    "aside",
    { className: "sidebar", "aria-label": "Main navigation" },
    h("div", { className: "brand" }, h("span", { className: "brand-mark" }, "H"), h("span", null, "Hoppify")),
    h(
      "nav",
      { className: "nav-list" },
      Object.values(SECTIONS).map((section) =>
        h(
          "a",
          {
            className: activeSection === section.key ? "nav-item active" : "nav-item",
            href: `#${section.key}`,
            key: section.key,
            onClick: () => onNavigate(section.key),
          },
          h("span", { className: `nav-icon ${section.key}-icon`, "aria-hidden": "true" }),
          h("span", null, section.label),
        ),
      ),
    ),
  );
}

function Header({ section, totalLabel }) {
  return h(
    "header",
    { className: "topbar" },
    h(
      "div",
      null,
      h("p", { className: "section-kicker" }, section.kicker || "Gallery"),
      h("h1", null, section.title),
    ),
    h("div", { className: "counter", "aria-label": section.counterLabel }, totalLabel),
  );
}

function Gallery({
  error,
  hasMore,
  items,
  onLoadMore,
  onPreview,
  onRetry,
  previewItem,
  section,
  sentinelRef,
  status,
}) {
  if (status === "loading" && items.length === 0) {
    return h("div", { className: "state" }, section.loadingText);
  }

  if (status === "error" && items.length === 0) {
    return h(
      "div",
      { className: "state error-state" },
      h("span", null, error || section.errorText),
      h("button", { type: "button", onClick: onRetry }, "Retry"),
    );
  }

  if (items.length === 0) {
    return h("div", { className: "state" }, section.emptyText);
  }

  return h(
    React.Fragment,
    null,
    h(
      "section",
      { className: section.gridClass ? `gallery-grid ${section.gridClass}` : "gallery-grid" },
      items.map((item) =>
        h(GalleryCard, {
          active: previewItem?.uuid === item.uuid,
          item,
          key: item.uuid,
          onPreview,
          section,
        }),
      ),
    ),
    h(
      "div",
      { className: "scroll-status", ref: sentinelRef },
      status === "loading-more"
        ? section.loadingMoreText
        : hasMore
          ? h("button", { type: "button", onClick: onLoadMore }, "Load more")
          : section.endText,
    ),
    status === "error"
      ? h(
          "div",
          { className: "inline-error" },
          h("span", null, error || section.errorText),
          h("button", { type: "button", onClick: onRetry }, "Retry"),
        )
      : null,
  );
}

function RecognitionTable({ error, hasMore, items, onLoadMore, onRetry, section, sentinelRef, status }) {
  if (status === "loading" && items.length === 0) {
    return h("div", { className: "state" }, section.loadingText);
  }

  if (status === "error" && items.length === 0) {
    return h(
      "div",
      { className: "state error-state" },
      h("span", null, error || section.errorText),
      h("button", { type: "button", onClick: onRetry }, "Retry"),
    );
  }

  if (items.length === 0) {
    return h("div", { className: "state" }, section.emptyText);
  }

  return h(
    React.Fragment,
    null,
    h(
      "section",
      { className: "recognitions-panel" },
      h(
        "div",
        { className: "recognitions-table-wrap" },
        h(
          "table",
          { className: "recognitions-table" },
          h(
            "thead",
            null,
            h(
              "tr",
              null,
              h("th", { scope: "col" }, "Crop"),
              h("th", { scope: "col" }, "Name"),
              h("th", { scope: "col" }, "Brewery"),
              h("th", { scope: "col" }, "Style"),
              h("th", { scope: "col" }, "Country"),
              h("th", { scope: "col" }, "ABV"),
              h("th", { scope: "col" }, "Confidence"),
              h("th", { scope: "col" }, "Status"),
              h("th", { scope: "col" }, "Model"),
              h("th", { scope: "col" }, "Recognized"),
            ),
          ),
          h(
            "tbody",
            null,
            items.map((item) => h(RecognitionRow, { item, key: `${item.uuid}:${item.promptVersion}` })),
          ),
        ),
      ),
    ),
    h(
      "div",
      { className: "scroll-status", ref: sentinelRef },
      status === "loading-more"
        ? section.loadingMoreText
        : hasMore
          ? h("button", { type: "button", onClick: onLoadMore }, "Load more")
          : section.endText,
    ),
    status === "error"
      ? h(
          "div",
          { className: "inline-error" },
          h("span", null, error || section.errorText),
          h("button", { type: "button", onClick: onRetry }, "Retry"),
        )
      : null,
  );
}

function RecognitionRow({ item }) {
  const result = item.result || {};
  const crop = item.crop || {};
  const title = [optionalText(result.beerName), optionalText(result.brewery)].filter((value) => value !== "-").join(" - ");
  const cropTitle = title || crop.uuid || item.uuid || "Recognition crop";

  return h(
    "tr",
    null,
    h(
      "td",
      { className: "crop-cell" },
      h(
        "div",
        { className: "recognition-crop" },
        crop.imageUrl
          ? h("img", {
              alt: cropTitle,
              loading: "lazy",
              src: crop.imageUrl,
            })
          : h("span", null, "-"),
      ),
    ),
    h("td", { className: "strong-cell" }, optionalText(result.beerName)),
    h("td", null, optionalText(result.brewery)),
    h("td", null, optionalText(result.style)),
    h("td", null, optionalText(result.country)),
    h("td", { className: "numeric-cell" }, formatABV(result.abv)),
    h("td", { className: "numeric-cell" }, formatConfidence(result.confidence)),
    h(
      "td",
      null,
      h("span", { className: `status-pill ${statusClass(result.status)}` }, statusLabel(result.status)),
    ),
    h("td", { className: "model-cell", title: item.model || "" }, optionalText(item.model)),
    h("td", { className: "date-cell" }, formatDate(item.createdAt)),
  );
}

function GalleryCard({ active, item, onPreview, section }) {
  const dimensions = item.width && item.height ? `${item.width}x${item.height}` : "";
  const size = formatBytes(item.sizeBytes);
  const capturedAt = formatDate(item.createdAt);
  const title = capturedAt === "Unknown" ? section.singularTitle || section.singular : capturedAt;
  const cardClass = section.cardClass ? `capture-card ${section.cardClass}` : "capture-card";
  const clickable = typeof onPreview === "function" && section.key === "crops";
  const fullCardClass = [cardClass, clickable ? "clickable-card" : "", active ? "active-card" : ""]
    .filter(Boolean)
    .join(" ");
  const cardProps = {
    className: fullCardClass,
  };
  if (clickable) {
    cardProps.type = "button";
    cardProps.onClick = () => onPreview(item);
    cardProps["aria-label"] = `Open crop preview ${dimensions || item.uuid}`;
  }

  return h(
    clickable ? "button" : "article",
    cardProps,
    h(
      "div",
      { className: "thumb" },
      h("img", {
        alt: section.itemAlt,
        loading: "lazy",
        src: item.imageUrl,
      }),
    ),
    h(
      "div",
      { className: "card-body" },
      h("h2", { title }, title),
      h(
        "dl",
        { className: "metadata" },
        dimensions ? h("div", null, h("dt", null, "Pixels"), h("dd", null, dimensions)) : null,
        size ? h("div", null, h("dt", null, "File"), h("dd", null, size)) : null,
      ),
    ),
  );
}

function CropPreviewPanel({ item, onClose }) {
  const dimensions = item.width && item.height ? `${item.width}x${item.height}` : "-";
  const size = formatBytes(item.sizeBytes) || "-";

  return h(
    "aside",
    { className: "preview-panel", "aria-label": "Crop preview" },
    h(
      "div",
      { className: "preview-header" },
      h(
        "div",
        null,
        h("p", { className: "section-kicker" }, "Preview"),
        h("h2", null, "Crop"),
      ),
      h(
        "button",
        { className: "icon-button", type: "button", onClick: onClose, "aria-label": "Close preview" },
        "X",
      ),
    ),
    h(
      "div",
      { className: "preview-image" },
      h("img", {
        alt: "Selected crop",
        src: item.imageUrl,
      }),
    ),
    h(
      "dl",
      { className: "metadata preview-metadata" },
      h("div", null, h("dt", null, "Pixels"), h("dd", null, dimensions)),
      h("div", null, h("dt", null, "File"), h("dd", null, size)),
      h("div", null, h("dt", null, "Created"), h("dd", null, formatDate(item.createdAt))),
      h("div", null, h("dt", null, "UUID"), h("dd", { title: item.uuid }, shortUUID(item.uuid))),
    ),
    h(
      "a",
      { className: "preview-link", href: item.imageUrl, target: "_blank", rel: "noreferrer" },
      "Open image",
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

function sectionFromHash() {
  const key = window.location.hash.replace(/^#/, "").trim();

  return SECTIONS[key] ? key : "captures";
}

function formatTotal(count, section) {
  if (count === 0) {
    return `No ${section.plural}`;
  }
  if (count === 1) {
    return `1 ${section.singular}`;
  }

  return `${count} ${section.plural}`;
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

function shortUUID(value) {
  if (typeof value !== "string" || value.length <= 12) {
    return value || "-";
  }

  return `${value.slice(0, 8)}...${value.slice(-4)}`;
}

function optionalText(value) {
  if (typeof value !== "string") {
    return "-";
  }

  const trimmed = value.trim();

  return trimmed || "-";
}

function formatABV(value) {
  if (!Number.isFinite(value) || value <= 0) {
    return "-";
  }

  return `${value.toFixed(value % 1 === 0 ? 0 : 1)}%`;
}

function formatConfidence(value) {
  if (!Number.isFinite(value)) {
    return "-";
  }

  return `${Math.round(value * 100)}%`;
}

function statusLabel(status) {
  const labels = {
    identified: "Identified",
    uncertain: "Uncertain",
    unreadable: "Unreadable",
    not_beer: "Not beer",
  };

  return labels[status] || optionalText(status);
}

function statusClass(status) {
  if (status === "identified") {
    return "status-identified";
  }
  if (status === "uncertain") {
    return "status-uncertain";
  }
  if (status === "not_beer") {
    return "status-not-beer";
  }

  return "status-muted";
}

ReactDOM.createRoot(document.getElementById("root")).render(h(App));
