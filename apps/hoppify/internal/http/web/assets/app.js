const { useCallback, useEffect, useMemo, useRef, useState } = React;
const h = React.createElement;
const PAGE_SIZE = 30;

const SECTIONS = {
  add: {
    key: "add",
    label: "Add",
    title: "Add",
    kicker: "Workspace",
    counterLabel: "Pipeline status",
    singular: "image",
    plural: "images",
    view: "add",
  },
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
    section.view === "add" ? h(AddWorkspace, { section }) : h(GalleryWorkspace, { section }),
  );
}

function AddWorkspace({ section }) {
  const [stage, setStage] = useState("idle");
  const [dragActive, setDragActive] = useState(false);
  const [fileName, setFileName] = useState("");
  const [previewURL, setPreviewURL] = useState("");
  const [capture, setCapture] = useState(null);
  const [detectedCount, setDetectedCount] = useState(null);
  const [rows, setRows] = useState([]);
  const [error, setError] = useState("");
  const fileInputRef = useRef(null);
  const runRef = useRef(0);
  const abortRef = useRef(null);

  const processedCount = useMemo(
    () => rows.filter((row) => row.status === "done" || row.status === "error").length,
    [rows],
  );
  const failedCount = useMemo(() => rows.filter((row) => row.status === "error").length, [rows]);
  const recognizedCount = useMemo(() => rows.filter((row) => row.status === "done").length, [rows]);
  const totalLabel = pipelineCounterLabel(stage, rows.length, processedCount);
  const busy = isProcessingStage(stage);

  const clearCurrentRun = useCallback(() => {
    runRef.current += 1;
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
  }, []);

  const resetPipeline = useCallback(() => {
    clearCurrentRun();
    setStage("idle");
    setDragActive(false);
    setFileName("");
    setPreviewURL((current) => {
      if (current) {
        URL.revokeObjectURL(current);
      }

      return "";
    });
    setCapture(null);
    setDetectedCount(null);
    setRows([]);
    setError("");
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  }, [clearCurrentRun]);

  useEffect(() => () => clearCurrentRun(), [clearCurrentRun]);

  useEffect(
    () => () => {
      if (previewURL) {
        URL.revokeObjectURL(previewURL);
      }
    },
    [previewURL],
  );

  const startPipeline = useCallback(
    async (file) => {
      if (!file) {
        return;
      }

      clearCurrentRun();
      const runID = runRef.current;
      const controller = new AbortController();
      abortRef.current = controller;

      setStage("uploading");
      setFileName(file.name || "image");
      setPreviewURL((current) => {
        if (current) {
          URL.revokeObjectURL(current);
        }

        return URL.createObjectURL(file);
      });
      setCapture(null);
      setDetectedCount(null);
      setRows([]);
      setError("");

      try {
        const nextCapture = await uploadCapture(file, controller.signal);
        if (runRef.current !== runID || controller.signal.aborted) {
          return;
        }
        setCapture(nextCapture);
        setStage("detecting");

        const detection = await detectCapture(nextCapture.uuid, controller.signal);
        if (runRef.current !== runID || controller.signal.aborted) {
          return;
        }

        const boxes = detectionBoxes(detection);
        setDetectedCount(boxes.length);
        if (boxes.length === 0) {
          setStage("complete");
          return;
        }

        setStage("cropping");
        const cropResponse = await createCrops(nextCapture.uuid, boxes, controller.signal);
        if (runRef.current !== runID || controller.signal.aborted) {
          return;
        }

        const cropRows = cropsToRows(cropResponse.crops).map((row) => ({ ...row, status: "running" }));
        setRows(cropRows);
        setStage("recognizing");

        const batchItems = [];
        await identifyCrops(
          cropRows.map((row) => row.uuid),
          controller.signal,
          (item) => {
            batchItems.push(item);
            setRows((current) => mergeBatchRecognition(current, item));
          },
        );
        if (runRef.current !== runID || controller.signal.aborted) {
          return;
        }
        const recognizedRows = mergeBatchRecognitions(cropRows, batchItems);
        const failures = recognizedRows.filter((row) => row.status === "error").length;
        setRows(recognizedRows);

        if (failures > 0) {
          setError(`${failures} recognition${failures === 1 ? "" : "s"} failed`);
          setStage("error");
        } else {
          setError("");
          setStage("complete");
        }
      } catch (err) {
        if (controller.signal.aborted || runRef.current !== runID) {
          return;
        }
        setError(friendlyError(err));
        setStage("error");
      } finally {
        if (runRef.current === runID) {
          abortRef.current = null;
        }
      }
    },
    [clearCurrentRun],
  );

  const handleFileInput = useCallback(
    (event) => {
      const file = event.currentTarget.files?.[0];
      startPipeline(file);
    },
    [startPipeline],
  );

  const handleDrop = useCallback(
    (event) => {
      event.preventDefault();
      setDragActive(false);
      if (busy) {
        return;
      }
      startPipeline(event.dataTransfer.files?.[0]);
    },
    [busy, startPipeline],
  );

  const handleDragOver = useCallback(
    (event) => {
      event.preventDefault();
      if (!busy) {
        setDragActive(true);
      }
    },
    [busy],
  );

  const handleDragLeave = useCallback((event) => {
    if (event.currentTarget === event.target) {
      setDragActive(false);
    }
  }, []);

  return h(
    "div",
    { className: "workspace" },
    h(Header, { section, totalLabel }),
    h(
      "main",
      { className: "content add-content", "aria-label": "Add image pipeline" },
      h(
        "section",
        { className: "add-panel" },
        h(
          "label",
          {
            className: [
              "upload-frame",
              dragActive ? "upload-frame-active" : "",
              previewURL ? "upload-frame-preview" : "",
              busy ? "upload-frame-busy" : "",
            ]
              .filter(Boolean)
              .join(" "),
            onDragLeave: handleDragLeave,
            onDragOver: handleDragOver,
            onDrop: handleDrop,
          },
          h("input", {
            accept: "image/*",
            disabled: busy,
            onChange: handleFileInput,
            ref: fileInputRef,
            type: "file",
          }),
          previewURL
            ? h("img", { alt: "Selected capture", className: "upload-preview", src: previewURL })
            : h(
                "div",
                { className: "upload-empty" },
                h("span", { className: "upload-plus", "aria-hidden": "true" }, "+"),
                h("span", null, "Add image"),
              ),
          busy
            ? h(
                "div",
                { className: "upload-overlay", role: "status" },
                h(Spinner, null),
                h("span", null, stageText(stage)),
              )
            : null,
        ),
        h(
          "div",
          { className: "pipeline-summary" },
          h(
            "div",
            null,
            h("p", { className: "summary-title" }, fileName || "Add image"),
            h("p", { className: "summary-detail" }, pipelineSummary(stage, capture, rows.length, recognizedCount, failedCount)),
          ),
          capture || rows.length > 0 || stage === "error" || stage === "complete"
            ? h("button", { className: "reset-button", type: "button", onClick: resetPipeline }, "New image")
            : null,
        ),
        detectedCount !== null ? h("p", { className: "detected-count" }, formatDetectedCount(detectedCount)) : null,
        error ? h("p", { className: "pipeline-error" }, error) : null,
      ),
      rows.length > 0
        ? h(PipelineProgress, {
            failedCount,
            processedCount,
            recognizedCount,
            totalCount: rows.length,
          })
        : null,
      rows.length > 0 ? h(PipelineTable, { rows }) : null,
    ),
    h(Footer),
  );
}

