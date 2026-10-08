// node --test docs/provision/tests/core.test.mjs
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  PASSPHRASE_WORDS, SETUP_FILE_ITER, base45Decode, base45Encode, base64urlDecode, buildPayload,
  decodeSetupCodes, decryptSetupFile, encodeSetupCodes, encryptSetupFile, generatePassphrase,
  linkUri, parseOtpauth, payloadToPackage, totpCode, validatePackage, zlibDecompress,
} from "../core.js";
import * as serverWeb from "../../../server/web/setupfile.js";

const fixture = (p) => new URL(p, import.meta.url);

test("Base45 RFC 9285 vectors", () => {
  const cases = { AB: "BB8", "Hello!!": "%69 VD92EX0", "base-45": "UJCLQE7W581", "ietf!": "QED8WEX0" };
  for (const [plain, enc] of Object.entries(cases)) {
    assert.equal(base45Encode(new TextEncoder().encode(plain)), enc);
    assert.equal(new TextDecoder().decode(base45Decode(enc)), plain);
  }
});

test("TOTP RFC 6238 vectors", async () => {
  const b32 = (s) => {
    const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";
    let bits = "";
    for (const c of new TextEncoder().encode(s)) bits += c.toString(2).padStart(8, "0");
    let out = "";
    for (let i = 0; i < bits.length; i += 5) out += A[parseInt(bits.slice(i, i + 5).padEnd(5, "0"), 2)];
    return out;
  };
  const s20 = b32("12345678901234567890");
  const s32 = b32("12345678901234567890123456789012");
  const s64 = b32("1234567890123456789012345678901234567890123456789012345678901234");
  const at = (t) => new Date(t * 1000);
  assert.equal(await totpCode({ secret: s20, digits: 8 }, at(59)), "94287082");
  assert.equal(await totpCode({ secret: s32, digits: 8, algorithm: "SHA256" }, at(59)), "46119246");
  assert.equal(await totpCode({ secret: s64, digits: 8, algorithm: "SHA512" }, at(59)), "90693936");
  assert.equal(await totpCode({ secret: s20, digits: 8 }, at(1111111109)), "07081804");
  assert.equal(await totpCode({ secret: s20, digits: 8 }, at(20000000000)), "65353130");
});

test("otpauth links", () => {
  const t = parseOtpauth("otpauth://totp/VPN:marko?secret=jbsw y3dp ehpk3pxp&issuer=VPN&digits=8&period=60&algorithm=sha256");
  assert.deepEqual(t, { secret: "JBSWY3DPEHPK3PXP", digits: 8, period: 60, algorithm: "SHA256" });
  assert.throws(() => parseOtpauth("otpauth://hotp/x?secret=JBSWY3DPEHPK3PXP"));
});

test("RDP links match the server's encoding", () => {
  assert.equal(
    linkUri({ kind: "rdp", host: "10.0.0.5", username: "CORP\\marko" }),
    "rdp://full%20address=s:10.0.0.5:3389&username=s:CORP%5Cmarko",
  );
});

test("validation mirrors the server", () => {
  const ok = { name: "X", ovpn: "client\nremote a 1194\n" };
  validatePackage(ok);
  const bad = [
    { name: "", ovpn: ok.ovpn },
    { name: "X", ovpn: "client\n" },
    { name: "X", ovpn: "client\nremote a\nca ca.crt\n" },
    { ...ok, links: [{ title: "a", kind: "web", url: "javascript:alert(1)" }] },
    { ...ok, links: [{ title: "a", kind: "app", url: "javascript:alert(1)" }] },
    { ...ok, links: [{ title: "a", kind: "rdp", host: "a b" }] },
    { ...ok, totp: { secret: "not base32!" } },
  ];
  for (const p of bad) assert.throws(() => validatePackage(p));
});

function samplePackage(fillerBytes) {
  const noise = new Uint8Array(fillerBytes);
  crypto.getRandomValues(noise);
  const b64 = Buffer.from(noise).toString("base64").replace(/(.{64})/g, "$1\n");
  return {
    name: "Office",
    ovpn: `client\r\nremote vpn.example.com 1194 udp\r\n<ca>\r\n${b64}\n</ca>\r\n`,
    username: "marko",
    password: "pässwörd",
    totp: { secret: "JBSWY3DPEHPK3PXP" },
    links: [
      { title: "Intranet", kind: "web", url: "https://intranet.example.com" },
      { title: "My PC", kind: "rdp", host: "10.0.0.5", username: "CORP\\marko" },
    ],
  };
}

