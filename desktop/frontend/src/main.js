import "./style.css";
// Development preview without the Go backend: /?mock=<scenario> (see dev/mock.js).
if (import.meta.env.DEV && new URLSearchParams(location.search).has("mock")) await import("../dev/mock.js");
import { call, on, clipboardText, copyText } from "./api.js";
import { h, icon, iconButton, logo, toast } from "./dom.js";
import { setLanguage, t, has } from "./i18n.js";
import { statusHero } from "./hero.js";

const root = document.getElementById("app");
const PRIVACY_URL = "https://kalipsers.github.io/TunnelKey/privacy-policy.html";
const LOCK_AFTER_HIDDEN_MS = 30_000;

let state = null;
let page = { name: "home" };
let live = []; // update functions of the visible page, run on status changes
let sheet = null; // open modal
let selectedId = localStorage.getItem("selectedProfile") || "";
let pendingLink = null; // link to open once connected
let retryTimer;

// ---- Boot -----------------------------------------------------------------

async function refresh() {
  try {
    state = await call("State");
  } catch (e) {
    state = { initError: String(e), status: { phase: "disconnected" }, profiles: [] };
  }
  setLanguage(state.language);
  render();
}

on("state", refresh);
on("status", (s) => {
  if (!state) return;
  const prev = state.status?.phase;
  state.status = s;
  if (s.phase === "connected" && prev !== "connected" && pendingLink != null) {
    const i = pendingLink;
    pendingLink = null;
    openLink(i);
  }
  if (s.phase === "failed" || s.phase === "disconnected") pendingLink = null;
  updateLive();
  // The helper may ask the app to re-render (e.g. auto retry banner).
  if (prev !== s.phase && (s.phase === "failed" || prev === "failed")) refresh();
});
on("log", (lines) => document.dispatchEvent(new CustomEvent("tk-log", { detail: lines })));
// While locked, the app answers the sign-in itself after unlocking.
on("need-creds", () => { if (!state?.locked) openSignIn({ reconnect: true }); });
on("open-file", (path) => importPath(path));
on("error", (code) => toast(errorText(code), "error"));

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape" && sheet) closeSheet();
});

// Lock the provisioned secrets when the window stays hidden, like the phone app.
let hiddenTimer;
document.addEventListener("visibilitychange", () => {
  clearTimeout(hiddenTimer);
  if (document.hidden) hiddenTimer = setTimeout(() => call("Lock").catch(() => {}), LOCK_AFTER_HIDDEN_MS);
});

refresh().then(async () => {
  const files = await call("TakePendingFiles").catch(() => []);
  for (const f of files || []) importPath(f);
});

// ---- Rendering ----------------------------------------------------------------

function go(name, data = {}) {
  page = { name, ...data };
  render();
}

function render() {
  live = [];
  clearInterval(retryTimer);
  let view;
  if (!state) return;
  if (state.initError) view = fatalView(state.initError);
  else if (state.managed && state.locked && !page.name.startsWith("setup")) view = lockView();
  else {
    switch (page.name) {
      case "editor": view = editorView(); break;
      case "logs": view = logsView(); break;
      case "about": view = aboutView(); break;
      case "security": view = securityView(); break;
      case "setup-open": view = setupOpenView(); break;
      case "setup-summary": view = setupSummaryView(); break;
      case "setup-pin": view = pinCreateView(); break;
      default: view = state.managed ? managedView() : homeView();
    }
  }
  root.replaceChildren(view);
  updateLive();
  const focus = root.querySelector("[autofocus]");
  if (focus) focus.focus();
}

function updateLive() {
  for (const fn of live) fn(state.status || {});
}

function errorText(code) {
  const msg = String(code?.message ?? code ?? "");
  if (msg.startsWith("profile:")) return t("error_profile", msg.slice(8).trim());
  if (has(msg)) return t(msg);
  return t("unexpected", msg);
}

// ---- Shared pieces ---------------------------------------------------------------

function topBar(title, actions, back) {
  return h(
    "header.topbar",
    back ? iconButton("back", t("action_back"), back) : logo(28),
    h("h1.topbar-title", { title }, title),
    h("div.topbar-actions", actions),
  );
}

function helperBanner() {
  if (state.helperUp) return null;
  return h(
    "div.banner.banner-warn",
    { role: "alert" },
    icon("alert"),
    h("div", h("strong", t("helper_title")), h("p", t("helper_body_" + platformKey()))),
  );
}

function platformKey() {
  return ["windows", "darwin", "linux"].includes(state.platform) ? state.platform : "linux";
}

function failureText(status, twoFactor) {
  switch (status.failure) {
    case "auth_failed":
      return status.message || t(twoFactor ? "error_auth_failed_2fa" : "error_auth_failed");
    case "needs_sign_in":
      return t(twoFactor ? "error_need_creds_2fa" : "error_need_creds");
    case "timeout":
      return t("error_timeout");
    case "profile":
      return t("error_profile", status.message || "");
    case "no_engine":
      return t("error_no_engine_" + platformKey());
    default:
      return status.message || "";
  }
}

