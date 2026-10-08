// Tunnelkey setup codes, computed entirely in the browser.
// Same format and rules as the Go server (server/setupcode.go, server/api.go);
// spec: docs/provisioning-format.md. No DOM access here, so it also runs in Node.

export const SINGLE_CODE_MAX = 2900; // Base45 chars that still fit one QR (ECC L)
export const PART_MAX = 1500; // chars per part when splitting (ECC M)
export const MAX_PROFILE_BYTES = 256 * 1024;
export const MAX_LINKS = 20;

const BASE45 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ $%*+-./:";

// ---------------------------------------------------------------- profile

export function normalizeNewlines(s) {
  return s.replace(/\r\n/g, "\n").replace(/^\uFEFF/, "");
}

/** Quick look at an .ovpn file for the form (remote, auth, inline blocks, problems). */
export function inspectOvpn(text) {
  const out = { remote: "", auth: false, nocache: false, staticChallenge: false, inline: [], external: [] };
  let block = null;
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim();
    if (!line || line.startsWith("#") || line.startsWith(";")) continue;
    if (block) {
      if (line.toLowerCase() === `</${block}>`) block = null;
      continue;
    }
    const tag = line.match(/^<([a-z0-9-]+)>$/i);
    if (tag) {
      const name = tag[1].toLowerCase();
      if (name !== "connection") {
        block = name;
        out.inline.push(name);
      }
      if (name === "auth-user-pass") out.auth = true;
      continue;
    }
    const f = line.split(/\s+/);
    const d = f[0].toLowerCase();
    if (d === "remote" && !out.remote && f[1]) out.remote = f[1] + (f[2] ? `:${f[2]}` : "") + (f[3] ? `/${f[3]}` : "");
    else if (d === "auth-user-pass") out.auth = true;
    else if (d === "auth-nocache") out.nocache = true;
    else if (d === "static-challenge") out.staticChallenge = true;
    else if (["ca", "cert", "key", "tls-auth", "tls-crypt", "tls-crypt-v2", "pkcs12"].includes(d) && f[1]) out.external.push(f[1]);
  }
  return out;
}

// ---------------------------------------------------------------- TOTP (RFC 6238)

