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
  sharedKey?: string | null;
  network?: string | null;
  alpn?: string | null;
  rate?: string | number | null;
  etar?: string | number | null;
  logLevel?: string | null;
  endpoint?: PortalEndpointLike | string | null;
}

const WILDCARD_HOSTS = new Set(["", "0.0.0.0", "::", "[::]", "*"]);
const PORTAL_EFFECTIVE_QUERY_KEYS = new Set([
  "net",
  "tls",
  "alpn",
  "rate",
  "etar",
  "dial",
  "socks",
  "next",
  "up",
  "down",
  "pool",
  "sni",
  "pin",
]);
const NEXT_ONLY_QUERY_KEYS = new Set(["up", "down", "pool", "sni", "pin"]);

interface ParsedPortalUrl {
  hostname: string;
  port: string;
  searchParams: URLSearchParams;
  username: string;
}

const encodeValue = (value: string | number) =>
  encodeURIComponent(String(value));

const formatHost = (host: string) => {
  const value = host.trim();

  if (!value || (value.startsWith("[") && value.endsWith("]"))) return value;

  return value.includes(":") ? `[${value}]` : value;
};

const endpointHostname = (endpoint?: PortalEndpointLike | string | null) => {
  if (!endpoint || typeof endpoint === "string") return "";

  const configured = endpoint.hostname?.trim();

  if (configured && !WILDCARD_HOSTS.has(configured)) return configured;

  const rawUrl = endpoint.url?.trim();

  if (!rawUrl) return "";

  try {
    return new URL(rawUrl).hostname;
  } catch {
    try {
      return new URL(`master://${rawUrl}`).hostname;
    } catch {
      return "";
    }
  }
};