/** Failure card + automatic retry notice, kept current by live updates. */
function statusNotes(twoFactorOf) {
  const box = h("div.notes");
  live.push((st) => {
    box.replaceChildren();
    if (state.retryIn > 0) {
      let left = state.retryIn;
      const note = h("div.banner.banner-info", icon("info"), h("p", t("retry_next_code", left)));
      box.append(note);
      clearInterval(retryTimer);
      retryTimer = setInterval(() => {
        left = Math.max(0, left - 1);
        note.querySelector("p").textContent = t("retry_next_code", left);
      }, 1000);
      return;
    }
    if (st.phase === "failed") {
      const text = failureText(st, twoFactorOf());
      if (text) box.append(h("div.banner.banner-error", { role: "alert" }, icon("alert"), h("p", text)));
    }
  });
  return box;
}

function heroBlock(name) {
  const hero = statusHero();
  live.push((st) => hero.update(st, name()));
  return hero.el;
}

function connectBar(onConnect, canConnect) {
  const bar = h("footer.connect-bar");
  live.push((st) => {
    const phase = st.phase || "disconnected";
    let btn;
    if (phase === "connected") btn = h("button.btn.btn-outline.btn-wide", { onclick: disconnect }, t("action_disconnect"));
    else if (phase === "connecting" || phase === "reconnecting")
      btn = h("button.btn.btn-outline.btn-wide", { onclick: disconnect }, t("action_cancel"));
    else if (phase === "disconnecting")
      // Still clickable: sends Disconnect again.
      btn = h("button.btn.btn-outline.btn-wide", { onclick: disconnect }, t("status_disconnecting"));
    else if (state.retryIn > 0) btn = h("button.btn.btn-outline.btn-wide", { onclick: disconnect }, t("action_cancel"));
    else btn = h("button.btn.btn-primary.btn-wide", { onclick: onConnect, disabled: !canConnect() || !state.helperUp }, t("action_connect"));
    bar.replaceChildren(btn);
  });
  return bar;
}

async function disconnect() {
  pendingLink = null;
  try {
    await call("Disconnect");
  } catch (e) {
    toast(errorText(e), "error");
  }
}

function menu(items) {
  const btn = iconButton("more", t("action_menu"), (e) => {
    e.stopPropagation();
    const existing = document.querySelector(".menu");
    if (existing) return existing.remove();
    const m = h(
      "div.menu",
      { role: "menu" },
      items.filter(Boolean).map((it) =>
        it === "-"
          ? h("div.menu-sep")
          : h(
              "button.menu-item" + (it.danger ? ".danger" : ""),
              { role: "menuitem", type: "button", onclick: () => { m.remove(); it.action(); } },
              icon(it.icon, 18),
              h("span", it.label),
            ),
      ),
    );
    document.body.append(m);
    const r = btn.getBoundingClientRect();
    m.style.top = `${r.bottom + 6}px`;
    m.style.right = `${document.documentElement.clientWidth - r.right}px`;
    m.querySelector("button")?.focus();
    setTimeout(() => document.addEventListener("click", () => m.remove(), { once: true }));
  });
  return btn;
}

function languageItem() {
  return {
    icon: "language",
    label: `${t("action_language")}: ${state.language === "sk" ? "Slovenčina" : state.language === "en" ? "English" : t("lang_auto")}`,
    action: () =>
      openSheet((close) => [
        h("h2.sheet-title", t("action_language")),
        [["", t("lang_auto")], ["en", "English"], ["sk", "Slovenčina"]].map(([code, label]) =>
          h(
            "button.choice" + ((state.language || "") === code ? ".selected" : ""),
            { type: "button", onclick: async () => { await call("SetLanguage", code); close(); refresh(); } },
            h("span.choice-title", label),
            (state.language || "") === code ? icon("check") : null,
          ),
        ),
      ]),
  };
}

// ---- Sheets & dialogs ----------------------------------------------------------------

function openSheet(build) {
  closeSheet();
  const panel = h("div.sheet", { role: "dialog", "aria-modal": "true" });
  const backdrop = h("div.backdrop", { onclick: (e) => e.target === backdrop && closeSheet() }, panel);
  const close = () => closeSheet();
  panel.append(...[build(close)].flat(Infinity).filter(Boolean));
  document.body.append(backdrop);
  sheet = backdrop;
  requestAnimationFrame(() => backdrop.classList.add("open"));
  (panel.querySelector("[autofocus]") || panel.querySelector("input,button"))?.focus();
  return close;
}

function closeSheet() {
  if (!sheet) return;
  const s = sheet;
  sheet = null;
  s.classList.remove("open");
  setTimeout(() => s.remove(), 160);
}

