// Tunnelkey setup codes — static edition. Runs entirely in the browser: the
// profile, password and TOTP secret never leave this page (see the CSP in
// index.html: no network access). DOM is built with h(); user data is only
// ever set as text, never as HTML.

import qrcode from "./vendor/qrcode.js";
import {
  MAX_LINKS, MAX_PROFILE_BYTES, SETUP_FILE_MIN_PASSWORD, SETUP_FILE_TYPE, buildPayload,
  decodeSetupCodes, encodeSetupCodes, encryptSetupFile, generatePassphrase, inspectOvpn,
  normalizeTotp, parseOtpauth, secondsLeft, totpCode,
} from "./core.js";

const app = document.getElementById("app");

// ---------------------------------------------------------------- helpers

function h(tag, props = {}, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(props || {})) {
    if (v == null || v === false) continue;
    if (k === "class") el.className = v;
    else if (k === "text") el.textContent = v;
    else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
    else if (k in el && k !== "list" && k !== "form") el[k] = v;
    else el.setAttribute(k, v === true ? "" : v);
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue;
    el.append(c instanceof Node ? c : document.createTextNode(String(c)));
  }
  return el;
}

const SVG_NS = "http://www.w3.org/2000/svg";
const ICONS = {
  plus: ["M12 5v14", "M5 12h14"],
  qr: ["M4 4h6v6H4z", "M14 4h6v6h-6z", "M4 14h6v6H4z", "M14 14h2v2h-2z", "M18 18h2v2h-2z", "M14 18h2", "M18 14h2"],
  edit: ["M4 20h4L19 9l-4-4L4 16z", "M13 7l4 4"],
  trash: ["M4 7h16", "M10 11v6", "M14 11v6", "M6 7l1 13h10l1-13", "M9 7V4h6v3"],
  up: ["M12 19V5", "M6 11l6-6 6 6"],
  down: ["M12 5v14", "M6 13l6 6 6-6"],
  file: ["M6 3h8l4 4v14H6z", "M14 3v4h4", "M9 13h6", "M9 17h6"],
  globe: ["M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z", "M3 12h18", "M12 3c3 3.5 3 14.5 0 18", "M12 3c-3 3.5-3 14.5 0 18"],
  monitor: ["M3 5h18v11H3z", "M8 20h8", "M12 16v4"],
  app: ["M5 5h6v6H5z", "M13 5h6v6h-6z", "M5 13h6v6H5z", "M13 13h6v6h-6z"],
  warn: ["M12 3l10 18H2z", "M12 10v4", "M12 17.5v.5"],
  check: ["M5 12l5 5 9-10"],
  shield: ["M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z", "M8.5 12l2.5 2.5 4.5-5"],
  play: ["M7 4l13 8-13 8z"],
  print: ["M6 9V3h12v6", "M6 18H3v-8h18v8h-3", "M6 14h12v7H6z"],
  back: ["M19 12H5", "M11 6l-6 6 6 6"],
  save: ["M5 3h11l3 3v15H5z", "M8 3v5h8V3", "M8 14h8v7H8z"],
  open: ["M3 7h6l2 2h10v10H3z"],
  download: ["M12 4v11", "M7 10l5 5 5-5", "M5 20h14"],
  key: ["M8 11a4 4 0 1 0 0 8 4 4 0 0 0 0-8z", "M11 12l9-9", "M17 6l3 3", "M15 8l2 2"],
  copy: ["M9 9h11v11H9z", "M5 15H4V4h11v1"],
  eye: ["M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12z", "M12 9a3 3 0 1 0 0 6 3 3 0 0 0 0-6z"],
  eyeOff: ["M3 3l18 18", "M10.6 5.1A10.8 10.8 0 0 1 12 5c6.5 0 10 7 10 7a17 17 0 0 1-3.2 4.2", "M6.6 6.6C3.8 8.4 2 12 2 12s3.5 7 10 7c1.9 0 3.5-.6 4.9-1.4", "M9.9 9.9a3 3 0 0 0 4.2 4.2"],
};

