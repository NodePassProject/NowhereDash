export interface PortalEndpointLike {
  hostname?: string | null;
  url?: string | null;
}

export interface PortalTunnelLike {
  commandURL?: string | null;
  configURL?: string | null;
  commandLine?: string | null;
  configLine?: string | null;
  listenHost?: string | null;
  listenPort?: string | number | null;
  tcpPort?: string | null;
  udpPort?: string | null;
  tcpFamily?: string | null;
  udpFamily?: string | null;
  morph?: string | null;
  sharedKey?: string | null;
  network?: string | null;
  tlsMode?: string | null;
  certPath?: string | null;
  keyPath?: string | null;
  rate?: string | number | null;
  etar?: string | number | null;
  dial?: string | null;
  socks?: string | null;
  next?: string | null;
  up?: string | null;
  down?: string | null;
  mux?: string | null;
  sni?: string | null;
  pin?: string | null;
  logLevel?: string | null;
  endpoint?: PortalEndpointLike | string | null;
}

export interface ServiceEndpoint {
  host: string;
  tcpPort: string;
  udpPort: string;
  tcpFamily: string;
  udpFamily: string;
}

const WILDCARD_HOSTS = new Set(["", "0.0.0.0", "::", "[::]", "*"]);
const QUERY_DEFAULTS = {
  tls: "1",
  morph: "0",
  rate: "0",
  etar: "0",
  dial: "auto",
  socks: "none",
  next: "none",
};
const NEXT_KEYS = new Set(["up", "down", "mux", "sni", "pin"]);
const normalizeHost = (host: string) => {
  if (host.startsWith("[") && host.endsWith("]")) {
    try {
      return new URL(`http://${host}`).hostname.slice(1, -1);
    } catch {
      return host;
    }
  }

  return host;
};
const formatHost = (host: string) => {
  const value = normalizeHost(host.trim());

  return value.includes(":") ? `[${value}]` : value;
};

export const formatServiceEndpoint = (endpoint: ServiceEndpoint) => {
  const host = formatHost(endpoint.host || "*");

  if (
    endpoint.tcpPort &&
    endpoint.tcpPort === endpoint.udpPort &&
    endpoint.tcpFamily === "any" &&
    endpoint.udpFamily === "any"
  )
    return `${host}:${endpoint.tcpPort}`;

  return (
    host +
    (["tcp", "udp"] as const)
      .map((carrier) => {
        const port = endpoint[`${carrier}Port`];
        const family = endpoint[`${carrier}Family`];

        return port
          ? `/${carrier}${family === "any" ? "" : family}:${port}`
          : "";
      })
      .join("")
  );
};