function confirmDialog(title, body, action, danger = true) {
  return new Promise((resolve) => {
    let answered = false;
    const done = (v, close) => { answered = true; close(); resolve(v); };
    openSheet((close) => [
      h("h2.sheet-title", title),
      h("p.sheet-body", body),
      h(
        "div.sheet-actions",
        h("button.btn.btn-text", { type: "button", onclick: () => done(false, close) }, t("action_cancel")),
        h("button.btn" + (danger ? ".btn-danger" : ".btn-primary"), { type: "button", autofocus: true, onclick: () => done(true, close) }, action),
      ),
    ]);
    const obs = new MutationObserver(() => {
      if (!document.body.contains(sheet) && !answered) { obs.disconnect(); resolve(false); }
    });
    obs.observe(document.body, { childList: true });
  });
}

function field(label, input, hint) {
  const id = "f" + Math.random().toString(36).slice(2, 8);
  input.id = id;
  return h("div.field", h("label", { for: id }, label), input, hint ? h("p.hint", hint) : null);
}

function passwordInput(attrs = {}) {
  const input = h("input.input", { type: "password", autocomplete: "off", spellcheck: "false", ...attrs });
  const toggle = iconButton("eye", t("action_show_password"), () => {
    const show = input.type === "password";
    input.type = show ? "text" : "password";
    toggle.replaceChildren(icon(show ? "eyeOff" : "eye"));
    toggle.setAttribute("aria-label", t(show ? "action_hide_password" : "action_show_password"));
  });
  const wrap = h("div.input-wrap", input, toggle);
  wrap.input = input;
  return wrap;
}

function toggleRow(label, body, checked, onchange) {
  const id = "t" + Math.random().toString(36).slice(2, 8);
  const input = h("input", { type: "checkbox", id, role: "switch", checked, onchange: (e) => onchange(e.target.checked) });
  return h("label.toggle-row", { for: id }, h("div", h("span.toggle-label", label), body ? h("p.hint", body) : null), h("span.switch", input, h("span.slider")));
}

function segmented(options, value, onchange) {
  const group = h("div.segmented", { role: "radiogroup" });
  for (const [v, label] of options) {
    group.append(
      h("button" + (v === value ? ".active" : ""), {
        type: "button", role: "radio", "aria-checked": String(v === value),
        onclick: () => {
          group.querySelectorAll("button").forEach((b) => { b.classList.remove("active"); b.setAttribute("aria-checked", "false"); });
          const b = group.children[options.findIndex((o) => o[0] === v)];
          b.classList.add("active");
          b.setAttribute("aria-checked", "true");
          onchange(v);
        },
      }, label),
    );
  }
  return group;
}

// ---- Home (profiles) ---------------------------------------------------------------

function selectedProfile() {
  const list = state.profiles || [];
  return list.find((p) => p.id === selectedId) || list[0] || null;
}

function homeView() {
  const list = state.profiles || [];
  const selected = selectedProfile();
  const content = h(
    "main.content",
    helperBanner(),
    state.weakKeyStorage ? h("div.banner.banner-info", icon("info"), h("p", t("weak_key_storage"))) : null,
    heroBlock(() => selectedProfile()?.name || ""),
    statusNotes(() => !!selectedProfile()?.twoFactor),
    list.length ? profileList(list, selected) : emptyState(),
  );
  return h(
    "div.screen",
    topBar(t("app_name"), [
      iconButton("plus", t("action_import"), openImportSheet),
      menu([
        { icon: "plus", label: t("action_import"), action: openImportSheet },
        { icon: "log", label: t("action_logs"), action: () => go("logs") },
        languageItem(),
        { icon: "info", label: t("action_about"), action: () => go("about") },
      ]),
    ]),
    content,
    connectBar(() => connectProfile(selectedProfile()), () => !!selectedProfile()),
  );
}

function profileList(list, selected) {
  const busy = () => !["disconnected", "failed"].includes(state.status?.phase || "disconnected");
  const items = list.map((p) => {
    const row = h(
      "div.profile" + (p.id === selected?.id ? ".selected" : ""),
      {
        role: "radio", tabindex: "0", "aria-checked": String(p.id === selected?.id),
        onclick: () => select(p),
        onkeydown: (e) => (e.key === " " || e.key === "Enter") && (e.preventDefault(), select(p)),
      },
      h("span.radio"),
      h(
        "div.profile-text",
        h("span.profile-name", p.name),
        h("span.profile-meta", p.remote, p.twoFactor ? h("span.badge", t("badge_2fa")) : null, p.hasSavedPassword ? h("span.badge.badge-quiet", t("badge_saved")) : null),
      ),
      iconButton("edit", t("action_edit"), (e) => { e.stopPropagation(); go("editor", { profileId: p.id }); }),
    );
    return row;
  });
  function select(p) {
    if (busy() && p.id !== selected?.id) return; // switch profiles when disconnected
    selectedId = p.id;
    localStorage.setItem("selectedProfile", p.id);
    render();
  }
  return h("section.section", h("h3.section-title", t("profiles_header")), h("div.profiles", { role: "radiogroup", "aria-label": t("profiles_header") }, items));
}