const portalUrlFromCommand = (commandLine?: string | null) => {
  if (!commandLine) return "";

  return commandLine.match(/portal:\/\/[^\s"']+/i)?.[0] ?? "";
};

/**
 * WHATWG URL rejects Nowhere's credential-free, empty-host runtime form
 * (`portal://:2077?...`), so Portal authorities need a small native parser.
 */
const parsePortalUrl = (rawUrl: string): ParsedPortalUrl | null => {
  const match = rawUrl
    .trim()
    .match(/^portal:\/\/([^/?#]*)(?:\?([^#]*))?(?:#.*)?$/i);

  if (!match) return null;

  const authority = match[1];
  const separator = authority.lastIndexOf("@");
  const encodedUsername = separator >= 0 ? authority.slice(0, separator) : "";
  const hostAndPort =
    separator >= 0 ? authority.slice(separator + 1) : authority;
  let hostname = "";
  let port = "";

  if (hostAndPort.startsWith("[")) {
    const ipv6 = hostAndPort.match(/^\[([^\]]*)\](?::(\d+))?$/);

    if (!ipv6) return null;
    hostname = ipv6[1];
    port = ipv6[2] ?? "";
  } else {
    const colon = hostAndPort.lastIndexOf(":");

    if (colon < 0) {
      hostname = hostAndPort;
    } else {
      hostname = hostAndPort.slice(0, colon);
      port = hostAndPort.slice(colon + 1);
      if (!/^\d+$/.test(port) || hostname.includes(":")) return null;
    }
  }

  let username = encodedUsername;

  try {
    username = decodeURIComponent(encodedUsername);
  } catch {
    // Keep malformed credentials usable as an opaque fallback value.
  }

  return {
    hostname,
    port,
    searchParams: new URLSearchParams(match[2] ?? ""),
    username,
  };
};

const normalizePortalHost = (host: string) => {
  const value = host.replace(/^\[|\]$/g, "").toLowerCase();

  return WILDCARD_HOSTS.has(value) ? "" : value;
};

const withoutAuthorityCredential = (value: string) => {
  const separator = value.lastIndexOf("@");

  return separator >= 0 ? value.slice(separator + 1) : value;
};

const comparablePortalQueryValue = (key: string, value: string) =>
  key === "socks" || key === "next" ? withoutAuthorityCredential(value) : value;

const portalUrlsMatch = (commandURL: string, configURL: string) => {
  const command = parsePortalUrl(commandURL);
  const config = parsePortalUrl(configURL);

  if (!command || !config) return false;

  if (
    command.port !== config.port ||
    normalizePortalHost(command.hostname) !==
      normalizePortalHost(config.hostname)
  ) {
    return false;
  }

  // Current Nowhere releases redact the Portal credential. Older compatible
  // emitters may still return it, in which case it must identify this command.
  if (config.username && config.username !== command.username) return false;

  const commandNext = command.searchParams.get("next");
  const nextEnabled = Boolean(commandNext && commandNext !== "none");

  for (const key of new Set(command.searchParams.keys())) {
    if (!PORTAL_EFFECTIVE_QUERY_KEYS.has(key)) continue;
    if (!nextEnabled && NEXT_ONLY_QUERY_KEYS.has(key)) continue;

    const commandValue = command.searchParams.get(key);
    const configValue = config.searchParams.get(key);

    if (
      commandValue == null ||
      configValue == null ||
      comparablePortalQueryValue(key, commandValue) !==
        comparablePortalQueryValue(key, configValue)
    ) {
      return false;
    }
  }

  return true;
};

const portalUrlFromTunnel = (tunnel: PortalTunnelLike) => {
  const commandURL =
    portalUrlFromCommand(tunnel.commandURL) ||
    portalUrlFromCommand(tunnel.commandLine);
  const configURL =
    portalUrlFromCommand(tunnel.configURL) ||
    portalUrlFromCommand(tunnel.configLine);

  return configURL && (!commandURL || portalUrlsMatch(commandURL, configURL))
    ? configURL
    : commandURL;
};

const valuesFromPortalUrl = (portalUrl: string) => {
  const parsed = parsePortalUrl(portalUrl);

  if (!parsed) return {};

  return {
    sharedKey: parsed.username,
    listenHost: parsed.hostname,
    listenPort: parsed.port,
    network: parsed.searchParams.get("net") ?? undefined,
    alpn: parsed.searchParams.get("alpn") ?? undefined,
    rate: parsed.searchParams.get("rate") ?? undefined,
    etar: parsed.searchParams.get("etar") ?? undefined,
    logLevel: parsed.searchParams.get("log") ?? undefined,
  };
};

export const buildPortalUrl = (tunnel: PortalTunnelLike) => {
  const existing = portalUrlFromTunnel(tunnel);

  if (existing) return existing;

  const host = formatHost(tunnel.listenHost?.trim() ?? "");
  const port = tunnel.listenPort == null ? "" : String(tunnel.listenPort);
  const sharedKey = tunnel.sharedKey?.trim() ?? "";

  if (!port || !sharedKey) return "";

  return `portal://${encodeValue(sharedKey)}@${host}:${port}`;
};

/**
 * Builds a native Vector URL for a Portal. Portal upstream sni/pin values are
 * deliberately not inherited: they describe the Portal's own `next` hop.
 */
export const deriveVectorUrl = (
  tunnel: PortalTunnelLike,
  endpoint?: PortalEndpointLike | null,
  socks = "127.0.0.1:1080",
) => {
  const commandURL =
    portalUrlFromCommand(tunnel.commandURL) ||
    portalUrlFromCommand(tunnel.commandLine);
  const fromUrl = valuesFromPortalUrl(portalUrlFromTunnel(tunnel));
  const fromCommand = valuesFromPortalUrl(commandURL);
  const sharedKey =
    fromCommand.sharedKey ||
    tunnel.sharedKey?.trim() ||
    fromUrl.sharedKey ||
    "";
  const listenPort = String(
    fromUrl.listenPort ?? tunnel.listenPort ?? "",
  ).trim();
  const listenHost = (fromUrl.listenHost ?? tunnel.listenHost ?? "").trim();
  const publicHost = !WILDCARD_HOSTS.has(listenHost)
    ? listenHost
    : endpointHostname(
        endpoint ??
          (typeof tunnel.endpoint === "object" ? tunnel.endpoint : null),
      );

  if (!sharedKey || !listenPort || !publicHost) return null;

  const network = (fromUrl.network ?? tunnel.network ?? "mix").toLowerCase();
  const carrier = network === "tcp" ? "tcp" : "udp";
  const pool = carrier === "tcp" ? 5 : 0;
  const alpn = fromUrl.alpn ?? tunnel.alpn ?? "now/1";
  const rate = fromUrl.rate ?? tunnel.rate ?? 0;
  const etar = fromUrl.etar ?? tunnel.etar ?? 0;
  const log =
    fromCommand.logLevel ?? tunnel.logLevel ?? fromUrl.logLevel ?? "info";
  const query = [
    ["up", carrier],
    ["down", carrier],
    ["pool", pool],
    ["sni", "none"],
    ["pin", "none"],
    ["alpn", alpn],
    ["rate", rate],
    ["etar", etar],
    ["socks", socks],
    ["log", log],
  ]
    .map(([key, value]) => `${key}=${encodeValue(value)}`)
    .join("&");

  return `nowhere://${encodeValue(sharedKey)}@${formatHost(publicHost)}:${listenPort}?${query}`;
};
