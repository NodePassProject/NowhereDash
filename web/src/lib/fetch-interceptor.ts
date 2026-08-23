/**
 * 全局 Fetch 拦截器
 * 自动为所有 API 请求添加 JWT token
 */

// 保存原始的 fetch 函数
const originalFetch = window.fetch;

// 获取 token
function getToken(): string | null {
  if (typeof window !== "undefined") {
    return localStorage.getItem("nowhere.token");
  }
  return null;
}

// 检查是否是 API 请求
function isApiRequest(url: string | URL | Request): boolean {
  const urlString = typeof url === "string" ? url : url instanceof Request ? url.url : url.toString();
  return urlString.includes("/api/");
}

// 检查是否是公开端点（不需要认证）
function isPublicEndpoint(url: string | URL | Request): boolean {
  const urlString = typeof url === "string" ? url : url instanceof Request ? url.url : url.toString();

  const publicEndpoints = [
    "/api/auth/login",
    "/api/auth/init",
    "/api/auth/check-default-credentials",
    "/api/auth/oauth2",
    "/api/oauth2/callback",
    "/api/oauth2/login",
  ];

  return publicEndpoints.some(endpoint => urlString.includes(endpoint));
}

// 覆盖全局 fetch 函数
window.fetch = async function (
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<Response> {
  // 检查是否是 API 请求且不是公开端点
  if (isApiRequest(input) && !isPublicEndpoint(input)) {
    // 获取 token
    const token = getToken();

    if (token) {
      // 创建新的 headers 对象
      const headers = new Headers(init?.headers);

      // 如果还没有 Authorization header，则添加
      if (!headers.has("Authorization")) {
        headers.set("Authorization", `Bearer ${token}`);
      }

      // 创建新的 init 对象
      init = {
        ...init,
        headers,
      };

      console.log("🔐 自动添加 Authorization header:", input);
    } else {
      console.warn("⚠️ API 请求缺少 token:", input);
    }
  }

  // 调用原始 fetch
  const response = await originalFetch(input, init);

  // 如果返回 401，token 可能已过期或被踢出，清除本地存储并跳转到登录页
  if (response.status === 401 && !isPublicEndpoint(input)) {
    console.warn("🚨 Token 已过期或被踢出，清除本地存储并跳转到登录页");
    if (typeof window !== "undefined") {
      localStorage.removeItem("nowhere.token");
      localStorage.removeItem("nowhere.tokenExpiresAt");
      localStorage.removeItem("nowhere.user");

      // 延迟跳转，避免在请求过程中跳转导致问题
      setTimeout(() => {
        // 只在非登录页时跳转
        if (!window.location.pathname.includes("/login")) {
          window.location.href = "/login";
        }
      }, 100);
    }
  }

  return response;
};

console.log("✅ Fetch 拦截器已安装");

// 导出一个空对象以便导入
export {};