function emptyState() {
  return h(
    "section.empty",
    h("h3", t("empty_title")),
    h("p", t("empty_body")),
    h("div.empty-actions",
      h("button.btn.btn-primary", { onclick: chooseFile }, icon("file", 18), t("import_choose_file")),
      h("button.btn.btn-outline", { onclick: pasteProfile }, icon("clipboard", 18), t("import_paste")),
    ),
    h("p.hint", t("drop_hint")),
  );
}

function openImportSheet() {
  openSheet((close) => [
    h("h2.sheet-title", t("import_sheet_title")),
    h("button.choice", { type: "button", autofocus: true, onclick: () => { close(); chooseFile(); } },
      icon("file"), h("div", h("span.choice-title", t("import_choose_file")), h("span.choice-body", t("import_choose_file_body")))),
    h("button.choice", { type: "button", onclick: () => { close(); pasteProfile(); } },
      icon("clipboard"), h("div", h("span.choice-title", t("import_paste")), h("span.choice-body", t("import_paste_body")))),
    h("p.hint.center", t("drop_hint")),
  ]);
}

async function chooseFile() {
  handleImport(await call("ChooseFile").catch((e) => ({ kind: "error", error: String(e) })));
}

async function importPath(path) {
  handleImport(await call("ImportPath", path).catch((e) => ({ kind: "error", error: String(e) })));
}

async function pasteProfile() {
  const text = await clipboardText();
  if (!text || !/\bremote\s/.test(text)) {
    if (text && text.includes('"tunnelkey"')) return toast(t("import_paste_setup"), "error");
    return toast(t("import_clipboard_empty"), "error");
  }
  handleImport(await call("ImportText", text).catch((e) => ({ kind: "error", error: String(e) })));
}

function handleImport(r) {
  if (!r || r.kind === "cancelled") return;
  if (r.kind === "setup") return go("setup-open", { path: r.path });
  if (r.kind === "ovpn") return go("editor", { draft: r.draft });
  const known = { file_too_large: "import_too_large", file_unreadable: "import_unreadable", paste_setup_file: "import_paste_setup" };
  toast(known[r.error] ? t(known[r.error]) : t("import_refused", r.error), "error");
}

// ---- Connecting -----------------------------------------------------------------------

function connectProfile(p) {
  if (!p) return;
  if (p.needsCredentials && !(p.hasSavedPassword && !p.twoFactor)) return openSignIn({ profile: p });
  call("Connect", p.id, {}).catch((e) => toast(errorText(e), "error"));
}

function connectManaged() {
  const m = state.managed;
  const needs = m.needsCredentials && (m.needsUser || !m.hasPassword || m.manualCode);
  if (needs) return openSignIn({ managed: m });
  call("ConnectManaged", {}).catch((e) => toast(errorText(e), "error"));
}

/** Sign-in sheet for profiles, provisioned configs and reconnect requests. */
function openSignIn({ profile, managed, reconnect }) {
  if (reconnect) {
    managed = state.managed && !profile ? state.managed : null;
    profile = managed ? null : selectedProfile();
  }
  const p = profile;
  const m = managed;
  const name = m ? m.name : p?.name || "";
  const needUser = m ? m.needsUser : !p?.username;
  const needPassword = m ? !m.hasPassword : true;
  const twoFactor = m ? m.manualCode : p?.twoFactor;
  const codeLength = m ? m.codeLength : p?.codeLength || 6;
  const staticChallenge = !m && p?.staticChallenge;
  const saved = !m && p?.hasSavedPassword;

  const user = h("input.input", { type: "text", autocomplete: "username", spellcheck: "false" });
  const pass = passwordInput({ placeholder: saved ? t("password_saved_hint") : "" });
  const code = h("input.input.code-input", { type: "text", inputmode: "numeric", maxlength: codeLength, autocomplete: "one-time-code", placeholder: "•".repeat(codeLength), "aria-label": t("field_code") });
  let remember = !!saved;
  const err = h("p.form-error", { role: "alert" });
  const preview = h("p.preview");
  const updatePreview = () => {
    if (!twoFactor || staticChallenge || m) return;
    const pw = "•".repeat(Math.min(pass.input.value.length || (saved ? 8 : 0), 12));
    const c = code.value.replace(/\D/g, "");
    preview.replaceChildren(t("signin_sent_as") + ": ", h("code", p.codeAfter ? [pw, h("mark", c)] : [h("mark", c), pw]));
  };
  code.addEventListener("input", () => { code.value = code.value.replace(/\D/g, "").slice(0, codeLength); updatePreview(); });
  pass.input.addEventListener("input", updatePreview);

  async function submit(e, close) {
    e.preventDefault();
    err.textContent = "";
    const input = { username: user.value.trim(), password: pass.input.value, code: code.value, remember };
    if (twoFactor && input.code.length !== codeLength) return (err.textContent = t("code_invalid"));
    try {
      if (reconnect) await call("ProvideCredentials", input);
      else if (m) await call("ConnectManaged", input);
      else await call("Connect", p.id, input);
      close();
    } catch (x) {
      err.textContent = errorText(x);
    }
  }

  openSheet((close) => {
    const form = h("form.form", { onsubmit: (e) => submit(e, close) },
      h("h2.sheet-title", t("signin_title", name)),
      reconnect ? h("p.sheet-body", t("signin_reconnect")) : null,
      !needUser && (p?.username) ? h("p.hint", t("signin_user", p.username)) : null,
      needUser ? field(t("field_username"), user) : null,
      needPassword ? field(t("field_password"), pass) : null,
      twoFactor ? field(t("field_code"), code, staticChallenge ? t("signin_static_challenge") : t("signin_code_hint", codeLength)) : null,
      preview,
      !m && !reconnect && needPassword
        ? toggleRow(t("toggle_remember"), t("toggle_remember_body", platformAccount()), remember, (v) => (remember = v))
        : null,
      err,
      h("div.sheet-actions",
        h("button.btn.btn-text", { type: "button", onclick: close }, t("action_cancel")),
        h("button.btn.btn-primary", { type: "submit" }, t("action_connect")),
      ),
    );
    return form;
  });
  const first = (needUser && user) || (needPassword && pass.input) || (twoFactor && code);
  first?.focus();
  updatePreview();
}

