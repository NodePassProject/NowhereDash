import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import ts from "typescript";

const source = readFileSync(
  new URL("../src/lib/portal-url.ts", import.meta.url),
  "utf8",
);
const { outputText } = ts.transpileModule(source, {
  compilerOptions: {
    target: ts.ScriptTarget.ES2022,
    module: ts.ModuleKind.ES2022,
  },
});
const {
  buildPortalUrl,
  deriveVectorUrl,
  portalServiceEndpoint,
  formatServiceEndpoint,
  parseNextEndpoint,
  validateServiceEndpoint,
  validateSocksEndpoint,
} = await import(
  `data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`
);

test("v2 carrier paths preserve ports, families, IPv6 and Morph in Vector exports", () => {
  const url = new URL(
    deriveVectorUrl(
      {
        commandLine:
          "portal://key%3A%2F%40@*/udp6:2017/tcp4:2006?morph=1&alpn=ignored&net=udp",
      },
      { hostname: "entry.example" },
    ),
  );
  assert.equal(url.protocol, "vector:");
  assert.equal(url.hostname, "entry.example");
  assert.equal(decodeURIComponent(url.username), "key:/@");
  assert.equal(url.pathname, "/tcp4:2006/udp6:2017");
  assert.equal(url.searchParams.get("morph"), "1");
  assert.equal(url.searchParams.get("up"), "tcp");
  assert.equal(url.searchParams.get("down"), "tcp");
  assert.equal(url.searchParams.get("mux"), "0");
  assert.equal(url.searchParams.has("alpn"), false);
  assert.equal(url.searchParams.has("net"), false);
});

test("native query export preserves encoded credentials and literal plus", () => {
  const raw = buildPortalUrl({
    sharedKey: "local", listenHost: "*", tcpPort: "2000", udpPort: "",
    next: "up%40key@origin.example/udp6:3017", mux: "1",
    tlsMode: "2", certPath: "/cert+name.pem", keyPath: "/key name.pem",
  });
  assert.ok(raw.includes("next=up%40key@origin.example/udp6:3017"), raw);
  assert.ok(raw.includes("key=%2Fkey%20name.pem"), raw);
  assert.equal(new URL(raw).searchParams.get("mux"), "0");
  const socks = buildPortalUrl({ sharedKey: "local", listenPort: "2000", socks: "user:p%40ss@proxy.example:1080" });
  assert.ok(socks.includes("socks=user:p%40ss@proxy.example:1080"), socks);
});

test("removed options cannot survive through stale runtime configuration", () => {
  for (const option of ["morph=1", "rate=50", "etar=50"]) {
    const vector = new URL(deriveVectorUrl({
      commandLine: "portal://key@*:2000",
      configLine: `portal://*:2000?${option}`,
      morph: "1", rate: 50, etar: 50,
    }, { hostname: "origin.example" }));
    for (const key of ["morph", "rate", "etar"])
      assert.equal(vector.searchParams.get(key), "0", `${option}: ${key}`);
  }
});

test("native next key limits and malformed form hosts fail validation", () => {
  assert.throws(() => parseNextEndpoint(`${"a".repeat(256)}@host:2000`));
  assert.throws(() => parseNextEndpoint(`${"%E4%B8%AD".repeat(86)}@host:2000`));
  for (const host of ["bad%host", "bad\thost", "broken]", "[broken", "bad<host", "not:ipv6"])
    assert.throws(() => validateServiceEndpoint({host, tcpPort: "2000", udpPort: "", tcpFamily: "any", udpFamily: "any"}), host);
  assert.doesNotThrow(() => validateServiceEndpoint({host: "[::1]", tcpPort: "2000", udpPort: "", tcpFamily: "6", udpFamily: "any"}));
  assert.throws(() => validateSocksEndpoint("proxy&rate=50:1080"));
});