function PipelineProgress({ failedCount, processedCount, recognizedCount, totalCount }) {
  const percent = totalCount > 0 ? Math.round((processedCount / totalCount) * 100) : 0;

  return h(
    "section",
    { className: "pipeline-progress-panel", "aria-label": "Recognition progress" },
    h(
      "div",
      { className: "progress-header" },
      h("span", null, "Gemini progress"),
      h("span", null, `${processedCount}/${totalCount}`),
    ),
    h(
      "div",
      {
        "aria-valuemax": totalCount,
        "aria-valuemin": 0,
        "aria-valuenow": processedCount,
        className: "progress-track",
        role: "progressbar",
      },
      h("div", { className: "progress-bar", style: { width: `${percent}%` } }),
    ),
    h(
      "div",
      { className: "progress-meta" },
      h("span", null, `${recognizedCount} recognized`),
      failedCount > 0 ? h("span", { className: "progress-failed" }, `${failedCount} failed`) : null,
    ),
  );
}

function PipelineTable({ rows }) {
  return h(
    "section",
    { className: "pipeline-table-panel" },
    h(
      "div",
      { className: "pipeline-table-wrap" },
      h(
        "table",
        { className: "pipeline-table" },
        h(
          "thead",
          null,
          h(
            "tr",
            null,
            h("th", { scope: "col" }, "Crop"),
            h("th", { scope: "col" }, "Status"),
            h("th", { scope: "col" }, "Beer"),
            h("th", { scope: "col" }, "Brewery"),
            h("th", { scope: "col" }, "Style"),
            h("th", { scope: "col" }, "Country"),
            h("th", { scope: "col" }, "ABV"),
            h("th", { scope: "col" }, "Confidence"),
            h("th", { scope: "col" }, "Model"),
          ),
        ),
        h(
          "tbody",
          null,
          rows.map((row) => h(PipelineRow, { key: row.uuid, row })),
        ),
      ),
    ),
  );
}