function platformAccount() {
  return { windows: "Windows", darwin: "macOS", linux: "Linux" }[state.platform] || "";
}

// ---- Provisioned (single-config) mode -----------------------------------------------------

function managedView() {
  const m = state.managed;
  const linksBox = m.links.length
    ? h("section.section",
        h("h3.section-title", t("links_header")),
        h("div.links", m.links.map((l, i) =>
          h("button.link", { type: "button", onclick: () => onLink(i) },
            h("span.link-icon", icon(l.k === "rdp" ? "monitor" : l.k === "app" ? "app" : "globe")),
            h("div.link-text", h("span.link-title", l.t), h("span.link-meta", t("link_kind_" + (l.k || "web")) + " · " + linkHost(l))),
            icon("chevron", 18),
          ),
        )),
      )
    : null;
  const lockItems = m.lockMethod === "pin" || m.lockMethod === "hello";
  return h(
    "div.screen",
    topBar(m.name, [
      menu([
        { icon: "log", label: t("action_logs"), action: () => go("logs") },
        lockItems ? { icon: "shield", label: t("action_settings"), action: () => go("security") } : null,
        lockItems ? { icon: "lock", label: t("action_lock_now"), action: () => call("Lock") } : null,
        languageItem(),
        { icon: "info", label: t("action_about"), action: () => go("about") },
        "-",
        { icon: "trash", label: t("action_remove_config"), danger: true, action: removeManaged },
      ]),
    ]),
    h("main.content", helperBanner(), heroBlock(() => m.remote.split(":")[0] || ""), statusNotes(() => m.hasTotp || m.manualCode), linksBox),
    connectBar(connectManaged, () => true),
  );
}

function linkHost(l) {
  if (l.k === "rdp") {
    const mm = /full%20address=s:([^&]+)/i.exec(l.u);
    return mm ? decodeURIComponent(mm[1]) : "";
  }
  try {
    return new URL(l.u).host || l.u;
  } catch {
    return l.u;
  }
}

function onLink(i) {
  const phase = state.status?.phase;
  if (phase === "connected") return openLink(i);
  // Links usually point inside the VPN: connect first, open once it's up.
  pendingLink = i;
  toast(t("link_opens_after_connect", state.managed.links[i].t));
  if (phase !== "connecting" && phase !== "reconnecting") connectManaged();
}

function openLink(i) {
  call("OpenLink", i).catch((e) => toast(errorText(e), "error"));
}

async function removeManaged() {
  const m = state.managed;
  if (await confirmDialog(t("remove_confirm_title", m.name), t("remove_confirm_body"), t("action_remove"))) {
    await call("RemoveManaged").catch((e) => toast(errorText(e), "error"));
    go("home");
  }
}

// ---- PIN pad -------------------------------------------------------------------------------

