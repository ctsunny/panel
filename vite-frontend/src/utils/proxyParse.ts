/**
 * Proxy protocol URL parser and reconstructor.
 * Supports: vmess, vless, trojan, ss (Shadowsocks), hysteria2/hy2, hysteria, tuic
 */

export interface ParsedProxy {
  protocol: string;   // e.g. "vmess", "vless", "trojan", "ss", "hysteria2", "tuic"
  host: string;
  port: number;
  name: string;       // display name decoded from fragment
  originalUrl: string;
  /** Reconstruct the URL with a new host and port */
  withAddress: (newHost: string, newPort: number) => string;
}

// ---------- helpers ----------

function decodeFragment(fragment: string): string {
  try {
    return decodeURIComponent(fragment);
  } catch {
    return fragment;
  }
}

/** Safely parse a URL - returns null when the browser rejects it */
function tryParseURL(raw: string): URL | null {
  try {
    return new URL(raw);
  } catch {
    return null;
  }
}

/** Replace host & port in a standard URL string */
function replaceHostPort(original: string, newHost: string, newPort: number): string {
  try {
    const u = new URL(original);
    // IPv6 addresses (multiple colons) need square brackets in the hostname setter
    u.hostname = newHost.includes(':') && !newHost.startsWith('[') ? `[${newHost}]` : newHost;
    u.port = String(newPort);
    return u.toString();
  } catch {
    return original;
  }
}

/** Decode URL-safe base64, handling missing padding */
function decodeBase64(b64: string): string {
  const padded = b64.replace(/-/g, '+').replace(/_/g, '/');
  const withPadding = padded + '='.repeat((4 - (padded.length % 4)) % 4);
  return atob(withPadding);
}
// vmess://BASE64  where base64 decodes to a JSON config object

interface VmessConfig {
  v?: string;
  ps?: string;
  add?: string;
  port?: string | number;
  id?: string;
  aid?: string | number;
  net?: string;
  type?: string;
  host?: string;
  path?: string;
  tls?: string;
  sni?: string;
  [key: string]: unknown;
}

function parseVmess(url: string): ParsedProxy | null {
  const b64 = url.slice('vmess://'.length);
  let json: VmessConfig;
  try {
    const decoded = decodeBase64(b64);
    json = JSON.parse(decoded);
  } catch {
    return null;
  }

  const host = json.add ?? '';
  const port = Number(json.port ?? 0);
  const name = json.ps ?? '';
  if (!host || !port) return null;

  return {
    protocol: 'vmess',
    host,
    port,
    name,
    originalUrl: url,
    withAddress(newHost, newPort) {
      const newConfig: VmessConfig = { ...json, add: newHost, port: String(newPort) };
      const newB64 = btoa(JSON.stringify(newConfig));
      return `vmess://${newB64}`;
    },
  };
}

// ---------- VLESS ----------
// vless://UUID@host:port?params#name

function parseVless(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port);
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'vless',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

// ---------- Trojan ----------
// trojan://PASSWORD@host:port?params#name

function parseTrojan(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port);
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'trojan',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

// ---------- Shadowsocks ----------
// SIP002: ss://BASE64(method:password)@host:port#name
// Old:    ss://BASE64(method:password@host:port)#name

function parseSS(url: string): ParsedProxy | null {
  // Try SIP002 first
  const u = tryParseURL(url);
  if (u && u.hostname) {
    const host = u.hostname;
    const port = parseInt(u.port);
    const name = decodeFragment(u.hash.slice(1));
    if (host && port) {
      return {
        protocol: 'ss',
        host,
        port,
        name,
        originalUrl: url,
        withAddress: (h, p) => replaceHostPort(url, h, p),
      };
    }
  }

  // Old format: ss://BASE64#name
  try {
    const [main, fragment] = url.slice('ss://'.length).split('#');
    const name = fragment ? decodeFragment(fragment) : '';
    const decoded = decodeBase64(main);
    // decoded: method:password@host:port
    const atIdx = decoded.lastIndexOf('@');
    if (atIdx === -1) return null;
    const hostPart = decoded.slice(atIdx + 1);
    const colonIdx = hostPart.lastIndexOf(':');
    if (colonIdx === -1) return null;
    const host = hostPart.slice(0, colonIdx);
    const port = parseInt(hostPart.slice(colonIdx + 1));
    const credentials = decoded.slice(0, atIdx);
    if (!host || !port) return null;

    return {
      protocol: 'ss',
      host,
      port,
      name,
      originalUrl: url,
      withAddress(newHost, newPort) {
        const newDecoded = `${credentials}@${newHost}:${newPort}`;
        const newB64 = btoa(newDecoded);
        return `ss://${newB64}${name ? '#' + encodeURIComponent(name) : ''}`;
      },
    };
  } catch {
    return null;
  }
}

