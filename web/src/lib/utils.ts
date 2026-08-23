import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/**
 * 构建 API URL
 * @param path API 路径
 * @returns 完整的 API URL
 */
export function buildApiUrl(path: string): string {
  // ---------- 浏览器端 ----------
  if (typeof window !== "undefined") {
    if (import.meta.env.DEV) {
      return path; // 开发环境由 vite 代理
    }

    return `${window.location.origin}${path}`; // 生产环境同源
  }

  // ---------- 服务器端 ----------
  // SSR/SSG 阶段返回原样（或按需拼同源）
  return path;
}

/**
 * 构建 WebSocket URL（确保为 ws/wss 的绝对 URL）
 * @param path WebSocket 路径（通常为 /api/ws/... 或 http(s)://...）
 * @returns 完整的 ws/wss URL
 */
export function buildWsUrl(path: string): string {
  if (typeof window === "undefined") return path;

  // 已经是 ws(s)://
  if (path.startsWith("ws://") || path.startsWith("wss://")) return path;

  const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";

  // http(s):// -> ws(s)://
  if (path.startsWith("http://") || path.startsWith("https://")) {
    const url = new URL(path);
    url.protocol = wsProtocol;
    return url.toString();
  }

  // /api/ws/... -> ws(s)://host/api/ws/...
  return `${wsProtocol}//${window.location.host}${path}`;
}

export function getAuthToken(): string | null {
  if (typeof window === "undefined") return null;
  return localStorage.getItem("nowhere.token");
}

/**
 * 将对象中的 BigInt 值转换为数字，用于 JSON 序列化
 * @param obj 要转换的对象
 * @returns 转换后的对象
 */
export function convertBigIntToNumber<T = any>(obj: T): T {
  if (obj === null || obj === undefined) {
    return obj;
  }

  if (typeof obj === "bigint") {
    // 如果 BigInt 值太大，转换为字符串；否则转换为数字
    return (obj > Number.MAX_SAFE_INTEGER ? obj.toString() : Number(obj)) as T;
  }

  if (Array.isArray(obj)) {
    return obj.map(convertBigIntToNumber) as T;
  }

  if (typeof obj === "object") {
    const converted: any = {};

    for (const [key, value] of Object.entries(obj)) {
      converted[key] = convertBigIntToNumber(value);
    }

    return converted as T;
  }

  return obj;
}

export function formatRelativeTime(timestamp: number): string {
  const now = Date.now();
  const diff = now - timestamp;
  const hours = Math.floor(diff / (1000 * 60 * 60));
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));
  const seconds = Math.floor((diff % (1000 * 60)) / 1000);

  if (hours > 24) {
    const days = Math.floor(hours / 24);

    return `${days}d`;
  }
  if (hours > 0) {
    return `${hours}h`;
  }
  if (minutes > 0) {
    return `${minutes}m`;
  }
  if (seconds >= 0) {
    return `${seconds}s`;
  }

  return "0s";
}

export function formatTime(timestamp: number): string {
  const date = new Date(timestamp);
  const year = date.getFullYear();
  const month = date.getMonth() + 1;
  const day = date.getDate();
  const hours = date.getHours().toString().padStart(2, "0");
  const minutes = date.getMinutes().toString().padStart(2, "0");
  const seconds = date.getSeconds().toString().padStart(2, "0");

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`;
}

export function formatTime12(timestamp: number): string {
  // example: 3:45 PM
  const date = new Date(timestamp);
  const hours = date.getHours();
  const minutes = date.getMinutes();
  const ampm = hours >= 12 ? "PM" : "AM";
  const hours12 = hours % 12 || 12;

  return `${hours12}:${minutes.toString().padStart(2, "0")} ${ampm}`;
}

/**
 * 格式化字节数为可读的字符串
 * @param bytes 字节数
 * @returns 格式化后的字符串 (如: "1.23 KB", "4.56 MB")
 */
export function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

// 重新导出隐私相关工具函数
export {
  formatUrlWithPrivacy,
  maskAddress,
  maskHostname,
} from "./utils/privacy";