export const validateServiceEndpoint = (
  endpoint: ServiceEndpoint,
  allowWildcard = true,
) => {
  const host = normalizeHost(endpoint.host);

  if (
    !host ||
    /[/@?#%[\]<>\s\\]/.test(host) ||
    Array.from(host).some(
      (char) => char.charCodeAt(0) < 32 || char.charCodeAt(0) === 127,
    )
  )
    throw new Error("Invalid endpoint host");
  if (host.includes(":")) {
    try {
      new URL(`http://[${host}]`);
    } catch {
      throw new Error("Invalid endpoint IPv6 address");
    }
  }
  if (!allowWildcard && host === "*")
    throw new Error("A remote endpoint requires a concrete host");
  if (!endpoint.tcpPort && !endpoint.udpPort)
    throw new Error("At least one TCP or UDP listener is required");
  for (const carrier of ["tcp", "udp"] as const) {
    const port = endpoint[`${carrier}Port`];
    const family = endpoint[`${carrier}Family`];

    if (!port) continue;
    if (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535)
      throw new Error(
        `${carrier.toUpperCase()} port must be between 1 and 65535`,
      );
    if (!["any", "4", "6"].includes(family))
      throw new Error("Invalid address family");
    if (
      (host.includes(":") && family === "4") ||
      (/^\d+\.\d+\.\d+\.\d+$/.test(host) && family === "6")
    )
      throw new Error("Address family conflicts with the endpoint IP");
  }
};

const parseServiceUrl = (raw: string, allowWildcard: boolean) => {
  // WHATWG rejects the compact empty host alias; normalize it before parsing.
  const normalized = raw.replace(/^(portal:\/\/(?:[^/?#]*@)?):/i, "$1*:");
  const path =
    normalized.split(/[?#]/, 1)[0].split("://")[1]?.split("/").slice(1) ?? [];

  if (path.some((segment) => !/^(tcp|udp)[46]?:\d+$/.test(segment)))
    throw new Error("Invalid carrier path");
  const parsed = new URL(normalized);
  const endpoint: ServiceEndpoint = {
    host: normalizeHost(parsed.hostname),
    tcpPort: "",
    udpPort: "",
    tcpFamily: "any",
    udpFamily: "any",
  };

  if (path.length) {
    if (parsed.port)
      throw new Error("Authority port and carrier path cannot be combined");
    for (const segment of path) {
      const [name, port] = segment.split(":");
      const carrier = name.slice(0, 3) as "tcp" | "udp";

      if (endpoint[`${carrier}Port`]) throw new Error("Duplicate carrier");
      endpoint[`${carrier}Port`] = String(Number(port));
      endpoint[`${carrier}Family`] = name.slice(3) || "any";
    }
  } else {
    endpoint.tcpPort = parsed.port;
    endpoint.udpPort = parsed.port;
  }
  validateServiceEndpoint(endpoint, allowWildcard);

  return { parsed, endpoint };
};

export const parseNextEndpoint = (value: string) => {
  if ((value.match(/@/g) ?? []).length !== 1 || /[?#&\s]/.test(value))
    throw new Error("Next requires one encoded shared key and endpoint");
  const { parsed, endpoint } = parseServiceUrl(`vector://${value}`, false);

  if (!parsed.username || parsed.password || value.split("@")[0].includes(":"))
    throw new Error("Invalid next shared key");
  if (
    new TextEncoder().encode(decodeURIComponent(parsed.username)).length > 255
  )
    throw new Error("Shared key must contain 1 to 255 bytes");

  return endpoint;
};

export const validateSocksEndpoint = (
  value: string,
  allowEmptyHost = false,
) => {
  if (!value || value === "none") return;
  if (/[&?#\s]/.test(value))
    throw new Error("Reserved SOCKS characters must be percent-encoded");
  const parts = value.split("@");

  if (parts.length > 2) throw new Error("Invalid SOCKS credentials");
  if (parts.length === 2) {
    const credentials = parts[0].split(":");

    if (credentials.length !== 2)
      throw new Error("SOCKS requires USERNAME:PASSWORD");
    for (const credential of credentials) {
      const size = new TextEncoder().encode(
        decodeURIComponent(credential),
      ).length;

      if (size < 1 || size > 255 || /[:/?#[\]@!$&'()*+,;=\s]/.test(credential))
        throw new Error(
          "SOCKS credentials require percent-encoded reserved characters",
        );
    }
  }
  const endpoint = decodeURIComponent(parts.at(-1)!);
  const normalized =
    allowEmptyHost && endpoint.startsWith(":")
      ? `127.0.0.1${endpoint}`
      : endpoint;
  const { parsed } = parseServiceUrl(`socks://${normalized}`, false);

  if (
    parsed.username ||
    parsed.password ||
    parsed.pathname ||
    parsed.search ||
    parsed.hash
  )
    throw new Error("SOCKS requires HOST:PORT");
};

export const validatePortalSNI = (value: string) => {
  if (!value || value === "none") return;
  if (
    value.length > 253 ||
    /[^\x20-\x7E]|[:[\]]/.test(value) ||
    /^\d+\.\d+\.\d+\.\d+$/.test(value)
  )
    throw new Error("SNI must be an ASCII DNS name");
};

// Native queries keep '+' literal and decode nested credentials only after
// splitting their authority delimiters.
const portalQuery = (parsed: URL) => {
  const values = new Map<string, string>();
  const read = (allowed: Set<string>) => {
    for (const pair of parsed.search.slice(1).split("&")) {
      const separator = pair.indexOf("=");
      const rawKey = separator < 0 ? pair : pair.slice(0, separator);
      const rawValue = separator < 0 ? "" : pair.slice(separator + 1);
      let key: string;

      try {
        key = decodeURIComponent(rawKey);
      } catch {
        continue;
      }
      if (!allowed.has(key) || values.has(key)) continue;
      const value = decodeURIComponent(rawValue);

      values.set(
        key,
        (key === "next" || key === "socks") && value !== "none"
          ? rawValue
          : value,
      );
    }
  };

  read(new Set([...Object.keys(QUERY_DEFAULTS), "crt", "key", "log"]));
  if (values.has("next") && values.get("next") !== "none") read(NEXT_KEYS);

  return values;
};

const encodeComponent = (value: string) =>
  encodeURIComponent(value).replace(
    /[!'()*]/g,
    (character) => `%${character.charCodeAt(0).toString(16).toUpperCase()}`,
  );

const encodeNativeQuery = (query: URLSearchParams) =>
  Array.from(
    query,
    ([key, value]) =>
      `${encodeComponent(key)}=${
        key === "next" || key === "socks" ? value : encodeComponent(value)
      }`,
  ).join("&");

const portalUrlFromCommand = (command?: string | null) => {
  const value = command?.trim() ?? "";

  return /^portal:\/\//i.test(value)
    ? value
    : (value.match(/portal:\/\/[^\s"']+/i)?.[0] ?? "");
};

const parsePortalUrl = (raw: string) => {
  try {
    const result = parseServiceUrl(raw, true);

    if (result.parsed.protocol !== "portal:") return null;
    decodeURIComponent(result.parsed.username);

    return result;
  } catch {
    return null;
  }
};

const withoutCredential = (value: string) =>
  value.slice(value.lastIndexOf("@") + 1);
const comparablePortalQuery = (parsed: URL) => {
  const query = portalQuery(parsed);
  const values = new Map<string, string>(
    Object.entries(QUERY_DEFAULTS).map(([key, fallback]) => [
      key,
      query.get(key) ?? fallback,
    ]),
  );

  if (values.get("socks") !== "none") {
    const endpoint = decodeURIComponent(
      withoutCredential(values.get("socks")!),
    );

    validateSocksEndpoint(values.get("socks")!);
    values.set(
      "socks",
      formatServiceEndpoint(
        parseServiceUrl(`socks://${endpoint}`, false).endpoint,
      ),
    );
  }
  if (values.get("next") !== "none") {
    const { endpoint } = parseServiceUrl(
      `vector://${withoutCredential(values.get("next")!)}`,
      false,
    );
    const carrier = endpoint.tcpPort ? "tcp" : "udp";

    values.set("next", formatServiceEndpoint(endpoint));
    for (const [key, fallback] of Object.entries({
      up: carrier,
      down: carrier,
      mux: "0",
      sni: "none",
      pin: "none",
    }))
      values.set(key, query.get(key) || fallback);
    if (values.get("up") === "udp" && values.get("down") === "udp")
      values.set("mux", "0");
  }
  for (const key of ["rate", "etar"])
    values.set(key, String(Number(values.get(key))));
  if (values.get("dial")?.includes(":"))
    values.set("dial", normalizeHost(`[${values.get("dial")}]`));

  return values;
};
const portalUrlsMatch = (commandURL: string, configURL: string) => {
  const command = parsePortalUrl(commandURL);
  const config = parsePortalUrl(configURL);

  if (
    !command ||
    !config ||
    formatServiceEndpoint(command.endpoint) !==
      formatServiceEndpoint(config.endpoint)
  )
    return false;
  if (
    config.parsed.username &&
    decodeURIComponent(config.parsed.username) !==
      decodeURIComponent(command.parsed.username)
  )
    return false;
  try {
    const expected = comparablePortalQuery(command.parsed);
    const actual = comparablePortalQuery(config.parsed);

    return Array.from(expected).every(
      ([key, value]) => actual.get(key) === value,
    );
  } catch {
    return false;
  }
};

const portalUrlFromTunnel = (tunnel: PortalTunnelLike) => {
  const command =
    portalUrlFromCommand(tunnel.commandURL) ||
    portalUrlFromCommand(tunnel.commandLine);
  const config =
    portalUrlFromCommand(tunnel.configURL) ||
    portalUrlFromCommand(tunnel.configLine);

  return config && (!command || portalUrlsMatch(command, config))
    ? config
    : command;
};

export const portalServiceEndpoint = (
  tunnel: PortalTunnelLike,
): ServiceEndpoint => {
  const fromUrl = parsePortalUrl(portalUrlFromTunnel(tunnel));

  if (fromUrl) return fromUrl.endpoint;
  const legacy = tunnel.tcpPort == null && tunnel.udpPort == null;
  const port = String(tunnel.listenPort ?? "");

  return {
    host: normalizeHost(tunnel.listenHost || "*"),
    tcpPort: legacy
      ? tunnel.network === "udp"
        ? ""
        : port
      : (tunnel.tcpPort ?? ""),
    udpPort: legacy
      ? tunnel.network === "tcp"
        ? ""
        : port
      : (tunnel.udpPort ?? ""),
    tcpFamily: tunnel.tcpFamily || "any",
    udpFamily: tunnel.udpFamily || "any",
  };
};

export const portalNetworkLabel = (tunnel: PortalTunnelLike) => {
  const endpoint = portalServiceEndpoint(tunnel);

  return [endpoint.tcpPort ? "TCP" : "", endpoint.udpPort ? "UDP" : ""]
    .filter(Boolean)
    .join(" + ");
};

export const portalListenerAddresses = (tunnel: PortalTunnelLike) => {
  const endpoint = portalServiceEndpoint(tunnel);
  const canonical = formatServiceEndpoint(endpoint);

  if (!canonical.includes("/")) return [canonical];

  return [
    endpoint.tcpPort ? formatServiceEndpoint({ ...endpoint, udpPort: "" }) : "",
    endpoint.udpPort ? formatServiceEndpoint({ ...endpoint, tcpPort: "" }) : "",
  ].filter(Boolean);
};

export const buildPortalUrl = (tunnel: PortalTunnelLike) => {
  const existing =
    portalUrlFromCommand(tunnel.commandURL) ||
    portalUrlFromCommand(tunnel.commandLine);

  if (existing) return existing;
  if (!tunnel.sharedKey) return "";
  const endpoint = portalServiceEndpoint(tunnel);

  try {
    validateServiceEndpoint(endpoint);
  } catch {
    return "";
  }
  const query = new URLSearchParams({
    tls: tunnel.tlsMode || "1",
    morph: tunnel.morph || "0",
  });

  for (const key of [
    "rate",
    "etar",
    "dial",
    "socks",
    "next",
    "up",
    "down",
    "mux",
    "sni",
    "pin",
  ] as const) {
    if (tunnel[key] != null && tunnel[key] !== "")
      query.set(key, String(tunnel[key]));
  }
  const next = tunnel.next && tunnel.next !== "none" ? tunnel.next : "none";

  query.set("next", next);
  if (next === "none") {
    for (const key of NEXT_KEYS) query.delete(key);
  } else {
    try {
      const endpoint = parseNextEndpoint(next);
      const carrier = endpoint.tcpPort ? "tcp" : "udp";

      query.set("up", tunnel.up || carrier);
      query.set("down", tunnel.down || carrier);
      query.set(
        "mux",
        query.get("up") === "udp" && query.get("down") === "udp"
          ? "0"
          : tunnel.mux || "0",
      );
    } catch {
      return "";
    }
  }
  try {
    validateSocksEndpoint(tunnel.socks || "none");
  } catch {
    return "";
  }
  if (tunnel.tlsMode === "2") {
    query.set("crt", tunnel.certPath || "");
    query.set("key", tunnel.keyPath || "");
  }
  query.set("log", tunnel.logLevel || "info");

  return `portal://${encodeComponent(tunnel.sharedKey)}@${formatServiceEndpoint(endpoint)}?${encodeNativeQuery(query)}`;
};

const endpointHostname = (endpoint?: PortalEndpointLike | string | null) => {
  if (!endpoint || typeof endpoint === "string") return "";
  if (endpoint.hostname && !WILDCARD_HOSTS.has(endpoint.hostname))
    return endpoint.hostname.trim();
  try {
    return new URL(endpoint.url || "").hostname;
  } catch {
    return "";
  }
};

export const deriveVectorUrl = (
  tunnel: PortalTunnelLike,
  endpoint?: PortalEndpointLike | null,
  socks = "127.0.0.1:1080",
) => {
  const commandURL =
    portalUrlFromCommand(tunnel.commandURL) ||
    portalUrlFromCommand(tunnel.commandLine);
  const fromCommand = parsePortalUrl(commandURL);
  const fromUrl = parsePortalUrl(portalUrlFromTunnel(tunnel));
  const service = portalServiceEndpoint(tunnel);
  const sharedKey = fromCommand?.parsed.username
    ? decodeURIComponent(fromCommand.parsed.username)
    : tunnel.sharedKey;

  if (!sharedKey) return null;
  if (WILDCARD_HOSTS.has(service.host))
    service.host = normalizeHost(endpointHostname(endpoint ?? tunnel.endpoint));
  try {
    validateServiceEndpoint(service, false);
    validateSocksEndpoint(socks, true);
  } catch {
    return null;
  }
  let sourceQuery: Map<string, string>;

  try {
    sourceQuery = fromUrl ? portalQuery(fromUrl.parsed) : new Map();
  } catch {
    return null;
  }
  const sourceValue = (key: "morph" | "rate" | "etar") =>
    fromUrl ? (sourceQuery.get(key) ?? "0") : String(tunnel[key] ?? "0");
  const carrier = service.tcpPort ? "tcp" : "udp";
  const query = new URLSearchParams({
    up: carrier,
    down: carrier,
    mux: "0",
    sni: "none",
    pin: "none",
    morph: sourceValue("morph"),
    rate: sourceValue("rate"),
    etar: sourceValue("etar"),
    socks,
    log:
      fromCommand?.parsed.searchParams.get("log") ?? tunnel.logLevel ?? "info",
  });

  return `vector://${encodeComponent(sharedKey)}@${formatServiceEndpoint(service)}?${encodeNativeQuery(query)}`;
};
