const API = "http://localhost:8081/api/v1";

const $ = (id) => document.getElementById(id);

function setConnection(state) {
  const badge = $("connectionBadge");
  const hero = document.querySelector(".pulse-ring");

  badge.className = `badge ${state}`;

  if (state === "online") {
    badge.innerHTML = '<span class="dot"></span> ONLINE';
    hero.className = "pulse-ring online";
  } else if (state === "offline") {
    badge.innerHTML = '<span class="dot"></span> OFFLINE';
    hero.className = "pulse-ring offline";
  } else {
    badge.innerHTML = '<span class="dot"></span> CONNECTING';
    hero.className = "pulse-ring";
  }
}

async function getJSON(path) {
  const response = await fetch(`${API}${path}`, {
    method: "GET",
    cache: "no-store"
  });

  if (!response.ok) {
    throw new Error(`${path} returned HTTP ${response.status}`);
  }

  return response.json();
}

function formatUptime(seconds) {
  seconds = Number(seconds || 0);

  const days = Math.floor(seconds / 86400);
  seconds %= 86400;

  const hours = Math.floor(seconds / 3600);
  seconds %= 3600;

  const minutes = Math.floor(seconds / 60);

  if (days > 0) return `${days}d ${hours}h`;
  if (hours > 0) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function formatAlgorithm(value) {
  return String(value || "—")
    .replaceAll("_", " ")
    .replace(/\b\w/g, c => c.toUpperCase());
}

function getBackendHealthy(backend) {
  if (typeof backend.healthy === "boolean") return backend.healthy;

  if (backend.healthy && typeof backend.healthy === "object") {
    if ("value" in backend.healthy) return Boolean(backend.healthy.value);
  }

  if (typeof backend.is_healthy === "boolean") return backend.is_healthy;

  return false;
}

function backendAddress(backend) {
  return backend.address || backend.url || backend.target || "Unknown address";
}

function backendName(backend, index) {
  return backend.id || backend.name || `Backend ${index + 1}`;
}

function renderStatus(status) {
  $("proxyStatus").textContent = status.status || "UNKNOWN";
  $("healthyCount").textContent = status.backends_healthy ?? "—";
  $("backendTotal").textContent = `/ ${status.backends_total ?? "—"} backends`;
  $("algorithm").textContent = formatAlgorithm(status.load_balancer_algorithm);
  $("uptime").textContent = formatUptime(status.uptime_seconds);
  $("cpu").textContent = status.num_cpu ? `${status.num_cpu} cores` : "—";
  $("cpuCores").textContent = status.num_cpu ?? "—";

  $("lastUpdated").textContent =
    `Updated ${new Date().toLocaleTimeString()}`;

  const operational =
    String(status.status || "").toUpperCase() === "OPERATIONAL";

  setConnection(operational ? "online" : "offline");
}

function renderBackends(backends) {
  const list = Array.isArray(backends) ? backends : [];

  const healthy = list.filter(getBackendHealthy).length;

  $("backendSummary").textContent =
    `${healthy}/${list.length} healthy`;

  if (!list.length) {
    $("backends").innerHTML =
      '<div class="empty">No backend servers found.</div>';
    return;
  }

  $("backends").innerHTML = list.map((backend, index) => {
    const isHealthy = getBackendHealthy(backend);
    const name = backendName(backend, index);
    const address = backendAddress(backend);

    return `
      <div class="backend">
        <div>
          <div class="backend-name">${escapeHTML(name)}</div>
          <div class="backend-address">${escapeHTML(address)}</div>
        </div>
        <span class="status ${isHealthy ? "healthy" : "unhealthy"}">
          ${isHealthy ? "HEALTHY" : "DOWN"}
        </span>
      </div>
    `;
  }).join("");

  const down = list.filter(b => !getBackendHealthy(b));

  if (down.length) {
    showAlert(
      `${down.length} backend${down.length > 1 ? "s are" : " is"} DOWN. FlexiProxy should avoid unhealthy servers.`
    );
  } else {
    hideAlert();
  }
}

function renderBreakers(states) {
  const entries = Object.entries(states || {});

  const openCount = entries.filter(
    ([, state]) => String(state).toUpperCase() === "OPEN"
  ).length;

  $("breakerSummary").textContent =
    openCount ? `${openCount} open` : "All closed";

  if (!entries.length) {
    $("breakers").innerHTML =
      '<div class="empty">No circuit-breaker data.</div>';
    return;
  }

  $("breakers").innerHTML = entries.map(([id, state]) => {
    const normalized = String(state || "UNKNOWN")
      .toLowerCase()
      .replaceAll("-", "_")
      .replaceAll(" ", "_");

    return `
      <div class="breaker">
        <span class="breaker-name">${escapeHTML(id)}</span>
        <span class="breaker-state breaker ${normalized}">
          ${escapeHTML(String(state))}
        </span>
      </div>
    `;
  }).join("");
}

function showAlert(message) {
  $("alert").textContent = `⚠ ${message}`;
  $("alert").classList.remove("hidden");
}

function hideAlert() {
  $("alert").classList.add("hidden");
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#039;");
}

async function refresh() {
  setConnection("connecting");

  try {
    const [status, backends, breakers] = await Promise.all([
      getJSON("/status"),
      getJSON("/backends"),
      getJSON("/circuit-breakers")
    ]);

    renderStatus(status);
    renderBackends(backends);
    renderBreakers(breakers);
  } catch (error) {
    console.error("FlexiProxy connection failed:", error);

    setConnection("offline");
    $("proxyStatus").textContent = "OFFLINE";
    $("lastUpdated").textContent =
      "Cannot connect to localhost:8081";

    $("backends").innerHTML =
      '<div class="empty">Start FlexiProxy to view backend status.</div>';

    $("breakers").innerHTML =
      '<div class="empty">Admin API unavailable.</div>';

    showAlert(
      "FlexiProxy Admin API is unavailable. Make sure the Go proxy is running on port 8081."
    );
  }
}

$("refreshBtn").addEventListener("click", refresh);

refresh();

setInterval(refresh, 2000);