function icon(name) {
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "1.8");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  svg.setAttribute("aria-hidden", "true");
  for (const d of ICONS[name] || []) {
    const p = document.createElementNS(SVG_NS, "path");
    p.setAttribute("d", d);
    svg.append(p);
  }
  return svg;
}

function toast(message) {
  const t = h("div", { class: "toast", role: "status", text: message });
  document.body.append(t);
  setTimeout(() => t.remove(), 2600);
}

function banner(kind, text) {
  return h("div", { class: `banner ${kind}` }, icon(kind === "ok" ? "check" : "warn"), h("div", { text }));
}

function segmented(name, options, value, onChange) {
  const wrap = h("div", { class: "segmented", role: "radiogroup" });
  for (const [v, label] of options) {
    const id = `${name}-${v}`;
    wrap.append(
      h("input", { type: "radio", name, id, value: v, checked: v === value, onchange: () => onChange(v) }),
      h("label", { for: id, text: label }),
    );
  }
  return wrap;
}

function select(options, value, onChange) {
  const el = h("select", { onchange: (e) => onChange(e.target.value) }, options.map(([v, label]) => h("option", { value: v, text: label })));
  el.value = value;
  return el;
}

function download(filename, blob) {
  const url = URL.createObjectURL(blob);
  const a = h("a", { href: url, download: filename });
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

const slug = (s) => (s || "tunnelkey").normalize("NFKD").replace(/[^\w.-]+/g, "-").replace(/^-+|-+$/g, "").toLowerCase() || "tunnelkey";

// ---------------------------------------------------------------- state

const emptyState = () => ({
  name: "",
  ovpn: "",
  username: "",
  password: "",
  twofa: "none", // none | totp | manual
  totp: { secret: "", digits: 6, period: 30, algorithm: "SHA1" },
  codePosition: "after",
  links: [],
});

let s = emptyState();
let cleanup = [];

function runCleanup() {
  for (const fn of cleanup) fn();
  cleanup = [];
}

/** Package in the same shape as the server's (also the saved file format). */
function toPackage() {
  return {
    name: s.name,
    ovpn: s.ovpn,
    username: s.username,
    password: s.password,
    totp: s.twofa === "totp" ? s.totp : null,
    manualCode: s.twofa === "manual",
    codePosition: s.codePosition,
    links: s.links,
  };
}

function fromPackage(p) {
  const t = p.totp && p.totp.secret ? p.totp : null;
  return {
    name: p.name || "",
    ovpn: p.ovpn || "",
    username: p.username || "",
    password: p.password || "",
    twofa: t ? "totp" : p.manualCode ? "manual" : "none",
    totp: { secret: t?.secret || "", digits: t?.digits || 6, period: t?.period || 30, algorithm: t?.algorithm || "SHA1" },
    codePosition: p.codePosition === "before" ? "before" : "after",
    links: Array.isArray(p.links) ? p.links.map((l) => ({ ...l })) : [],
  };
}

// ---------------------------------------------------------------- editor

function renderEditor() {
  runCleanup();

  // ---- 1. Profile
  const nameInput = h("input", { type: "text", id: "name", value: s.name, maxLength: 80, placeholder: "e.g. Office VPN", oninput: (e) => { s.name = e.target.value; updatePreview(); } });
  const fileInput = h("input", { type: "file", accept: ".ovpn,.conf,text/plain", id: "file" });
  const dropText = h("div", {}, h("strong", { text: s.ovpn ? "Replace .ovpn file" : "Choose .ovpn file" }), h("span", { class: "muted", text: "or drop it here — certificates and keys must be inline" }));
  const drop = h("label", { class: "drop", for: "file" }, h("div", { class: "drop-icon" }, icon("file")), dropText, fileInput);
  const detected = h("div", { class: "detected" });
  const profileWarnings = h("div");
  const raw = h("textarea", { spellcheck: false, value: s.ovpn, oninput: (e) => { s.ovpn = e.target.value; refreshProfile(); } });
  const noAuthNote = h("p", { class: "muted", hidden: true, text: "This profile signs in with certificates only (no auth-user-pass), so no credentials are needed." });
  let authCard;

  function refreshProfile() {
    const info = inspectOvpn(s.ovpn);
    detected.replaceChildren(...[
      info.remote && h("span", { class: "badge secure mono", text: info.remote }),
      ...info.inline.map((b) => h("span", { class: "badge", text: `<${b}>` })),
      info.auth && h("span", { class: "badge brass", text: "auth-user-pass" }),
      info.nocache && h("span", { class: "badge", text: "auth-nocache" }),
    ].filter(Boolean));
    const warnings = [];
    if (s.ovpn && !info.remote) warnings.push("No “remote” line found — this doesn't look like a client profile.");
    if (info.external.length) warnings.push(`References separate files (${info.external.join(", ")}). Embed them inline — the phone can't read other files.`);
    if (info.staticChallenge) warnings.push("The profile uses static-challenge: the phone sends the code as the challenge response instead of appending it to the password.");
    profileWarnings.replaceChildren(...warnings.map((w) => h("div", { class: "field" }, banner("warn", w))));
    dropText.firstChild.textContent = s.ovpn ? "Replace .ovpn file" : "Choose .ovpn file";
    if (authCard) authCard.hidden = !!s.ovpn && !info.auth;
    noAuthNote.hidden = !(s.ovpn && !info.auth);
    updatePreview();
  }

  async function loadFile(file) {
    if (!file) return;
    if (file.size > MAX_PROFILE_BYTES) { toast("That file is too large for a profile."); return; }
    s.ovpn = await file.text();
    raw.value = s.ovpn;
    if (!s.name) { s.name = file.name.replace(/\.(ovpn|conf)$/i, ""); nameInput.value = s.name; }
    refreshProfile();
  }
  fileInput.addEventListener("change", () => loadFile(fileInput.files[0]));
  drop.addEventListener("dragover", (e) => { e.preventDefault(); drop.classList.add("over"); });
  drop.addEventListener("dragleave", () => drop.classList.remove("over"));
  drop.addEventListener("drop", (e) => { e.preventDefault(); drop.classList.remove("over"); loadFile(e.dataTransfer.files[0]); });

  const profileCard = h("section", { class: "card" },
    h("div", { class: "card-head" }, h("h2", {}, h("span", { class: "step", text: "1" }), "Profile")),
    h("div", { class: "field" }, h("label", { for: "name", text: "Name shown on the phone" }), nameInput),
    h("div", { class: "field" }, drop, detected),
    profileWarnings,
    h("details", { class: "raw" }, h("summary", { text: "View or edit profile text" }), raw));

  // ---- 2. Sign-in
  const userInput = h("input", { type: "text", id: "user", value: s.username, autocomplete: "off", oninput: (e) => { s.username = e.target.value; } });
  const passInput = h("input", { type: "password", id: "pass", value: s.password, autocomplete: "new-password", placeholder: "Leave empty to ask on the phone", oninput: (e) => { s.password = e.target.value; } });

  const twofaChoice = segmented("twofa", [["none", "No 2FA"], ["totp", "Phone generates the code"], ["manual", "User types the code"]], s.twofa, (v) => { s.twofa = v; refreshTwofa(); });
  const secretInput = h("input", {
    type: "text", class: "mono", id: "secret", autocomplete: "off", spellcheck: false, value: s.totp.secret,
    placeholder: "Base32 secret or otpauth:// link",
    oninput: (e) => { s.totp.secret = e.target.value.trim(); refreshCode(); },
  });
  const digitsSel = select([["6", "6 digits"], ["8", "8 digits"]], String(s.totp.digits), (v) => { s.totp.digits = +v; refreshCode(); });
  const periodInput = h("input", { type: "number", min: 10, max: 300, value: s.totp.period, oninput: (e) => { s.totp.period = +e.target.value || 30; refreshCode(); } });
  const algoSel = select([["SHA1", "SHA-1 (standard)"], ["SHA256", "SHA-256"], ["SHA512", "SHA-512"]], s.totp.algorithm, (v) => { s.totp.algorithm = v; refreshCode(); });

  const codeEl = h("div", { class: "totp-code", text: "——————" });
  const ring = document.createElementNS(SVG_NS, "svg");
  ring.setAttribute("class", "ring");
  ring.setAttribute("viewBox", "0 0 36 36");
  const track = document.createElementNS(SVG_NS, "circle");
  const ringFill = document.createElementNS(SVG_NS, "circle");
  for (const c of [track, ringFill]) { c.setAttribute("cx", "18"); c.setAttribute("cy", "18"); c.setAttribute("r", "15"); }
  track.setAttribute("class", "track");
  ringFill.setAttribute("class", "fill");
  const CIRC = 2 * Math.PI * 15;
  ringFill.setAttribute("stroke-dasharray", String(CIRC));
  ring.append(track, ringFill);
  const totpError = h("div");

  const totpFields = h("div", {},
    h("div", { class: "field" }, h("label", { for: "secret", text: "TOTP secret" }), secretInput,
      h("div", { class: "hint", text: "Paste the base32 secret or the whole otpauth:// link from the user's 2FA enrolment." })),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", { text: "Digits" }), digitsSel),
      h("div", { class: "field" }, h("label", { text: "Period (s)" }), periodInput),
      h("div", { class: "field" }, h("label", { text: "Algorithm" }), algoSel)),
    totpError,
    h("div", { class: "totp-preview" }, ring, h("div", {}, codeEl, h("div", { class: "muted", text: "Compare with the code your server or authenticator shows right now." }))));

  const positionField = h("div", { class: "field" },
    h("label", { text: "How the server expects it" }),
    segmented("pos", [["after", "Password + code"], ["before", "Code + password"]], s.codePosition, (v) => { s.codePosition = v; }),
    h("div", { class: "hint", text: "The code is joined to the password and sent as one password, e.g. hunter2 + 123456 → hunter2123456." }));

  function refreshTwofa() {
    totpFields.hidden = s.twofa !== "totp";
    positionField.hidden = s.twofa === "none";
    refreshCode();
    updatePreview();
  }

  let codeValid = false;
  async function refreshCode() {
    totpError.replaceChildren();
    codeValid = false;
    if (s.twofa !== "totp" || !s.totp.secret) { codeEl.textContent = "——————"; return; }
    try {
      if (s.totp.secret.startsWith("otpauth://")) {
        // Adopt the link's settings, keep only the secret in the field.
        s.totp = parseOtpauth(s.totp.secret);
        secretInput.value = s.totp.secret;
        digitsSel.value = String(s.totp.digits);
        periodInput.value = s.totp.period;
        algoSel.value = s.totp.algorithm;
      }
      normalizeTotp(s.totp);
      const code = await totpCode(s.totp);
      codeEl.textContent = code.replace(/(\d{3,4})(\d{3,4})/, "$1 $2");
      codeValid = true;
      tick();
    } catch (err) {
      codeEl.textContent = "——————";
      totpError.replaceChildren(h("div", { class: "field" }, banner("error", err.message)));
    }
  }
  let lastWindow = null;
  function tick() {
    if (!codeValid) return;
    const period = Number(s.totp.period) || 30;
    const left = secondsLeft(period);
    ringFill.setAttribute("stroke-dashoffset", String(CIRC * (1 - left / period)));
    const win = Math.floor(Date.now() / 1000 / period);
    if (lastWindow !== null && win !== lastWindow) refreshCode();
    lastWindow = win;
  }
  const timer = setInterval(tick, 1000);
  cleanup.push(() => clearInterval(timer));

  authCard = h("section", { class: "card" },
    h("div", { class: "card-head" }, h("h2", {}, h("span", { class: "step", text: "2" }), "Sign-in")),
    h("div", { class: "row" },
      h("div", { class: "field" }, h("label", { for: "user", text: "Username" }), userInput),
      h("div", { class: "field" }, h("label", { for: "pass", text: "Password (optional)" }), passInput)),
    h("div", { class: "field" }, h("label", { text: "Two-factor code" }), twofaChoice),
    totpFields,
    positionField);

  // ---- 3. Links
  const linksEl = h("div", { class: "links" });
  const addLink = h("button", { class: "btn ghost", type: "button", onclick: () => { s.links.push({ kind: "web", title: "", url: "" }); renderLinks(true); } }, icon("plus"), "Add link");

  function renderLinks(focusLast) {
    linksEl.replaceChildren(...s.links.map((l, i) => linkRow(l, i)));
    addLink.disabled = s.links.length >= MAX_LINKS;
    if (focusLast) linksEl.lastElementChild?.querySelector("input")?.focus();
    updatePreview();
  }

  function linkRow(l, i) {
    const kind = select([["web", "Website"], ["rdp", "Remote Desktop (RDP)"], ["app", "App link (URI)"]], l.kind, (v) => { l.kind = v; renderLinks(); });
    const fields = [];
    if (l.kind === "web") {
      fields.push(h("div", { class: "field" }, h("label", { text: "Address" }),
        h("input", { type: "url", value: l.url || "", placeholder: "https://intranet.example.com", oninput: (e) => { l.url = e.target.value; } })));
    } else if (l.kind === "rdp") {
      fields.push(h("div", { class: "row" },
        h("div", { class: "field" }, h("label", { text: "Computer (host or IP)" }),
          h("input", { type: "text", value: l.host || "", placeholder: "10.0.0.25 or pc01.corp.local", oninput: (e) => { l.host = e.target.value; } })),
        h("div", { class: "field" }, h("label", { text: "Port" }),
          h("input", { type: "number", min: 1, max: 65535, value: l.port || 3389, oninput: (e) => { l.port = +e.target.value; } })),
        h("div", { class: "field" }, h("label", { text: "Windows user (optional)" }),
          h("input", { type: "text", value: l.username || "", placeholder: "CORP\\marko", oninput: (e) => { l.username = e.target.value; } }))),
        h("div", { class: "hint", text: "Opens in Microsoft's Windows App / Remote Desktop client on the phone. The Windows password is typed there." }));
    } else {
      fields.push(h("div", { class: "field" }, h("label", { text: "URI" }),
        h("input", { type: "text", class: "mono", value: l.url || "", placeholder: "myapp://open?server=…", oninput: (e) => { l.url = e.target.value; } })));
    }
    const move = (d) => { const j = i + d; [s.links[i], s.links[j]] = [s.links[j], s.links[i]]; renderLinks(); };
    return h("div", { class: "link-row" },
      h("div", { class: "link-row-head" }, kind, h("div", { class: "spacer" }),
        h("button", { class: "icon-btn", type: "button", title: "Move up", "aria-label": "Move up", disabled: i === 0, onclick: () => move(-1) }, icon("up")),
        h("button", { class: "icon-btn", type: "button", title: "Move down", "aria-label": "Move down", disabled: i === s.links.length - 1, onclick: () => move(1) }, icon("down")),
        h("button", { class: "icon-btn", type: "button", title: "Remove", "aria-label": "Remove link", onclick: () => { s.links.splice(i, 1); renderLinks(); } }, icon("trash"))),
      h("div", { class: "field" }, h("label", { text: "Title" }),
        h("input", { type: "text", value: l.title || "", maxLength: 60, placeholder: "Title shown on the phone", oninput: (e) => { l.title = e.target.value; updatePreview(); } })),
      fields);
  }

  const linksCard = h("section", { class: "card" },
    h("div", { class: "card-head" }, h("h2", {}, h("span", { class: "step", text: "3" }), "Links"),
      h("span", { class: "muted", text: "Shown under the Connect button" })),
    linksEl, h("div", { class: "field" }), addLink);

  // ---- Phone preview
  const phoneTitle = h("div", { class: "phone-title" });
  const phoneState = h("div", { class: "phone-state" });
  const phoneLinks = h("div", { class: "phone-links" });
  const arch = document.createElementNS(SVG_NS, "svg");
  arch.setAttribute("viewBox", "0 0 110 84");
  arch.setAttribute("class", "phone-arch");
  for (const d of ["M8 82V46a47 47 0 0 1 94 0v36", "M24 82V48a31 31 0 0 1 62 0v34", "M40 82V50a15 15 0 0 1 30 0v32"]) {
    const p = document.createElementNS(SVG_NS, "path");
    p.setAttribute("d", d);
    arch.append(p);
  }
  function updatePreview() {
    phoneTitle.textContent = s.name || "Configuration name";
    phoneState.textContent = s.twofa === "totp" ? "Not connected · code generated on the phone" : s.twofa === "manual" ? "Not connected · asks for a code" : "Not connected";
    phoneLinks.replaceChildren(...s.links.map((l) =>
      h("div", { class: "phone-link" }, icon(l.kind === "rdp" ? "monitor" : l.kind === "app" ? "app" : "globe"), h("span", { text: l.title || "Untitled link" }))));
  }
  const previewEl = h("aside", { class: "preview" },
    h("div", { class: "phone" }, h("div", { class: "phone-screen" }, phoneTitle, arch, phoneState, phoneLinks,
      h("div", { class: "phone-connect" }, h("div", { text: "Connect" })))),
    h("div", { class: "phone-caption", text: "What the user sees after scanning" }));

  // ---- Save / open package files
  const openInput = h("input", { type: "file", accept: ".json,application/json", class: "hidden-input" });
  openInput.addEventListener("change", async () => {
    const f = openInput.files[0];
    if (!f) return;
    try {
      const data = JSON.parse(await f.text());
      if (data && data.tunnelkey === "setup-file") {
        toast("That's an encrypted desktop setup file — open it in Tunnelkey on the computer.");
        return;
      }
      s = fromPackage(data);
      renderEditor();
      toast("Package loaded");
    } catch {
      toast("That isn't a Tunnelkey package file.");
    }
  });
  const toolbar = h("div", { class: "toolbar no-print" },
    h("button", {
      class: "btn ghost small", type: "button",
      onclick: () => {
        if (!confirm("The file contains the profile, password and 2FA secret in plain text. Store it somewhere safe. Continue?")) return;
        download(`${slug(s.name)}.tunnelkey.json`, new Blob([JSON.stringify(toPackage(), null, 2)], { type: "application/json" }));
      },
    }, icon("save"), "Save package file"),
    h("button", { class: "btn ghost small", type: "button", onclick: () => openInput.click() }, icon("open"), "Open package file"),
    h("button", {
      class: "btn ghost small", type: "button",
      onclick: () => { if (confirm("Clear the form?")) { s = emptyState(); renderEditor(); } },
    }, icon("trash"), "Clear"),
    openInput);

  // ---- Create
  const status = h("div", { class: "status muted" });
  const create = h("button", { class: "btn primary", type: "submit" }, icon("qr"), "Create setup codes");
  const form = h("form", {
    onsubmit: async (e) => {
      e.preventDefault();
      status.replaceChildren();
      if (s.twofa === "totp" && !s.totp.secret) {
        status.replaceChildren(banner("error", "Enter the TOTP secret, or choose another 2FA option."));
        return;
      }
      create.disabled = true;
      try {
        const payload = buildPayload(toPackage());
        const codes = await encodeSetupCodes(payload);
        // Self-check before showing anything.
        await decodeSetupCodes(codes.map((c) => c.text));
        renderCodes(codes, payload);
      } catch (err) {
        status.replaceChildren(banner("error", err.message));
        create.disabled = false;
      }
    },
  },
    profileCard, authCard, noAuthNote, linksCard,
    h("div", { class: "savebar" }, status, create));

  app.replaceChildren(
    h("div", { class: "page-head" },
      h("div", {},
        h("div", { class: "eyebrow", text: "Provision a phone" }),
        h("h1", { text: "Create a setup code" }),
        h("p", { class: "lede", text: "Put a VPN profile, its sign-in, the 2FA secret and your links onto a phone with one scan of the Tunnelkey app." })),
      toolbar),
    h("div", { class: "card offline-note" }, icon("shield"),
      h("div", {},
        h("strong", { text: "Runs only in this browser. " }),
        h("span", { class: "muted", text: "Nothing you enter is uploaded or stored — this page has no server and is not allowed to make network requests. Close the tab and it's gone." }))),
    h("div", { class: "editor" }, form, previewEl),
    footer());

  refreshProfile();
  refreshTwofa();
  renderLinks();
  if (!s.name) nameInput.focus();
}