function pinPad({ onComplete, disabled = () => false }) {
  let pin = "";
  const dots = h("div.pin-dots", { role: "img" });
  const draw = () => {
    dots.replaceChildren(...Array.from({ length: 8 }, (_, i) => h("span.dot" + (i < pin.length ? ".filled" : ""))));
    dots.setAttribute("aria-label", t("pin_dots", pin.length, 8));
  };
  const press = (d) => {
    if (disabled() || pin.length >= 8) return;
    pin += d;
    draw();
    if (pin.length === 8) {
      const value = pin;
      setTimeout(() => onComplete(value), 120);
    }
  };
  const del = () => { pin = pin.slice(0, -1); draw(); };
  const keys = ["1", "2", "3", "4", "5", "6", "7", "8", "9", "", "0", "del"].map((k) =>
    k === "" ? h("span") :
    k === "del" ? h("button.key.key-quiet", { type: "button", "aria-label": t("action_delete_digit"), onclick: del }, icon("backspace", 22)) :
    h("button.key", { type: "button", onclick: () => press(k) }, k),
  );
  const pad = h("div.pin", { tabindex: "0", autofocus: true,
    onkeydown: (e) => {
      if (/^[0-9]$/.test(e.key)) press(e.key);
      else if (e.key === "Backspace") del();
      else return;
      e.preventDefault();
    },
  }, dots, h("div.keys", keys));
  pad.reset = () => { pin = ""; draw(); };
  draw();
  return pad;
}

// ---- Lock screen ------------------------------------------------------------------------------

let helloAutoTried = false;

function lockView() {
  const m = state.managed;
  const msg = h("p.form-error.center", { role: "alert" });
  const body = h("div.lock-body");
  let lockedUntil = 0;
  let countdown;

  const showWait = () => {
    clearInterval(countdown);
    const tick = () => {
      const left = Math.ceil((lockedUntil - Date.now()) / 1000);
      if (left <= 0) { clearInterval(countdown); msg.textContent = ""; lockedUntil = 0; return; }
      const mm = Math.floor(left / 60), ss = left % 60;
      msg.textContent = t("lock_wait", mm ? `${mm}:${String(ss).padStart(2, "0")}` : `${ss} s`);
    };
    tick();
    countdown = setInterval(tick, 1000);
  };

  if (m.lockMethod === "hello") {
    const unlock = async () => {
      msg.textContent = "";
      try {
        await call("UnlockWithHello");
      } catch (e) {
        msg.textContent = errorText(e);
      }
    };
    body.append(h("button.btn.btn-primary.btn-wide", { onclick: unlock, autofocus: true }, icon("face", 20), t("lock_use_hello")));
    if (!helloAutoTried) { helloAutoTried = true; setTimeout(unlock, 300); }
  } else {
    const pad = pinPad({
      disabled: () => Date.now() < lockedUntil,
      onComplete: async (pin) => {
        const r = await call("UnlockWithPin", pin).catch((e) => { msg.textContent = errorText(e); return null; });
        pad.reset();
        if (!r || r.ok) return;
        if (r.wiped) { toast(t("lock_wiped"), "error"); return; }
        msg.textContent = t("lock_wrong_pin", r.attemptsLeft);
        if (r.lockedUntil) { lockedUntil = r.lockedUntil; showWait(); }
      },
    });
    body.append(h("p.lock-prompt", t("lock_enter_pin")), pad);
    call("PinStatus").then((r) => { if (r.lockedUntil) { lockedUntil = r.lockedUntil; showWait(); } }).catch(() => {});
  }
  return h("div.screen.lock",
    h("div.lock-head", logo(56), h("h1", t("lock_title")), h("p.muted", m.name)),
    body, msg);
}

// ---- Setup file flow -------------------------------------------------------------------------

let setupSummary = null;

function setupOpenView() {
  const pass = passwordInput({ autofocus: true, autocomplete: "off" });
  const err = h("p.form-error", { role: "alert" });
  const btn = h("button.btn.btn-primary.btn-wide", { type: "submit" }, t("setup_open"));
  const form = h("form.form.page-form", {
    onsubmit: async (e) => {
      e.preventDefault();
      err.textContent = "";
      btn.disabled = true;
      try {
        setupSummary = await call("OpenSetupFile", page.path, pass.input.value);
        go("setup-summary");
      } catch (x) {
        err.textContent = errorText(x);
        btn.disabled = false;
        pass.input.select();
      }
    },
  },
    h("div.setup-head", icon("key", 28), h("p", t("setup_open_body")), h("p.file-name", page.path.split(/[\\/]/).pop())),
    field(t("setup_file_password"), pass),
    err,
    btn,
  );
  return h("div.screen", topBar(t("setup_open_title"), [], () => go("home")), h("main.content", form));
}

