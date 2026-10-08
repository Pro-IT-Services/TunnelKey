// Tunnelkey setup files (desktop): the setup-code payload, encrypted with a
// password in the browser. The password never leaves this page.
// Spec: docs/provisioning-format.md. Keep in sync with docs/provision/core.js
// (docs/provision/tests/core.test.mjs checks that both agree).

async function streamBytes(bytes, transform) {
  const stream = new Blob([bytes]).stream().pipeThrough(transform);
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

/** zlib (RFC 1950) — CompressionStream("deflate") produces exactly that. */
const zlibCompress = (bytes) => streamBytes(bytes, new CompressionStream("deflate"));
const zlibDecompress = (bytes) => streamBytes(bytes, new DecompressionStream("deflate"));

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