// ---------------------------------------------------------------- codes

function renderQr(text, ecc) {
  const qr = qrcode(0, ecc);
  qr.addData(text, "Alphanumeric");
  qr.make();
  const n = qr.getModuleCount();
  const quiet = 4;
  const scale = Math.max(4, Math.floor(900 / (n + quiet * 2)));
  const size = (n + quiet * 2) * scale;
  const canvas = h("canvas", { width: size, height: size });
  const ctx = canvas.getContext("2d");
  ctx.fillStyle = "#fff";
  ctx.fillRect(0, 0, size, size);
  ctx.fillStyle = "#000";
  for (let r = 0; r < n; r++) {
    for (let c = 0; c < n; c++) {
      if (qr.isDark(r, c)) ctx.fillRect((c + quiet) * scale, (r + quiet) * scale, scale, scale);
    }
  }
  return canvas;
}

function renderCodes(codes, payload) {
  runCleanup();
  const multi = codes.length > 1;
  const canvases = codes.map((c) => renderQr(c.text, c.ecc));

  const grid = h("div", { class: "codes" }, canvases.map((cv, i) =>
    h("figure", { class: "code" }, cv, multi && h("figcaption", { class: "code-label", text: `${i + 1} / ${codes.length}` }))));

  function cycle() {
    let i = 0;
    const img = h("img", { alt: "" });
    const label = h("div", { class: "cycle-label" });
    const show = () => {
      img.src = canvases[i].toDataURL("image/png");
      img.alt = `Setup code ${i + 1} of ${codes.length}`;
      label.textContent = multi ? `${i + 1} / ${codes.length}` : "";
    };
    const close = () => { clearInterval(timer); document.removeEventListener("keydown", onKey); overlay.remove(); };
    const onKey = (e) => { if (e.key === "Escape") close(); };
    const overlay = h("div", { class: "cycle", role: "dialog", "aria-label": "Setup codes", onclick: close },
      label, img, h("button", { class: "btn cycle-close", type: "button", text: "Close", onclick: close }));
    show();
    const timer = multi ? setInterval(() => { i = (i + 1) % codes.length; show(); }, 1400) : null;
    document.addEventListener("keydown", onKey);
    document.body.append(overlay);
    cleanup.push(close);
  }

  function downloadPngs() {
    canvases.forEach((cv, i) => cv.toBlob((blob) => {
      download(`${slug(payload.n)}${multi ? `-${i + 1}-of-${codes.length}` : ""}.png`, blob);
    }, "image/png"));
  }

  const hasSecret = !!payload.t || !!payload.p;
  app.replaceChildren(
    h("div", { class: "page-head" },
      h("div", {},
        h("button", { class: "btn ghost small no-print", type: "button", onclick: renderEditor }, icon("back"), "Edit"),
        h("div", { class: "eyebrow", text: multi ? `${codes.length} setup codes` : "Setup code" }),
        h("h1", { text: payload.n })),
      h("div", { class: "package-actions no-print" },
        h("button", { class: "btn primary", type: "button", onclick: cycle }, icon("play"), multi ? "Full screen (cycles)" : "Full screen"),
        h("button", { class: "btn ghost", type: "button", onclick: () => window.print() }, icon("print"), "Print"),
        h("button", { class: "btn ghost", type: "button", onclick: downloadPngs }, icon("download"), multi ? "Download PNGs" : "Download PNG"))),
    h("div", { class: "codes-wrap" },
      grid,
      h("aside", { class: "no-print" },
        h("div", { class: "card" },
          h("h3", { text: "On the phone" }),
          h("ol", { class: "steps-list" },
            h("li", { text: "Open Tunnelkey, tap + and choose “Scan setup code”." }),
            h("li", { text: multi ? `Point the camera at each code — any order. The app shows which of the ${codes.length} are still missing.` : "Point the camera at the code." }),
            h("li", { text: hasSecret ? "Protect it with fingerprint / face or an 8-digit PIN (required, because secrets are stored)." : "Optionally protect it with fingerprint / face or a PIN." }),
            h("li", { text: "Tap Connect." }))),
        desktopCard(payload.n, async () => payload),
        h("div", { class: "card" },
          banner("warn", "These codes are not encrypted. Anyone who sees or photographs them gets this VPN access" + (payload.t ? ", including the 2FA secret." : ".")),
          h("div", { class: "field" }),
          h("p", { class: "muted", text: "Show them only to their owner. Downloaded images and printouts contain the same secrets — delete or shred them once the phone is set up." })))),
    footer());
}