// ---------- Hysteria2 / hy2 ----------
// hysteria2://auth@host:port?params#name
// hy2://auth@host:port?params#name

function parseHysteria2(url: string): ParsedProxy | null {
  // Normalise scheme so URL parser accepts it
  const normalised = url.replace(/^hy2:\/\//, 'hysteria2://');
  const u = tryParseURL(normalised);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port);
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'hysteria2',
    host,
    port,
    name,
    originalUrl: url,
    withAddress(newHost, newPort) {
      const replaced = replaceHostPort(normalised, newHost, newPort);
      // Restore hy2:// if original used that scheme
      return url.startsWith('hy2://') ? replaced.replace(/^hysteria2:\/\//, 'hy2://') : replaced;
    },
  };
}

// ---------- Hysteria (v1) ----------
// hysteria://host:port?auth=...&protocol=...#name

function parseHysteria(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port);
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'hysteria',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

// ---------- TUIC ----------
// tuic://uuid:password@host:port?params#name

function parseTUIC(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port);
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'tuic',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

// ---------- SOCKS5 / HTTP ----------

const DEFAULT_SOCKS5_PORT = 1080;
const DEFAULT_HTTP_PORT = 80;

function parseSocks5(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port || String(DEFAULT_SOCKS5_PORT));
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'socks5',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

function parseHTTP(url: string): ParsedProxy | null {
  const u = tryParseURL(url);
  if (!u) return null;
  const host = u.hostname;
  const port = parseInt(u.port || String(DEFAULT_HTTP_PORT));
  const name = decodeFragment(u.hash.slice(1));
  if (!host || !port) return null;

  return {
    protocol: 'http',
    host,
    port,
    name,
    originalUrl: url,
    withAddress: (h, p) => replaceHostPort(url, h, p),
  };
}

// ---------- Public API ----------

/**
 * Parse a single proxy protocol URL.
 * Returns null if the URL is not a recognised proxy protocol or cannot be parsed.
 */
export function parseProxyURL(raw: string): ParsedProxy | null {
  const url = raw.trim();
  if (url.startsWith('vmess://')) return parseVmess(url);
  if (url.startsWith('vless://')) return parseVless(url);
  if (url.startsWith('trojan://')) return parseTrojan(url);
  if (url.startsWith('ss://')) return parseSS(url);
  if (url.startsWith('hysteria2://') || url.startsWith('hy2://')) return parseHysteria2(url);
  if (url.startsWith('hysteria://')) return parseHysteria(url);
  if (url.startsWith('tuic://')) return parseTUIC(url);
  if (url.startsWith('socks5://')) return parseSocks5(url);
  if (url.startsWith('http://') || url.startsWith('https://')) return parseHTTP(url);
  return null;
}

/**
 * Batch-parse a block of text containing one proxy URL per line.
 * Lines that cannot be parsed are returned with success=false.
 */
export interface BatchParseResult {
  line: string;
  parsed: ParsedProxy | null;
  error?: string;
}

export function batchParseProxyURLs(text: string): BatchParseResult[] {
  return text
    .split('\n')
    .map(l => l.trim())
    .filter(l => l.length > 0)
    .map(line => {
      const parsed = parseProxyURL(line);
      if (!parsed) return { line, parsed: null, error: '无法识别的协议格式' };
      return { line, parsed };
    });
}

/**
 * Protocol display label map.
 */
export const PROTOCOL_LABELS: Record<string, string> = {
  vmess: 'VMess',
  vless: 'VLESS',
  trojan: 'Trojan',
  ss: 'Shadowsocks',
  hysteria2: 'Hysteria2',
  hysteria: 'Hysteria',
  tuic: 'TUIC',
  socks5: 'SOCKS5',
  http: 'HTTP',
  https: 'HTTPS',
};