function PipelineRow({ row }) {
  const recognition = row.recognition || {};
  const result = recognition.result || {};

  return h(
    "tr",
    null,
    h(
      "td",
      { className: "crop-cell" },
      h(
        "div",
        { className: "pipeline-crop" },
        h("img", {
          alt: `Crop ${row.index}`,
          loading: "lazy",
          src: row.imageUrl,
        }),
      ),
    ),
    h("td", null, h(RowStatus, { row, result })),
    h("td", { className: "strong-cell" }, optionalText(result.beerName)),
    h("td", null, optionalText(result.brewery)),
    h("td", null, optionalText(result.style)),
    h("td", null, optionalText(result.country)),
    h("td", { className: "numeric-cell" }, formatABV(result.abv)),
    h("td", { className: "numeric-cell" }, formatConfidence(result.confidence)),
    h("td", { className: "model-cell", title: recognition.model || "" }, optionalText(recognition.model)),
  );
}

function RowStatus({ row, result }) {
  if (row.status === "done") {
    return h(
      "span",
      { className: `status-pill ${statusClass(result.status)}` },
      `${statusLabel(result.status)}${row.cached ? " cached" : ""}`,
    );
  }
  if (row.status === "error") {
    return h("span", { className: "row-error", title: row.error || "Recognition failed" }, "Failed");
  }

  return h(
    "span",
    { className: "row-waiting" },
    h(Spinner, { small: true }),
    row.status === "running" ? "Gemini" : "Queued",
  );
}

function Spinner({ small }) {
  return h("span", { className: small ? "spinner spinner-small" : "spinner", "aria-hidden": "true" });
}

async function uploadCapture(file, signal) {
  const form = new FormData();
  form.append("files", file, file.name || "capture");

  const payload = await requestJSON("/api/v1/captures", {
    body: form,
    method: "POST",
    signal,
  });
  const captures = Array.isArray(payload.captures) ? payload.captures : [];
  if (!captures[0]?.uuid) {
    throw new Error("Capture response is empty");
  }

  return captures[0];
}

async function detectCapture(uuid, signal) {
  return requestJSON("/api/v1/detect", {
    body: JSON.stringify({ uuid }),
    headers: { "Content-Type": "application/json" },
    method: "POST",
    signal,
  });
}

async function createCrops(uuid, boxes, signal) {
  return requestJSON("/api/v1/crops", {
    body: JSON.stringify({ uuid, boxes }),
    headers: { "Content-Type": "application/json" },
    method: "POST",
    signal,
  });
}

async function identifyCrops(uuids, signal, onItem) {
  const response = await fetch("/api/v1/beer-labels/identify-batch", {
    body: JSON.stringify({ uuids }),
    headers: {
      Accept: "application/x-ndjson",
      "Content-Type": "application/json",
    },
    method: "POST",
    signal,
  });
  if (!response.ok) {
    let payload = null;
    try {
      payload = await response.json();
    } catch (err) {
      payload = null;
    }
    throw new Error(apiErrorMessage(payload, response.status));
  }
  if (!response.body?.getReader) {
    throw new Error("Streaming response is unavailable");
  }

  await readNDJSON(response.body, onItem);
}

async function readNDJSON(body, onItem) {
  const reader = body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";
    lines.forEach((line) => emitNDJSONLine(line, onItem));
  }

  buffer += decoder.decode();
  emitNDJSONLine(buffer, onItem);
}

function emitNDJSONLine(line, onItem) {
  const trimmed = line.trim();
  if (!trimmed) {
    return;
  }

  onItem(JSON.parse(trimmed));
}

async function requestJSON(url, options) {
  const { headers, ...rest } = options;
  const response = await fetch(url, {
    ...rest,
    headers: { Accept: "application/json", ...(headers || {}) },
  });
  let payload = null;
  try {
    payload = await response.json();
  } catch (err) {
    if (response.ok) {
      throw new Error("Response is not JSON");
    }
  }
  if (!response.ok) {
    throw new Error(apiErrorMessage(payload, response.status));
  }

  return payload;
}

function apiErrorMessage(payload, status) {
  const apiError = payload?.error;
  if (typeof apiError?.message === "string" && apiError.message.trim()) {
    return apiError.message.trim();
  }
  if (typeof apiError?.code === "string" && apiError.code.trim()) {
    return apiError.code.trim().replaceAll("_", " ");
  }

  return `HTTP ${status}`;
}

