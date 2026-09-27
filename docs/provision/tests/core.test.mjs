// node --test docs/provision/tests/
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import {
  base45Decode, base45Encode, buildPayload, decodeSetupCodes, encodeSetupCodes,
  linkUri, parseOtpauth, totpCode, validatePackage,
} from "../core.js";

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