function setupSummaryView() {
  const s = setupSummary;
  if (!s) return homeView();
  let choice = s.needsLock ? (state.helloAvailable ? "hello" : "pin") : (state.helloAvailable ? "hello" : "pin");
  const err = h("p.form-error", { role: "alert" });
  const options = [];
  if (state.helloAvailable) options.push(["hello", t("setup_use_hello"), t("setup_use_hello_body"), "face"]);
  options.push(["pin", t("setup_use_pin"), t("setup_use_pin_body"), "key"]);
  if (!s.needsLock) options.push(["none", t("setup_no_lock"), "", "lock"]);
  const choices = h("div.choices", { role: "radiogroup" }, options.map(([v, title, body, ic]) =>
    h("button.choice" + (v === choice ? ".selected" : ""), {
      type: "button", role: "radio", "aria-checked": String(v === choice),
      onclick: (e) => {
        choice = v;
        choices.querySelectorAll(".choice").forEach((c) => { c.classList.remove("selected"); c.setAttribute("aria-checked", "false"); });
        e.currentTarget.classList.add("selected");
        e.currentTarget.setAttribute("aria-checked", "true");
      },
    }, icon(ic), h("div", h("span.choice-title", title), body ? h("span.choice-body", body) : null)),
  ));
  const items = [t("setup_item_vpn")];
  if (s.hasPassword) items.push(t("setup_item_password"));
  if (s.hasTotp) items.push(t("setup_item_totp"));
  if (s.links.length) items.push(t("setup_item_links", s.links.length));
  const cont = h("button.btn.btn-primary.btn-wide", {
    onclick: async () => {
      err.textContent = "";
      if (choice === "pin") return go("setup-pin", { purpose: "install" });
      cont.disabled = true;
      try {
        await call("InstallSetup", choice, "");
        setupSummary = null;
        go("home");
      } catch (x) {
        err.textContent = errorText(x);
        cont.disabled = false;
      }
    },
  }, t("action_ok"));
  return h("div.screen", topBar(t("setup_title", s.name), [], () => go("home")),
    h("main.content",
      h("section.section", h("h3.section-title", t("setup_contains")), h("ul.checklist", items.map((i) => h("li", icon("check", 18), i)))),
      s.replaces ? h("div.banner.banner-warn", icon("alert"), h("p", t("setup_replace_warning", s.replaces))) : null,
      h("p.muted", t(s.needsLock ? "setup_lock_required" : "setup_lock_optional")),
      choices, err, cont,
    ));
}

/** Choose + confirm an 8-digit PIN (install or change). */
function pinCreateView() {
  let first = null;
  const title = h("h2.pin-title", t("pin_create_title"));
  const sub = h("p.muted.center", t("pin_create_body"));
  const err = h("p.form-error.center", { role: "alert" });
  const pad = pinPad({
    onComplete: async (pin) => {
      err.textContent = "";
      if (first == null) {
        const problem = await call("CheckPin", pin);
        pad.reset();
        if (problem) return (err.textContent = t("pin_problem_" + problem));
        first = pin;
        title.textContent = t("pin_confirm_title");
        sub.textContent = "";
        return;
      }
      pad.reset();
      if (pin !== first) {
        first = null;
        title.textContent = t("pin_create_title");
        sub.textContent = t("pin_create_body");
        return (err.textContent = t("pin_mismatch"));
      }
      try {
        if (page.purpose === "change") {
          await call("ChangeLock", "pin", pin);
          toast(t("security_updated"));
          go("home");
        } else {
          await call("InstallSetup", "pin", pin);
          setupSummary = null;
          go("home");
        }
      } catch (x) {
        first = null;
        title.textContent = t("pin_create_title");
        err.textContent = errorText(x);
      }
    },
  });
  const back = () => go(page.purpose === "change" ? "security" : "setup-summary");
  return h("div.screen", topBar("", [], back), h("main.content.pin-page", title, sub, pad, err));
}

// ---- Security ----------------------------------------------------------------------------

function securityView() {
  const m = state.managed;
  const current = { hello: "security_current_hello", pin: "security_current_pin" }[m.lockMethod] || "security_current_none";
  const err = h("p.form-error", { role: "alert" });
  const actions = [];
  if (state.helloAvailable && m.lockMethod !== "hello") {
    actions.push(h("button.choice", { type: "button", onclick: async () => {
      try { await call("ChangeLock", "hello", ""); toast(t("security_updated")); go("home"); } catch (x) { err.textContent = errorText(x); }
    } }, icon("face"), h("span.choice-title", t("security_switch_hello"))));
  }
  actions.push(h("button.choice", { type: "button", onclick: () => go("setup-pin", { purpose: "change" }) },
    icon("key"), h("span.choice-title", t(m.lockMethod === "pin" ? "security_change_pin" : "security_switch_pin"))));
  return h("div.screen", topBar(t("security_title"), [], () => go("home")),
    h("main.content", h("div.banner.banner-info", icon("shield"), h("p", t(current))), h("div.choices", actions), err));
}

// ---- Editor -----------------------------------------------------------------------------------