function detectionBoxes(payload) {
  const images = Array.isArray(payload?.images) ? payload.images : [];
  const boxes = [];

  images.forEach((image) => {
    const results = Array.isArray(image.results) ? image.results : [];
    results.forEach((detection) => {
      const box = detection.box || {};
      const bbox = [box.x1, box.y1, box.x2, box.y2].map((value) => Number(value));
      if (bbox.every(Number.isFinite) && bbox[2] > bbox[0] && bbox[3] > bbox[1]) {
        boxes.push({
          bbox,
          confidence: Number.isFinite(detection.confidence) ? detection.confidence : 0,
        });
      }
    });
  });

  return boxes;
}

function cropsToRows(crops) {
  return (Array.isArray(crops) ? crops : []).map((crop, index) => ({
    cached: false,
    error: "",
    imageUrl: `/api/v1/captures/${encodeURIComponent(crop.uuid)}/image`,
    index: index + 1,
    recognition: null,
    status: "pending",
    uuid: crop.uuid,
  }));
}

function mergeBatchRecognitions(rows, items) {
  const byUUID = new Map();
  items.forEach((item) => {
    if (typeof item.uuid === "string" && item.uuid.trim()) {
      byUUID.set(item.uuid, item);
    }
  });

  return rows.map((row) => {
    const item = byUUID.get(row.uuid);
    if (!item) {
      return {
        ...row,
        error: "Recognition response is missing",
        status: "error",
      };
    }
    if (item.error) {
      return {
        ...row,
        error: item.error.message || item.error.code || "Recognition failed",
        status: "error",
      };
    }

    return {
      ...row,
      cached: Boolean(item.recognition?.cached),
      error: "",
      recognition: item.recognition,
      status: "done",
    };
  });
}

function mergeBatchRecognition(rows, item) {
  return rows.map((row) => (row.uuid === item.uuid ? applyBatchRecognition(row, item) : row));
}

function applyBatchRecognition(row, item) {
  if (item.error) {
    return {
      ...row,
      error: item.error.message || item.error.code || "Recognition failed",
      status: "error",
    };
  }

  return {
    ...row,
    cached: Boolean(item.recognition?.cached),
    error: "",
    recognition: item.recognition,
    status: "done",
  };
}

function isProcessingStage(stage) {
  return ["uploading", "detecting", "cropping", "recognizing"].includes(stage);
}

function stageText(stage) {
  const labels = {
    uploading: "Saving capture",
    detecting: "Detecting beers",
    cropping: "Creating crops",
    recognizing: "Waiting for Gemini",
  };

  return labels[stage] || "Processing";
}

function pipelineCounterLabel(stage, totalCount, processedCount) {
  if (stage === "idle") {
    return "Ready";
  }
  if (stage === "recognizing" && totalCount > 0) {
    return `${processedCount}/${totalCount}`;
  }
  if (stage === "complete") {
    return totalCount > 0 ? "Complete" : "No detections";
  }
  if (stage === "error") {
    return "Needs attention";
  }

  return stageText(stage);
}

function pipelineSummary(stage, capture, totalCount, recognizedCount, failedCount) {
  if (stage === "idle") {
    return "Workspace is empty";
  }
  if (stage === "uploading") {
    return "Saving capture";
  }
  if (stage === "detecting") {
    return capture?.uuid ? `Capture ${shortUUID(capture.uuid)}` : "Detecting beers";
  }
  if (stage === "cropping") {
    return "Creating crops";
  }
  if (stage === "recognizing") {
    return `${recognizedCount}/${totalCount} Gemini responses`;
  }
  if (stage === "complete") {
    return totalCount > 0 ? `${recognizedCount} recognitions saved` : "No beers detected";
  }
  if (stage === "error") {
    return failedCount > 0 ? `${failedCount} rows need attention` : "Pipeline stopped";
  }

  return "Ready";
}

function formatDetectedCount(count) {
  return `${count} ${count === 1 ? "beer" : "beers"} detected`;
}

function friendlyError(err) {
  if (err instanceof DOMException && err.name === "AbortError") {
    return "Canceled";
  }
  if (err instanceof Error && err.message.trim()) {
    return err.message.trim();
  }

  return "Request failed";
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
            className: [
              "nav-item",
              section.key === "add" ? "add-nav" : "",
              activeSection === section.key ? "active" : "",
            ]
              .filter(Boolean)
              .join(" "),
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

  return SECTIONS[key] ? key : "add";
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
