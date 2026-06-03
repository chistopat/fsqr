(() => {
  const defaultCenter = { lat: 34.772013, lon: 32.429736, zoom: 13 };
  const maxSearchDistanceMeters = 50000;
  const minSearchDistanceMeters = 500;
  const searchLimit = 128;

  const queryInput = document.getElementById("query");
  const form = document.getElementById("search-form");
  const statusDot = document.getElementById("status-dot");
  const statusText = document.getElementById("status-text");
  const statsText = document.getElementById("stats-text");
  const summary = document.getElementById("summary");
  const details = document.getElementById("details");
  const results = document.getElementById("results");
  const locateButton = document.getElementById("locate-button");
  const resultTemplate = document.getElementById("result-row-template");

  const initial = readInitialState();
  queryInput.value = initial.query;

  const map = L.map("map", {
    center: [initial.lat, initial.lon],
    zoom: initial.zoom,
    zoomControl: true,
    preferCanvas: true,
  });

  L.tileLayer("https://tile.openstreetmap.org/{z}/{x}/{y}.png", {
    maxZoom: 19,
    attribution: "&copy; OpenStreetMap contributors",
  }).addTo(map);

  const markerLayer = L.layerGroup().addTo(map);
  const state = {
    abort: null,
    places: [],
    selectedUUID: "",
    searchTimer: 0,
    lastStartedAt: 0,
    suppressNextMoveSearch: false,
  };

  renderSummary({ tookMS: 0, places: 0, distance: currentDistanceMeters() });
  syncCoordinateText();

  form.addEventListener("submit", event => {
    event.preventDefault();
    runSearch("manual");
  });

  queryInput.addEventListener("input", () => {
    expandQueryInput();
    window.clearTimeout(state.searchTimer);
    state.searchTimer = window.setTimeout(() => runSearch("typing"), 650);
  });

  document.querySelectorAll("[data-query]").forEach(button => {
    button.addEventListener("click", () => {
      queryInput.value = button.dataset.query;
      expandQueryInput();
      runSearch("preset");
    });
  });

  locateButton.addEventListener("click", () => {
    if (!navigator.geolocation) {
      setStatus("error", "Browser geolocation is not available");
      return;
    }

    setStatus("busy", "Waiting for browser location");
    navigator.geolocation.getCurrentPosition(
      position => {
        const { latitude, longitude } = position.coords;
        map.setView([latitude, longitude], Math.max(map.getZoom(), 14));
        runSearch("location");
      },
      () => setStatus("error", "Could not read browser location"),
      { enableHighAccuracy: true, timeout: 8000, maximumAge: 60000 },
    );
  });

  map.on("moveend", () => {
    syncCoordinateText();
    if (state.suppressNextMoveSearch) {
      state.suppressNextMoveSearch = false;
      return;
    }

    window.clearTimeout(state.searchTimer);
    state.searchTimer = window.setTimeout(() => runSearch("map"), 450);
  });

  expandQueryInput();
  runSearch("initial");

  function readInitialState() {
    const params = new URLSearchParams(window.location.search);
    const lat = finiteNumber(params.get("lat"), defaultCenter.lat);
    const lon = finiteNumber(params.get("lon"), defaultCenter.lon);
    const zoom = finiteNumber(params.get("z") || params.get("zoom"), defaultCenter.zoom);
    const query = params.get("q") || params.get("query") || "coffee nearby";

    return { lat, lon, zoom, query };
  }

  function finiteNumber(raw, fallback) {
    const value = Number(raw);
    return Number.isFinite(value) ? value : fallback;
  }

  async function runSearch(reason) {
    const query = queryInput.value.trim();
    if (!query) {
      markerLayer.clearLayers();
      results.replaceChildren();
      details.className = "details-card empty";
      details.textContent = "Type a human search intent to query places.";
      renderSummary({ tookMS: 0, places: 0, distance: currentDistanceMeters() });
      setStatus("error", "Search query is empty");
      return;
    }

    if (state.abort) {
      state.abort.abort();
    }

    const center = map.getCenter();
    const distance = currentDistanceMeters();
    const requestID = Date.now();
    state.lastStartedAt = requestID;
    state.abort = new AbortController();

    setStatus("busy", `Searching ${query}`);
    syncCoordinateText();
    writePermalink(query, center);

    const url = new URL("/api/v1/search", window.location.origin);
    url.searchParams.set("query", query);
    url.searchParams.set("location", `${center.lat.toFixed(6)},${center.lng.toFixed(6)}`);
    url.searchParams.set("limit", String(searchLimit));
    url.searchParams.set("distance_meters", String(distance));

    try {
      const response = await fetch(url, { signal: state.abort.signal });
      if (!response.ok) {
        throw new Error(await readAPIError(response));
      }

      const payload = await response.json();
      if (state.lastStartedAt !== requestID) {
        return;
      }

      state.places = Array.isArray(payload.places) ? payload.places : [];
      state.selectedUUID = "";
      renderPlaces(state.places);
      renderResults(state.places);
      renderSummary({ tookMS: payload.took_ms || 0, places: state.places.length, distance });
      clearDetails(state.places.length);
      setStatus("ready", statusMessage(reason, state.places.length));
    } catch (error) {
      if (error.name === "AbortError") {
        return;
      }
      setStatus("error", error.message || "Search failed");
    }
  }

  async function readAPIError(response) {
    try {
      const payload = await response.json();
      return payload.error?.message || `HTTP ${response.status}`;
    } catch {
      return `HTTP ${response.status}`;
    }
  }

  function statusMessage(reason, count) {
    const prefix = reason === "map" ? "Map moved" : "Search complete";
    return `${prefix}: ${count.toLocaleString()} places`;
  }

  function renderPlaces(places) {
    markerLayer.clearLayers();

    places.forEach((place, index) => {
      const latlng = [place.lat, place.lon];
      const rankOpacity = Math.max(0.28, 0.85 - index * 0.004);
      const halo = L.circleMarker(latlng, {
        radius: 18,
        color: "#ff6a00",
        weight: 0,
        fillColor: "#ff6a00",
        fillOpacity: 0.12,
        className: "place-halo",
      });
      const marker = L.circleMarker(latlng, {
        radius: 5,
        color: "#fff15a",
        weight: 1,
        fillColor: "#ff6a00",
        fillOpacity: rankOpacity,
        className: "place-dot",
      });

      marker.bindTooltip(place.name || place.uuid, { direction: "top", offset: [0, -6] });
      marker.on("click", () => selectPlace(place));
      halo.on("click", () => selectPlace(place));
      markerLayer.addLayer(halo);
      markerLayer.addLayer(marker);
    });
  }

  function renderResults(places) {
    results.replaceChildren();

    places.forEach((place, index) => {
      const row = resultTemplate.content.firstElementChild.cloneNode(true);
      row.dataset.uuid = place.uuid;
      row.querySelector(".result-name").textContent = `${index + 1}. ${place.name}`;
      row.querySelector(".result-meta").textContent = `${place.lat.toFixed(6)}, ${place.lon.toFixed(6)}`;
      row.querySelector("button").addEventListener("click", () => selectPlace(place));
      results.append(row);
    });
  }

  async function selectPlace(place) {
    state.selectedUUID = place.uuid;
    highlightSelectedResult();
    state.suppressNextMoveSearch = true;
    map.panTo([place.lat, place.lon], { animate: true, duration: 0.4 });

    details.className = "details-card";
    details.textContent = "Loading details...";

    try {
      const response = await fetch(`/api/v1/places/${encodeURIComponent(place.uuid)}`);
      if (!response.ok) {
        throw new Error(await readAPIError(response));
      }
      renderDetails(await response.json());
    } catch (error) {
      renderDetails({ ...place, error: error.message || "Details failed" });
    }
  }

  function highlightSelectedResult() {
    results.querySelectorAll(".result-row").forEach(row => {
      row.classList.toggle("selected", row.dataset.uuid === state.selectedUUID);
    });
  }

  function renderDetails(place) {
    details.className = "details-card";
    details.replaceChildren();

    const title = document.createElement("h2");
    title.textContent = place.name || place.uuid;
    details.append(title);

    appendDetail(`${place.lat?.toFixed?.(6) || "?"}, ${place.lon?.toFixed?.(6) || "?"}`);
    if (place.category) {
      appendDetail(place.category.path || place.category.name);
    }
    if (place.address) {
      appendDetail(formatAddress(place.address));
    }
    if (place.contacts?.website) {
      appendDetail(place.contacts.website);
    }
    if (place.contacts?.tel) {
      appendDetail(place.contacts.tel);
    }
    if (place.error) {
      appendDetail(place.error);
    }
  }

  function appendDetail(text) {
    if (!text) {
      return;
    }
    const paragraph = document.createElement("p");
    paragraph.textContent = text;
    details.append(paragraph);
  }

  function clearDetails(count) {
    details.className = "details-card empty";
    details.textContent = count ? "Select a result for details." : "No places found in this map window.";
  }

  function formatAddress(address) {
    return [address.line, address.locality, address.region, address.country].filter(Boolean).join(", ");
  }

  function renderSummary({ tookMS, places, distance }) {
    const center = map.getCenter();
    const cells = [
      ["places", places.toLocaleString()],
      ["took", `${Number(tookMS).toLocaleString()} ms`],
      ["distance", `${distance.toLocaleString()} m`],
      ["center", `${center.lat.toFixed(4)}, ${center.lng.toFixed(4)}`],
    ];

    summary.replaceChildren(...cells.map(([label, value]) => {
      const cell = document.createElement("div");
      cell.className = "summary-cell";

      const labelNode = document.createElement("span");
      labelNode.textContent = label;
      const valueNode = document.createElement("strong");
      valueNode.textContent = value;

      cell.append(labelNode, valueNode);
      return cell;
    }));
  }

  function currentDistanceMeters() {
    const center = map.getCenter();
    const bounds = map.getBounds();
    const latDistance = center.distanceTo(L.latLng(bounds.getNorth(), center.lng));
    const lonDistance = center.distanceTo(L.latLng(center.lat, bounds.getEast()));
    const distance = Math.ceil(Math.max(latDistance, lonDistance));

    return Math.max(minSearchDistanceMeters, Math.min(maxSearchDistanceMeters, distance));
  }

  function setStatus(kind, message) {
    statusDot.className = `status-dot ${kind === "ready" ? "" : kind}`.trim();
    statusText.textContent = message;
  }

  function syncCoordinateText() {
    const center = map.getCenter();
    statsText.textContent = `center ${center.lat.toFixed(6)}, ${center.lng.toFixed(6)} / z${map.getZoom()}`;
  }

  function writePermalink(query, center) {
    const params = new URLSearchParams();
    params.set("q", query);
    params.set("lat", center.lat.toFixed(6));
    params.set("lon", center.lng.toFixed(6));
    params.set("z", String(map.getZoom()));
    window.history.replaceState(null, "", `${window.location.pathname}?${params}`);
  }

  function expandQueryInput() {
    queryInput.style.height = "auto";
    queryInput.style.height = `${Math.min(queryInput.scrollHeight, 160)}px`;
  }
})();