function editorView() {
  const d = page.draft;
  const p = d ? null : (state.profiles || []).find((x) => x.id === page.profileId);
  if (!d && !p) return homeView();
  const v = {
    name: d ? d.name : p.name,
    username: d ? "" : p.username,
    twoFactor: d ? d.staticChallenge : p.twoFactor,
    codeAfter: d ? true : p.codeAfter,
    codeLength: d ? 6 : p.codeLength || 6,
  };
  const needsCreds = d ? d.needsCredentials : p.needsCredentials;
  const staticChallenge = d ? d.staticChallenge : p.staticChallenge;
  const name = h("input.input", { type: "text", value: v.name, autofocus: true, oninput: (e) => (v.name = e.target.value) });
  const user = h("input.input", { type: "text", value: v.username, autocomplete: "off", spellcheck: "false", oninput: (e) => (v.username = e.target.value) });
  const err = h("p.form-error", { role: "alert" });
  const tfaDetails = h("div.tfa-details",
    field(t("code_position"), segmented([[true, t("code_after")], [false, t("code_before")]], v.codeAfter, (x) => (v.codeAfter = x))),
    field(t("code_length"), segmented([[6, t("code_length_value", 6)], [8, t("code_length_value", 8)]], v.codeLength, (x) => (v.codeLength = x))),
  );
  tfaDetails.hidden = !v.twoFactor || staticChallenge;

  const save = async (e) => {
    e.preventDefault();
    err.textContent = "";
    try {
      if (d) {
        const id = await call("SaveDraft", d.id, v);
        selectedId = id;
        localStorage.setItem("selectedProfile", id);
      } else {
        await call("UpdateProfile", p.id, v);
      }
      go("home");
    } catch (x) {
      err.textContent = errorText(x);
    }
  };
  const del = async () => {
    if (await confirmDialog(t("delete_confirm_title", p.name), t("delete_confirm_body"), t("action_delete"))) {
      await call("DeleteProfile", p.id).catch((x) => toast(errorText(x), "error"));
      go("home");
    }
  };
  const form = h("form.form.page-form", { onsubmit: save },
    field(t("field_name"), name),
    needsCreds ? field(t("field_username"), user) : h("div.banner.banner-info", icon("info"), h("p", t("profile_no_auth"))),
    h("p.remote", icon("globe", 16), d ? d.remote : p.remote),
    needsCreds ? h("section.section",
      h("h3.section-title", t("section_2fa")),
      staticChallenge ? h("div.banner.banner-info", icon("info"), h("p", t("warning_static_challenge"))) : null,
      toggleRow(t("toggle_2fa"), t("toggle_2fa_body"), v.twoFactor, (x) => { v.twoFactor = x; tfaDetails.hidden = !x || staticChallenge; }),
      tfaDetails,
    ) : null,
    p?.hasSavedPassword ? h("button.btn.btn-text", { type: "button", onclick: async () => { await call("ForgetPassword", p.id); toast(t("security_updated")); } }, icon("key", 18), t("action_forget_password")) : null,
    err,
    h("div.form-actions",
      p ? h("button.btn.btn-text.danger", { type: "button", onclick: del }, icon("trash", 18), t("action_delete")) : h("span"),
      h("button.btn.btn-primary", { type: "submit" }, t("action_save")),
    ),
  );
  return h("div.screen", topBar(t(d ? "editor_title_new" : "editor_title_edit"), [], () => go("home")), h("main.content", form));
}

// ---- Logs & about ----------------------------------------------------------------------------

function logsView() {
  const pre = h("div.log", { role: "log", tabindex: "0" });
  const empty = h("p.muted.center", t("logs_empty"));
  const add = (lines) => {
    if (!lines?.length) return;
    empty.remove();
    const atEnd = pre.scrollTop + pre.clientHeight >= pre.scrollHeight - 8;
    for (const l of lines) pre.append(h("div.log-line", l));
    if (atEnd) pre.scrollTop = pre.scrollHeight;
  };
  call("Logs").then(add);
  const listener = (e) => add(e.detail);
  document.addEventListener("tk-log", listener);
  live.push(() => { if (!pre.isConnected) document.removeEventListener("tk-log", listener); });
  const copy = async () => {
    const ok = await copyText([...pre.children].map((c) => c.textContent).join("\n"));
    if (ok) toast(t("action_copied"));
  };
  return h("div.screen", topBar(t("action_logs"), [iconButton("copy", t("action_copy"), copy)], () => go("home")),
    h("main.content.log-page", empty, pre));
}

function aboutView() {
  return h("div.screen", topBar(t("action_about"), [], () => go("home")),
    h("main.content.about",
      logo(72),
      h("h2", "Tunnelkey"),
      h("p.muted", t("about_version", state.version), state.helperVersion ? "  ·  " + t("about_helper", state.helperVersion) : ""),
      t("about_body").split("\n\n").map((para) => h("p", para)),
      h("button.btn.btn-outline", { onclick: () => call("OpenURL", PRIVACY_URL) }, t("action_privacy")),
    ));
}

function fatalView(text) {
  return h("div.screen.lock", h("div.lock-head", logo(56), h("h1", t("app_name"))), h("div.banner.banner-error", icon("alert"), h("p", t("unexpected", text))));
}