export function base32Decode(input) {
  const s = input.toUpperCase().replace(/[\s=-]/g, "");
  const bytes = [];
  let buffer = 0;
  let bits = 0;
  for (const ch of s) {
    const v = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".indexOf(ch);
    if (v < 0) throw new Error("The TOTP secret is not valid base32.");
    buffer = (buffer << 5) | v;
    bits += 5;
    if (bits >= 8) {
      bytes.push((buffer >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return new Uint8Array(bytes);
}

/** Validates and canonicalises TOTP settings (upper-case base32, defaults). */
export function normalizeTotp(t) {
  const out = {
    secret: String(t.secret || "").toUpperCase().replace(/[\s=-]/g, ""),
    digits: Number(t.digits) || 6,
    period: Number(t.period) || 30,
    algorithm: String(t.algorithm || "SHA1").toUpperCase().replace("SHA-", "SHA"),
  };
  if (!out.secret) throw new Error("Enter the TOTP secret.");
  base32Decode(out.secret);
  if (out.digits !== 6 && out.digits !== 8) throw new Error("TOTP digits must be 6 or 8.");
  if (out.period < 10 || out.period > 300) throw new Error("TOTP period must be 10–300 seconds.");
  if (!["SHA1", "SHA256", "SHA512"].includes(out.algorithm)) throw new Error("TOTP algorithm must be SHA1, SHA256 or SHA512.");
  return out;
}

/** Reads an otpauth://totp/... link. */
export function parseOtpauth(uri) {
  let u;
  try {
    u = new URL(uri.trim());
  } catch {
    throw new Error("Not a valid otpauth:// link.");
  }
  if (u.protocol !== "otpauth:") throw new Error("Not an otpauth:// link.");
  if (u.hostname.toLowerCase() !== "totp") throw new Error("Only time-based (TOTP) codes are supported.");
  const q = u.searchParams;
  return normalizeTotp({ secret: q.get("secret"), digits: q.get("digits"), period: q.get("period"), algorithm: q.get("algorithm") });
}

export async function totpCode(t, date = new Date()) {
  const cfg = normalizeTotp(t);
  const hash = { SHA1: "SHA-1", SHA256: "SHA-256", SHA512: "SHA-512" }[cfg.algorithm];
  const key = await crypto.subtle.importKey("raw", base32Decode(cfg.secret), { name: "HMAC", hash }, false, ["sign"]);
  const counter = Math.floor(date.getTime() / 1000 / cfg.period);
  const msg = new DataView(new ArrayBuffer(8));
  msg.setUint32(0, Math.floor(counter / 2 ** 32));
  msg.setUint32(4, counter >>> 0);
  const mac = new Uint8Array(await crypto.subtle.sign("HMAC", key, msg.buffer));
  const off = mac[mac.length - 1] & 0x0f;
  const bin = ((mac[off] & 0x7f) << 24) | (mac[off + 1] << 16) | (mac[off + 2] << 8) | mac[off + 3];
  return String(bin % 10 ** cfg.digits).padStart(cfg.digits, "0");
}

export function secondsLeft(period, date = new Date()) {
  return period - (Math.floor(date.getTime() / 1000) % period);
}

// ---------------------------------------------------------------- links

// Only what would break the URI is escaped; ':' stays literal as in
// Microsoft's documented examples (full%20address=s:host:3389).
function rdpEscape(s) {
  return s.replace(/[% &=\\#?]/g, (c) => "%" + c.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0"));
}

export function linkUri(l) {
  if (l.kind !== "rdp") return l.url;
  let uri = "rdp://full%20address=s:" + rdpEscape(`${l.host}:${l.port || 3389}`);
  if (l.username) uri += "&username=s:" + rdpEscape(l.username);
  return uri;
}

// ---------------------------------------------------------------- validation

/** Checks a package like the server does; returns a cleaned copy or throws Error(message). */
export function validatePackage(pkg) {
  const p = { ...pkg };
  p.name = String(p.name || "").trim();
  if (!p.name || [...p.name].length > 80) throw new Error("Name is required (up to 80 characters).");

  p.ovpn = normalizeNewlines(String(p.ovpn || ""));
  if (!p.ovpn.trim()) throw new Error("An .ovpn profile is required.");
  if (new TextEncoder().encode(p.ovpn).length > MAX_PROFILE_BYTES) throw new Error("The .ovpn profile is too large.");
  const info = inspectOvpn(p.ovpn);
  if (!info.remote) throw new Error("The .ovpn profile has no “remote” line.");
  if (info.external.length) {
    throw new Error(`The profile references the file “${info.external[0]}”. Embed it inline — the phone can't read separate files.`);
  }

  p.username = String(p.username || "").trim();
  p.password = String(p.password || "");
  p.totp = p.totp ? normalizeTotp(p.totp) : null;
  p.manualCode = !!p.manualCode && !p.totp;
  p.codePosition = p.codePosition === "before" ? "before" : "after";

  const links = p.links || [];
  if (links.length > MAX_LINKS) throw new Error(`At most ${MAX_LINKS} links.`);
  p.links = links.map((l, i) => {
    const title = String(l.title || "").trim();
    if (!title || [...title].length > 60) throw new Error(`Link ${i + 1} needs a title (up to 60 characters).`);
    if (l.kind === "web") {
      let u;
      try { u = new URL(String(l.url || "").trim()); } catch { u = null; }
      if (!u || !["http:", "https:"].includes(u.protocol) || !u.host) throw new Error(`Link “${title}”: enter a full http(s):// address.`);
      return { title, kind: "web", url: u.href };
    }
    if (l.kind === "rdp") {
      const host = String(l.host || "").trim();
      if (!host || /[ /&?]/.test(host)) throw new Error(`Link “${title}”: enter the computer's host name or IP address.`);
      const port = Number(l.port) || 3389;
      if (port < 1 || port > 65535) throw new Error(`Link “${title}”: port must be 1–65535.`);
      return { title, kind: "rdp", host, port, username: String(l.username || "").trim() };
    }
    if (l.kind === "app") {
      const url = String(l.url || "").trim();
      const scheme = (url.match(/^([a-z][a-z0-9+.-]*):/i) || [])[1];
      if (!scheme) throw new Error(`Link “${title}”: enter a URI with a scheme, e.g. myapp://open.`);
      if (["javascript", "data", "file", "vbscript"].includes(scheme.toLowerCase())) {
        throw new Error(`Link “${title}”: ${scheme}: links are not allowed.`);
      }
      return { title, kind: "app", url };
    }
    throw new Error(`Link “${title}”: unknown kind.`);
  });
  return p;
}

// ---------------------------------------------------------------- payload & codes

export function buildPayload(pkg) {
  const p = validatePackage(pkg);
  const out = { v: 1, n: p.name, o: p.ovpn };
  if (p.username) out.u = p.username;
  if (p.password) out.p = p.password;
  if (p.totp) out.t = { s: p.totp.secret, d: p.totp.digits, p: p.totp.period, a: p.totp.algorithm };
  if (p.manualCode) out.f = true;
  if (p.totp || p.manualCode) out.c = p.codePosition === "before" ? "b" : "a";
  if (p.links.length) out.l = p.links.map((l) => ({ t: l.title, k: l.kind, u: linkUri(l) }));
  return out;
}

export function base45Encode(bytes) {
  let s = "";
  for (let i = 0; i + 1 < bytes.length; i += 2) {
    const v = bytes[i] * 256 + bytes[i + 1];
    s += BASE45[v % 45] + BASE45[Math.floor(v / 45) % 45] + BASE45[Math.floor(v / 2025)];
  }
  if (bytes.length % 2) {
    const v = bytes[bytes.length - 1];
    s += BASE45[v % 45] + BASE45[Math.floor(v / 45)];
  }
  return s;
}

export function base45Decode(s) {
  const vals = [...s].map((c) => {
    const v = BASE45.indexOf(c);
    if (v < 0) throw new Error("Invalid Base45 character");
    return v;
  });
  if (vals.length % 3 === 1) throw new Error("Invalid Base45 length");
  const out = [];
  for (let i = 0; i < vals.length; i += 3) {
    if (i + 2 < vals.length) {
      const v = vals[i] + vals[i + 1] * 45 + vals[i + 2] * 2025;
      if (v > 0xffff) throw new Error("Invalid Base45 triplet");
      out.push(v >> 8, v & 0xff);
    } else {
      const v = vals[i] + vals[i + 1] * 45;
      if (v > 0xff) throw new Error("Invalid Base45 pair");
      out.push(v);
    }
  }
  return new Uint8Array(out);
}

async function streamBytes(bytes, transform) {
  const stream = new Blob([bytes]).stream().pipeThrough(transform);
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

/** zlib (RFC 1950) — CompressionStream("deflate") produces exactly that. */
export const zlibCompress = (bytes) => streamBytes(bytes, new CompressionStream("deflate"));
export const zlibDecompress = (bytes) => streamBytes(bytes, new DecompressionStream("deflate"));

function randomId(n) {
  const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  const buf = crypto.getRandomValues(new Uint8Array(n));
  return [...buf].map((b) => alphabet[b % alphabet.length]).join("");
}

/** Returns [{ text, ecc }] — one entry per QR code. */
export async function encodeSetupCodes(payload) {
  const json = new TextEncoder().encode(JSON.stringify(payload));
  const text = base45Encode(await zlibCompress(json));
  const n = text.length > SINGLE_CODE_MAX ? Math.ceil(text.length / PART_MAX) : 1;
  const id = randomId(6);
  const size = Math.ceil(text.length / n);
  const codes = [];
  for (let i = 0; i < n; i++) {
    const part = text.slice(i * size, (i + 1) * size);
    codes.push({ text: `TK1:${i + 1}/${n}:${id}:${part.length}:${part}`, ecc: n > 1 ? "M" : "L" });
  }
  return codes;
}

/** Reference reader (tests, and "check" in the UI). */
export async function decodeSetupCodes(texts) {
  const parts = new Map();
  let id = null;
  let total = 0;
  for (const t of texts) {
    const m = t.match(/^TK1:(\d+)\/(\d+):([0-9A-Z]+):(\d+):([\s\S]*)$/);
    if (!m) throw new Error("not a setup code");
    const [, i, n, pid, len, data] = m;
    if (id === null) { id = pid; total = Number(n); } else if (pid !== id) continue;
    parts.set(Number(i), data.padEnd(Number(len), " "));
  }
  if (parts.size !== total) throw new Error(`have ${parts.size} of ${total} parts`);
  const joined = [...Array(total).keys()].map((k) => parts.get(k + 1)).join("");
  const raw = await zlibDecompress(base45Decode(joined));
  return JSON.parse(new TextDecoder().decode(raw));
}

// ---------------------------------------------------------------- setup file (desktop)

export const SETUP_FILE_TYPE = "application/vnd.tunnelkey.setup+json";
export const SETUP_FILE_ITER = 600000;
export const SETUP_FILE_MIN_PASSWORD = 10;
const SETUP_FILE_ITER_MIN = 100000;
const SETUP_FILE_ITER_MAX = 10000000;

export function base64urlEncode(bytes) {
  let bin = "";
  for (const b of bytes) bin += String.fromCharCode(b);
  return btoa(bin).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

export function base64urlDecode(s) {
  if (typeof s !== "string" || !/^[A-Za-z0-9_-]*$/.test(s) || s.length % 4 === 1) throw new Error("Invalid base64url");
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(s.length / 4) * 4, "="));
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

const passwordLength = (password) => [...String(password).normalize("NFC")].length;

async function setupFileKey(password, salt, iter) {
  const raw = new TextEncoder().encode(String(password).normalize("NFC"));
  const base = await crypto.subtle.importKey("raw", raw, "PBKDF2", false, ["deriveKey"]);
  return crypto.subtle.deriveKey(
    { name: "PBKDF2", hash: "SHA-256", salt, iterations: iter },
    base, { name: "AES-GCM", length: 256 }, false, ["encrypt", "decrypt"]);
}

const setupFileAad = (iter, salt, iv) => new TextEncoder().encode(`tunnelkey-setup-file:1:${iter}:${salt}:${iv}`);

/** Encrypts a payload into the text of a .tunnelkey setup file (format v1). */
export async function encryptSetupFile(payload, password) {
  if (passwordLength(password || "") < SETUP_FILE_MIN_PASSWORD) {
    throw new Error(`The file password needs at least ${SETUP_FILE_MIN_PASSWORD} characters.`);
  }
  const plain = await zlibCompress(new TextEncoder().encode(JSON.stringify(payload)));
  const iter = SETUP_FILE_ITER;
  const salt = base64urlEncode(crypto.getRandomValues(new Uint8Array(16)));
  const iv = base64urlEncode(crypto.getRandomValues(new Uint8Array(12)));
  const key = await setupFileKey(password, base64urlDecode(salt), iter);
  const data = await crypto.subtle.encrypt(
    { name: "AES-GCM", iv: base64urlDecode(iv), additionalData: setupFileAad(iter, salt, iv), tagLength: 128 }, key, plain);
  const file = {
    tunnelkey: "setup-file",
    v: 1,
    kdf: { alg: "PBKDF2-SHA256", iter, salt },
    enc: { alg: "A256GCM", iv },
    data: base64urlEncode(new Uint8Array(data)),
  };
  return JSON.stringify(file, null, 2) + "\n";
}

/** Reads a .tunnelkey setup file; returns the payload or throws Error(message). */
export async function decryptSetupFile(text, password) {
  let f;
  try {
    f = JSON.parse(text);
  } catch {
    f = null;
  }
  if (!f || typeof f !== "object" || f.tunnelkey !== "setup-file") throw new Error("Not a Tunnelkey setup file.");
  if (f.v !== 1) throw new Error(`Unsupported setup file version ${f.v}.`);
  const kdf = f.kdf || {};
  const enc = f.enc || {};
  if (kdf.alg !== "PBKDF2-SHA256" || enc.alg !== "A256GCM") throw new Error("Unsupported setup file encryption.");
  if (!Number.isInteger(kdf.iter) || kdf.iter < SETUP_FILE_ITER_MIN || kdf.iter > SETUP_FILE_ITER_MAX) {
    throw new Error("Invalid setup file: iteration count out of range.");
  }
  let salt, iv, data;
  try {
    salt = base64urlDecode(kdf.salt);
    iv = base64urlDecode(enc.iv);
    data = base64urlDecode(f.data);
  } catch {
    throw new Error("Invalid setup file: bad base64url field.");
  }
  if (salt.length !== 16 || iv.length !== 12 || data.length < 16) throw new Error("Invalid setup file: wrong field length.");
  const key = await setupFileKey(password, salt, kdf.iter);
  let plain;
  try {
    plain = await crypto.subtle.decrypt(
      { name: "AES-GCM", iv, additionalData: setupFileAad(kdf.iter, kdf.salt, enc.iv), tagLength: 128 }, key, data);
  } catch {
    throw new Error("Wrong password, or the file has been modified.");
  }
  const payload = JSON.parse(new TextDecoder().decode(await zlibDecompress(new Uint8Array(plain))));
  if (!payload || payload.v !== 1) throw new Error("Unsupported payload version.");
  return payload;
}

// 256 common, unambiguous words: one random byte picks one word (8 bits).
export const PASSPHRASE_WORDS = [
  "acorn", "almond", "anchor", "apple", "arrow", "atlas", "autumn", "badge", "bagel", "bakery",
  "bamboo", "banana", "banjo", "barrel", "basket", "beacon", "beaver", "beetle", "bicycle",
  "biscuit", "bison", "blanket", "blossom", "bottle", "bracket", "breeze", "bridge", "brook",
  "bucket", "buffalo", "bundle", "butter", "cabbage", "cabin", "cactus", "camera", "candle",
  "canoe", "canyon", "caramel", "carbon", "carpet", "cashew", "castle", "cattle", "cedar", "cellar",
  "cement", "cereal", "chapel", "cheetah", "cherry", "chimney", "cinema", "circle", "citrus",
  "clover", "cobalt", "coconut", "comet", "compass", "copper", "cotton", "cougar", "crayon",
  "cricket", "crystal", "cupboard", "cushion", "dagger", "daisy", "dancer", "delta", "desert",
  "diamond", "dinner", "dolphin", "donkey", "dragon", "drawer", "eagle", "easel", "echo", "eclipse",
  "elbow", "elephant", "ember", "emerald", "engine", "falcon", "feather", "fennel", "ferry",
  "fiddle", "field", "figure", "filter", "finger", "flannel", "flower", "forest", "fossil",
  "fountain", "fox", "frost", "galaxy", "garden", "garlic", "gazelle", "gecko", "giant", "ginger",
  "glacier", "globe", "glove", "goblet", "granite", "gravel", "guitar", "hammer", "hamster",
  "harbor", "harvest", "hazel", "helmet", "heron", "hollow", "honey", "horizon", "hornet", "icicle",
  "igloo", "iguana", "island", "ivory", "jacket", "jaguar", "jasmine", "jelly", "jersey", "jigsaw",
  "jungle", "kayak", "kernel", "kettle", "kitten", "kiwi", "koala", "ladder", "lagoon", "lantern",
  "laptop", "lemon", "lettuce", "library", "lilac", "linen", "lizard", "lobster", "locket",
  "magnet", "mango", "maple", "marble", "meadow", "melon", "meteor", "mirror", "mitten", "monkey",
  "mosaic", "muffin", "museum", "napkin", "nectar", "needle", "noodle", "nutmeg", "oasis", "ocean",
  "olive", "onion", "orange", "orbit", "orchid", "otter", "oyster", "paddle", "palace", "panda",
  "paper", "parrot", "pebble", "pelican", "pencil", "pepper", "piano", "pickle", "pigeon", "pillow",
  "pirate", "planet", "pocket", "pony", "potato", "puzzle", "quartz", "quiver", "rabbit", "radish",
  "rainbow", "raven", "ribbon", "river", "rocket", "saddle", "salmon", "sandal", "saturn", "scarf",
  "shadow", "shovel", "silver", "sketch", "spider", "spinach", "spiral", "sponge", "squirrel",
  "statue", "summit", "sunset", "tablet", "teapot", "thunder", "tiger", "timber", "tomato",
  "tractor", "trumpet", "tulip", "tunnel", "turtle", "umbrella", "valley", "velvet", "violin",
  "volcano", "wagon", "walnut", "walrus", "window", "winter", "wizard", "yogurt", "zebra",
];

/** Five random words and two digits, e.g. "maple-otter-guitar-frost-pebble-42" (~46 bits). */
export function generatePassphrase() {
  const words = [...crypto.getRandomValues(new Uint8Array(5))].map((b) => PASSPHRASE_WORDS[b]);
  let n;
  do n = crypto.getRandomValues(new Uint8Array(1))[0]; while (n >= 200); // no modulo bias
  return [...words, String(n % 100).padStart(2, "0")].join("-");
}