// ---------------------------------------------------------------- desktop setup file

/** Card for Tunnelkey on Windows, macOS and Linux: an encrypted setup file,
 *  encrypted in this browser. getPayload() resolves to the setup-code payload. */
function desktopCard(name, getPayload) {
  const min = SETUP_FILE_MIN_PASSWORD;
  const pass = h("input", {
    type: "password", id: "file-pass", class: "mono", autocomplete: "new-password", spellcheck: false,
    placeholder: `At least ${min} characters`, oninput: () => { generated.hidden = true; refresh(); },
  });
  const toggle = h("button", { class: "icon-btn", type: "button", onclick: () => setShown(pass.type === "password") });
  const generate = h("button", {
    class: "btn ghost small", type: "button",
    onclick: () => {
      pass.value = generatePassphrase();
      generated.textContent = pass.value; // full width, so it can be read out or copied
      generated.hidden = false;
      setShown(true);
      refresh();
      pass.select();
    },
  }, icon("key"), "Generate");
  const copy = h("button", {
    class: "btn ghost small", type: "button",
    onclick: async () => {
      try {
        await navigator.clipboard.writeText(pass.value);
        toast("Password copied");
      } catch {
        setShown(true);
        pass.select();
        toast("Press Ctrl+C (⌘C) to copy");
      }
    },
  }, icon("copy"), "Copy");
  const generated = h("div", { class: "passphrase mono", hidden: true });
  const count = h("div", { class: "hint" });
  const status = h("div");
  const save = h("button", { class: "btn primary block", type: "button", onclick: saveFile }, icon("download"), "Download setup file");

  function setShown(show) {
    pass.type = show ? "text" : "password";
    const label = show ? "Hide password" : "Show password";
    toggle.title = label;
    toggle.setAttribute("aria-label", label);
    toggle.setAttribute("aria-pressed", String(show));
    toggle.replaceChildren(icon(show ? "eyeOff" : "eye"));
  }

  function refresh() {
    const n = [...pass.value.normalize("NFC")].length;
    save.disabled = n < min;
    copy.disabled = !pass.value;
    count.textContent = n < min ? `${min - n} more character${min - n === 1 ? "" : "s"} needed.` : "Long enough.";
  }

  async function saveFile() {
    save.disabled = true;
    status.replaceChildren(h("p", { class: "muted", text: "Encrypting…" }));
    try {
      const text = await encryptSetupFile(await getPayload(), pass.value);
      download(`${slug(name)}.tunnelkey`, new Blob([text], { type: SETUP_FILE_TYPE }));
      status.replaceChildren();
    } catch (err) {
      status.replaceChildren(h("div", { class: "field" }, banner("error", err.message)));
    }
    refresh();
  }

  setShown(false);
  refresh();
  return h("div", { class: "card desktop" },
    h("h3", { text: "Desktop (Windows, macOS, Linux)" }),
    h("p", { class: "muted", text: "Download an encrypted setup file and open it with Tunnelkey on the computer. It asks for this password." }),
    h("div", { class: "field" },
      h("label", { for: "file-pass", text: "File password" }),
      h("div", { class: "pass-wrap" }, pass, toggle),
      generated,
      count,
      h("div", { class: "pass-actions" }, generate, copy)),
    status,
    save,
    h("p", { class: "hint", text: "Send the file and the password through different channels." }));
}

function footer() {
  return h("p", { class: "footer no-print" },
    "Tunnelkey · open source (AGPL-3.0) · ",
    h("a", { href: "https://github.com/Pro-IT-Services/TunnelKey", rel: "noopener", text: "GitHub" }),
    " · ",
    h("a", { href: "../privacy-policy.html", text: "Privacy policy" }));
}

renderEditor();
