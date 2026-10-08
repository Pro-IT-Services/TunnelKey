// Development-only fake backend: `npm run dev` then open /?mock=<scenario>.
// Scenarios: empty, home, connected, failed, managed, locked-pin, locked-hello,
// helper-down. Never bundled into the app (loaded only in Vite dev mode).

const scenario = new URLSearchParams(location.search).get("mock") || "home";
const listeners = {};
const emit = (ev, ...a) => (listeners[ev] || []).forEach((f) => f(...a));

const profiles = [
  { id: "p1", name: "Office VPN", remote: "vpn.example.com:1194/udp", username: "marko", needsCredentials: true, twoFactor: true, codeAfter: true, codeLength: 6, hasSavedPassword: false },
  { id: "p2", name: "Home lab", remote: "home.example.net:443/tcp", username: "", needsCredentials: true, twoFactor: false, codeAfter: true, codeLength: 6, hasSavedPassword: true },
];
const managed = {
  name: "ProIT Office", profileId: "m1", remote: "gw.pro-it.example:1194/udp", hasTotp: true, hasPassword: true,
  manualCode: false, codeLength: 6, needsUser: false, needsCredentials: true, lockMethod: "pin",
  links: [
    { t: "Intranet", k: "web", u: "https://intranet.pro-it.example" },
    { t: "My workstation", k: "rdp", u: "rdp://full%20address=s:10.0.0.5:3389&username=s:marko" },
    { t: "Helpdesk app", k: "app", u: "helpdesk://open" },
  ],
};
const connected = { phase: "connected", vpnAddress: "10.8.0.6", server: "203.0.113.10:1194", connectedAt: Date.now() - 754000, bytesIn: 18_430_112, bytesOut: 2_104_550 };
const s = {
  version: "1.0.0", platform: "windows", language: "", helperUp: scenario !== "helper-down", helperVersion: "1.0.0",
  status: { phase: "disconnected" }, profiles: scenario === "empty" ? [] : profiles, managed: null,
  locked: false, helloAvailable: !location.search.includes("nohello"), weakKeyStorage: false, retryIn: 0,
};
if (scenario === "connected") s.status = connected;
if (scenario === "failed") s.status = { phase: "failed", failure: "auth_failed" };
if (scenario.startsWith("managed") || scenario.startsWith("locked")) s.managed = managed;
if (scenario === "locked-pin") s.locked = true;
if (scenario === "locked-hello") { s.locked = true; managed.lockMethod = "hello"; }
if (scenario === "managed-connected") s.status = connected;

const delay = (ms) => new Promise((r) => setTimeout(r, ms));
const logs = ["18:02:11 Connecting to Office VPN", "18:02:11 OpenVPN 2.6.14 x86_64-w64-mingw32", "18:02:12 TCP/UDP: Preserving recently used remote address", "18:02:13 Initialization Sequence Completed"];

window.runtime = {
  EventsOn: (ev, f) => (listeners[ev] ||= []).push(f),
  ClipboardGetText: async () => "",
  ClipboardSetText: async () => true,
};
window.go = { main: { App: {
  State: async () => structuredClone(s),
  TakePendingFiles: async () => [],
  Logs: async () => logs,
  SetLanguage: async (l) => { s.language = l; },
  Connect: async () => {
    s.status = { phase: "connecting", step: "AUTH" }; emit("status", s.status);
    await delay(1500); s.status = { ...connected, connectedAt: Date.now() }; emit("status", s.status);
  },
  ConnectManaged: async () => window.go.main.App.Connect(),
  Disconnect: async () => {
    s.status = { phase: "disconnecting" }; emit("status", s.status);
    await delay(600); s.status = { phase: "disconnected" }; emit("status", s.status);
  },
  UnlockWithPin: async (pin) => {
    if (pin === "48159263") { s.locked = false; emit("state"); return { ok: true }; }
    return { ok: false, attemptsLeft: 7, lockedUntil: 0 };
  },
  UnlockWithHello: async () => { await delay(400); throw "hello_cancelled"; },
  PinStatus: async () => ({ attemptsLeft: 10, lockedUntil: 0 }),
  CheckPin: async (pin) => (/^(\d)\1+$/.test(pin) ? "too_few_digits" : ""),
  ChooseFile: async () => ({ kind: "setup", path: String.raw`C:\Users\marko\Downloads\proit-office.tunnelkey` }),
  ImportPath: async () => ({ kind: "cancelled" }),
  ImportText: async () => ({ kind: "cancelled" }),
  OpenSetupFile: async (_p, pw) => {
    await delay(500);
    if (pw !== "correct-horse") throw "wrong_password";
    return { name: "ProIT Office", remote: managed.remote, hasTotp: true, hasPassword: true, manualCode: false, links: managed.links, needsLock: true, totpNeedsHello: !s.helloAvailable };
  },
  InstallSetup: async () => { s.managed = { ...managed }; emit("state"); },
  SaveDraft: async () => "p3",
  UpdateProfile: async () => {},
  DeleteProfile: async () => {},
  ForgetPassword: async () => {},
  RemoveManaged: async () => { s.managed = null; emit("state"); },
  ChangeLock: async () => {},
  Lock: async () => {},
  OpenLink: async () => {},
  OpenURL: async () => {},
  ProvideCredentials: async () => {},
} } };
