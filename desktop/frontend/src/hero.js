// The tunnel: three nested arches. Grey when idle, brass pulse while
// connecting, and a slow teal flow through the arches once protected — the
// same picture as the phone apps.
import { h, formatBytes, formatDuration } from "./dom.js";
import { t } from "./i18n.js";

const W = 220, H = 170, STROKE = 7, GAP = 20;

function arches() {
  let d = "";
  const parts = [];
  for (let i = 0; i < 3; i++) {
    const inset = i * GAP + STROKE / 2;
    const r = (W - inset * 2) / 2;
    const bottom = H - STROKE / 2;
    d = `M${inset},${bottom} L${inset},${inset + r} A${r},${r} 0 0 1 ${W - inset},${inset + r} L${W - inset},${bottom}`;
    parts.push(`<path class="arch arch-${i}" d="${d}"/>`);
  }
  return `<svg class="arches" viewBox="0 0 ${W} ${H + 1}" width="${W}" height="${H + 1}" aria-hidden="true">
    ${parts.join("")}<line class="ground" x1="0" y1="${H}" x2="${W}" y2="${H}"/></svg>`;
}

const phaseText = {
  disconnected: "status_disconnected",
  connecting: "status_connecting",
  connected: "status_connected",
  reconnecting: "status_reconnecting",
  disconnecting: "status_disconnecting",
  failed: "status_failed",
};

const stepText = {
  RESOLVE: "step_resolve",
  TCP_CONNECT: "step_connecting",
  WAIT: "step_connecting",
  AUTH: "step_auth",
  GET_CONFIG: "step_get_config",
  ASSIGN_IP: "step_assign_ip",
  ADD_ROUTES: "step_add_routes",
};

/** Builds the hero; returns {el, update(status, name, message)}. */
export function statusHero() {
  const art = h("div.hero-art", { html: arches() });
  const title = h("h2.hero-title", { "aria-live": "polite" });
  const sub = h("p.hero-sub");
  const stats = h("div.hero-stats");
  const el = h("section.hero", art, title, sub, stats);
  let timer;

  function update(status, name) {
    const phase = status.phase || "disconnected";
    el.dataset.phase = phase;
    title.textContent = t(phaseText[phase] || "status_disconnected");
    clearInterval(timer);

    const bits = [];
    if (name) bits.push(name);
    const subLine = () => {
      const extra = [];
      if (phase === "connected" && status.connectedAt) extra.push(formatDuration(Date.now() - status.connectedAt));
      else if (phase === "connecting" && stepText[status.step]) extra.push(t(stepText[status.step]));
      sub.textContent = [...bits, ...extra].join("  ·  ");
    };
    subLine();
    if (phase === "connected") timer = setInterval(subLine, 1000);

    stats.replaceChildren();
    if (phase === "connected") {
      const stat = (label, value) => h("div.stat", h("span.stat-label", label), h("span.stat-value", value));
      stats.append(
        stat(t("label_address"), status.vpnAddress || "—"),
        stat(t("label_down"), formatBytes(status.bytesIn)),
        stat(t("label_up"), formatBytes(status.bytesOut)),
      );
    }
  }
  return { el, update };
}