test("setup codes round-trip, single and multi-part, any order, trimmed spaces", async () => {
  for (const [bytes, expectMulti] of [[400, false], [4500, true]]) {
    const codes = await encodeSetupCodes(buildPayload(samplePackage(bytes)));
    assert.equal(codes.length > 1, expectMulti, `parts for ${bytes}`);
    const scanned = codes.map((c) => c.text.trimEnd()).reverse();
    const p = await decodeSetupCodes(scanned);
    assert.equal(p.n, "Office");
    assert.equal(p.p, "pässwörd");
    assert.equal(p.t.s, "JBSWY3DPEHPK3PXP");
    assert.equal(p.c, "a");
    assert.ok(!p.o.includes("\r"));
    assert.equal(p.l[1].u, "rdp://full%20address=s:10.0.0.5:3389&username=s:CORP%5Cmarko");
  }
});

test("reads a code produced by the Go server", async () => {
  const text = readFileSync(fixture("../../../android/app/src/test/resources/setup_code_single.txt"), "utf8");
  const p = await decodeSetupCodes([text]);
  assert.equal(p.n, "Office VPN");
  assert.equal(p.t.s, "JBSWY3DPEHPK3PXP");
  assert.equal(p.l.length, 2);
});

// ---------------------------------------------------------------- setup file (desktop)

const FILE_PASSWORD = "maple-otter-guitar-frost-pebble-42";

test("setup file: format and round trip", async () => {
  const payload = buildPayload(samplePackage(400));
  const text = await encryptSetupFile(payload, FILE_PASSWORD);
  assert.ok(text.endsWith("}\n"));
  const f = JSON.parse(text);
  assert.deepEqual(Object.keys(f), ["tunnelkey", "v", "kdf", "enc", "data"]);
  assert.equal(f.tunnelkey, "setup-file");
  assert.equal(f.v, 1);
  assert.deepEqual(Object.keys(f.kdf), ["alg", "iter", "salt"]);
  assert.equal(f.kdf.alg, "PBKDF2-SHA256");
  assert.equal(f.kdf.iter, SETUP_FILE_ITER);
  assert.equal(f.enc.alg, "A256GCM");
  for (const v of [f.kdf.salt, f.enc.iv, f.data]) assert.match(v, /^[A-Za-z0-9_-]+$/); // base64url, no padding
  assert.equal(base64urlDecode(f.kdf.salt).length, 16);
  assert.equal(base64urlDecode(f.enc.iv).length, 12);
  assert.ok(base64urlDecode(f.data).length > 16);
  assert.deepEqual(await decryptSetupFile(text, FILE_PASSWORD), payload);
  // Fresh salt and IV every time.
  const again = JSON.parse(await encryptSetupFile(payload, FILE_PASSWORD));
  assert.notEqual(again.kdf.salt, f.kdf.salt);
  assert.notEqual(again.enc.iv, f.enc.iv);
});

test("setup file: wrong password and tampering fail", async () => {
  const text = await encryptSetupFile(buildPayload(samplePackage(200)), FILE_PASSWORD);
  await assert.rejects(decryptSetupFile(text, FILE_PASSWORD + "x"), /Wrong password/);
  const edit = (fn) => { const f = JSON.parse(text); fn(f); return JSON.stringify(f); };
  // iter is bound by the AAD: a valid but different count must not decrypt.
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.iter = 100000; }), FILE_PASSWORD), /Wrong password/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.iter = 99999; }), FILE_PASSWORD), /iteration count/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.iter = 10000001; }), FILE_PASSWORD), /iteration count/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.iter = "600000"; }), FILE_PASSWORD), /iteration count/);
  const flip = (s) => (s[0] === "A" ? "B" : "A") + s.slice(1);
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.salt = flip(f.kdf.salt); }), FILE_PASSWORD), /Wrong password/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.enc.iv = flip(f.enc.iv); }), FILE_PASSWORD), /Wrong password/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.data = flip(f.data); }), FILE_PASSWORD), /Wrong password/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.kdf.salt += "=="; }), FILE_PASSWORD), /base64url/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.enc.iv = f.kdf.salt; }), FILE_PASSWORD), /length/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.v = 2; }), FILE_PASSWORD), /version/);
  await assert.rejects(decryptSetupFile(edit((f) => { f.enc.alg = "A128GCM"; }), FILE_PASSWORD), /Unsupported/);
  await assert.rejects(decryptSetupFile("TK1:1/1:ABCDEF:3:BB8", FILE_PASSWORD), /Not a Tunnelkey setup file/);
});

test("setup file: password rules and NFC", async () => {
  const payload = buildPayload({ name: "X", ovpn: "client\nremote a 1194\n" });
  await assert.rejects(encryptSetupFile(payload, "short-pw1"), /at least 10/);
  const composed = "Caf\u00e9-Kr\u00e4mer-2024";
  const decomposed = "Cafe\u0301-Kra\u0308mer-2024";
  const text = await encryptSetupFile(payload, composed);
  assert.deepEqual(await decryptSetupFile(text, decomposed), payload);
});