test("quoted shared keys survive Portal and Vector export", () => {
  const sharedKey = "key'with+reserved@bytes";
  const commandLine = buildPortalUrl({sharedKey, listenPort: "2000"});
  const vector = new URL(deriveVectorUrl({commandLine}, {hostname: "entry.example"}));
  assert.equal(decodeURIComponent(vector.username), sharedKey);
  assert.equal(portalServiceEndpoint({commandLine}).tcpPort, "2000");
});

test("compact alias enables both carriers and ignores the removed net parameter", () => {
  const endpoint = portalServiceEndpoint({
    commandLine: "portal://key@:2000?net=tcp",
  });
  assert.equal(formatServiceEndpoint(endpoint), "*:2000");
  assert.equal(endpoint.tcpPort, "2000");
  assert.equal(endpoint.udpPort, "2000");
  assert.equal(
    formatServiceEndpoint(
      portalServiceEndpoint({
        commandLine: "portal://key@*/tcp:2000/udp:2000",
      }),
    ),
    "*:2000",
  );
});

test("UDP-only Vector defaults and public host family constraints", () => {
  const tunnel = { commandLine: "portal://key@*/udp6:2017" };
  const vector = new URL(deriveVectorUrl(tunnel, { hostname: "2001:db8::1" }));
  assert.equal(vector.host, "[2001:db8::1]");
  assert.equal(vector.pathname, "/udp6:2017");
  assert.equal(vector.searchParams.get("up"), "udp");
  assert.equal(vector.searchParams.get("down"), "udp");
  assert.equal(deriveVectorUrl(tunnel, { hostname: "192.0.2.1" }), null);
});

test("runtime redaction and canonical endpoint order retain submitted credentials", () => {
  const commandLine = "portal://secret@*/udp:2017/tcp:2006?morph=1&rate=50";
  const tunnel = {
    commandLine,
    configLine: "portal://*/tcp:2006/udp:2017?morph=1&rate=50",
  };
  const vector = new URL(
    deriveVectorUrl(tunnel, { hostname: "entry.example" }),
  );
  assert.equal(vector.username, "secret");
  assert.equal(vector.searchParams.get("rate"), "50");
  assert.equal(buildPortalUrl(tunnel), commandLine);
  const stale = new URL(
    deriveVectorUrl(
      { ...tunnel, configLine: "portal://*/tcp:2006/udp:9999?morph=1&rate=99" },
      { hostname: "entry.example" },
    ),
  );
  assert.equal(stale.pathname, "/tcp:2006/udp:2017");
  assert.equal(stale.searchParams.get("rate"), "50");
});

test("entity-only Portal export includes the full v2 configuration", () => {
  const url = new URL(
    buildPortalUrl({
      sharedKey: "secret",
      listenHost: "*",
      tcpPort: "2006",
      udpPort: "2017",
      tcpFamily: "4",
      udpFamily: "6",
      morph: "1",
      rate: 40,
      next: "upstream@origin.example/udp6:3017",
      up: "udp",
      down: "udp",
      mux: "0",
    }),
  );
  assert.equal(url.pathname, "/tcp4:2006/udp6:2017");
  assert.equal(url.searchParams.get("morph"), "1");
  assert.equal(url.searchParams.get("rate"), "40");
  assert.equal(
    url.searchParams.get("next"),
    "upstream@origin.example/udp6:3017",
  );
});

test("next accepts independent carriers and rejects malformed endpoint grammar", () => {
  assert.equal(
    formatServiceEndpoint(parseNextEndpoint("key%40value@[::1]/udp6:2017")),
    "[::1]/udp6:2017",
  );
  for (const value of [
    "key@*/tcp:2000",
    "key@host:2000/tcp:2000",
    "key@host/tcp:2000/",
    "key@host/tcp:2000/tcp6:2000",
    "key@host/sctp:2000",
    "key@host/tcp:0",
    "key@host/tcp:65536",
    "key@host/tcp:2000/../udp:2000",
    "key@host/%2e/tcp:2000",
    "key@192.0.2.1/tcp6:2000",
    "key:password@host:2000",
    "key@host:2000?morph=1",
  ])
    assert.throws(() => parseNextEndpoint(value), value);
});