test("setup file: plaintext is zlib(JSON payload)", async () => {
  // Decrypt by hand to check the layering, not just the round trip.
  const payload = buildPayload({ name: "X", ovpn: "client\nremote a 1194\n" });
  const f = JSON.parse(await encryptSetupFile(payload, FILE_PASSWORD));
  const pw = await crypto.subtle.importKey("raw", new TextEncoder().encode(FILE_PASSWORD), "PBKDF2", false, ["deriveBits"]);
  const bits = await crypto.subtle.deriveBits({ name: "PBKDF2", hash: "SHA-256", salt: base64urlDecode(f.kdf.salt), iterations: f.kdf.iter }, pw, 256);
  const key = await crypto.subtle.importKey("raw", bits, "AES-GCM", false, ["decrypt"]);
  const aad = new TextEncoder().encode(`tunnelkey-setup-file:1:${f.kdf.iter}:${f.kdf.salt}:${f.enc.iv}`);
  const plain = await crypto.subtle.decrypt({ name: "AES-GCM", iv: base64urlDecode(f.enc.iv), additionalData: aad }, key, base64urlDecode(f.data));
  const json = new TextDecoder().decode(await zlibDecompress(new Uint8Array(plain)));
  assert.equal(json, JSON.stringify(payload));
});

test("passphrases", () => {
  assert.equal(PASSPHRASE_WORDS.length, 256);
  assert.equal(new Set(PASSPHRASE_WORDS).size, 256);
  for (const w of PASSPHRASE_WORDS) assert.match(w, /^[a-z]{3,8}$/);
  // 5 words × 8 bits + log2(100) for the digits.
  assert.ok(5 * Math.log2(PASSPHRASE_WORDS.length) + Math.log2(100) >= 45);
  const seen = new Set();
  for (let i = 0; i < 50; i++) {
    const p = generatePassphrase();
    const parts = p.split("-");
    assert.equal(parts.length, 6);
    for (const w of parts.slice(0, 5)) assert.ok(PASSPHRASE_WORDS.includes(w), w);
    assert.match(parts[5], /^\d\d$/);
    assert.ok(p.length >= 10);
    seen.add(p);
  }
  assert.equal(seen.size, 50);
});

test("server web UI copy agrees with this one", async () => {
  assert.deepEqual(serverWeb.PASSPHRASE_WORDS, PASSPHRASE_WORDS);
  const payload = buildPayload(samplePackage(300));
  assert.deepEqual(await decryptSetupFile(await serverWeb.encryptSetupFile(payload, FILE_PASSWORD), FILE_PASSWORD), payload);
  assert.deepEqual(await serverWeb.decryptSetupFile(await encryptSetupFile(payload, FILE_PASSWORD), FILE_PASSWORD), payload);
});

test("reads the setup file fixture shared with the Go tests", async () => {
  // server/testdata/setup_v1.tunnelkey, written once by this implementation.
  const text = readFileSync(fixture("../../../server/testdata/setup_v1.tunnelkey"), "utf8");
  const p = await decryptSetupFile(text, "correct-horse-battery-staple-07");
  assert.equal(p.n, "Office VPN");
  assert.equal(p.p, "pässwörd");
  assert.equal(p.t.s, "JBSWY3DPEHPK3PXP");
  assert.equal(p.l[1].u, "rdp://full%20address=s:10.0.0.5:3389&username=s:CORP%5Cmarko");
});

test("setup files open back into the editor", async () => {
  const pkg = {
    name: "Office VPN", ovpn: "client\nremote vpn.example.com 1194\nauth-user-pass\n", username: "marko",
    password: "pw", totp: { secret: "JBSWY3DPEHPK3PXP", digits: 6, period: 30, algorithm: "SHA1" },
    manualCode: false, codePosition: "before",
    links: [
      { kind: "web", title: "Intranet", url: "https://intranet.example.com" },
      { kind: "rdp", title: "PC & desk", host: "pc01.corp.local", port: 3390, username: String.raw`corp\marko` },
      { kind: "app", title: "App", url: "myapp://open?x=1" },
    ],
  };
  const payload = buildPayload(pkg);
  const file = await encryptSetupFile(payload, "correct-horse-battery");
  const back = payloadToPackage(await decryptSetupFile(file, "correct-horse-battery"));
  assert.deepEqual(buildPayload(back), payload);
  assert.equal(back.links[1].host, "pc01.corp.local");
  assert.equal(back.links[1].port, 3390);
  assert.equal(back.links[1].username, String.raw`corp\marko`);
  assert.equal(back.codePosition, "before");
});
