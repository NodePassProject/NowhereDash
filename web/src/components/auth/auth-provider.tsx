import {
  createContext,
  useContext,
  useEffect,
  useState,
  ReactNode,
} from "react";
import { useNavigate } from "react-router-dom";

import { buildApiUrl } from "@/lib/utils";

interface User {
  username: string;
}

interface AuthContextType {
  user: User | null;
  loading: boolean;
  logout: () => Promise<void>;
  checkAuth: (forceCheck?: boolean) => Promise<void>;
  setUserDirectly: (user: User | null) => void;
  getToken: () => string | null;
  setToken: (token: string, expiresAt?: string) => void;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export function useAuth() {
  const context = useContext(AuthContext);

  if (context === undefined) {
    throw new Error("useAuth 必须在 AuthProvider 内部使用");
  }

  return context;
}

interface AuthProviderProps {
  children: ReactNode;
}

export function AuthProvider({ children }: AuthProviderProps) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [lastCheckTime, setLastCheckTime] = useState<number>(0);
  const navigate = useNavigate();

  // 获取 token
  const getToken = () => {
    if (typeof window !== "undefined") {
      return localStorage.getItem("nowhere.token");
    }
    return null;
  };

  // 设置 token
  const setToken = (token: string, expiresAt?: string) => {
    if (typeof window !== "undefined") {
      localStorage.setItem("nowhere.token", token);
      if (expiresAt) {
        localStorage.setItem("nowhere.tokenExpiresAt", expiresAt);
      }
    }
  };

  // 清除 token
  const clearToken = () => {
    if (typeof window !== "undefined") {
      localStorage.removeItem("nowhere.token");
      localStorage.removeItem("nowhere.tokenExpiresAt");
    }
  };

  // 初始挂载时，尝试从 localStorage 读取用户信息，提供"乐观"登录体验，防止刷新立刻跳登录页
  useEffect(() => {
    if (typeof window !== "undefined") {
      try {
        const stored = localStorage.getItem("nowhere.user");

        if (stored) {
          const storedUser = JSON.parse(stored) as User;

          setUser(storedUser);
          console.log("📦 从localStorage恢复用户状态", storedUser);
        }
      } catch (e) {
        console.error("读取本地用户失败", e);
        localStorage.removeItem("nowhere.user");
      }
    }
  }, []);

  // 验证当前用户会话（简化版：仅检查 localStorage 和 token 是否存在）
  // 实际的认证验证由后端 JWT 中间件处理，401 响应会由 fetch 拦截器自动清理
  const checkAuth = async (forceCheck = false) => {
    console.log("🔍 检查本地认证状态", {
      forceCheck,
      user: user?.username,
      loading,
    });

    // 如果正在加载中且不是强制检查，则跳过
    if (loading && !forceCheck) {
      console.log("⚡ 跳过检查（正在加载中）");
      return;
    }

    setLoading(true);

    try {
      // 检查 token 是否存在
      const token = getToken();
      if (!token) {
        console.log("❌ 没有找到 token，清除用户状态");
        setUser(null);
        clearToken();
        if (typeof window !== "undefined") {
          localStorage.removeItem("nowhere.user");
        }
        return;
      }

      // 检查 token 是否过期（本地验证）
      const expiresAtStr = localStorage.getItem("nowhere.tokenExpiresAt");
      if (expiresAtStr) {
        const expiresAt = new Date(expiresAtStr);
        if (new Date() > expiresAt) {
          console.log("❌ Token 已过期，清除用户状态");
          setUser(null);
          clearToken();
          if (typeof window !== "undefined") {
            localStorage.removeItem("nowhere.user");
          }
          return;
        }
      }

      // Token 存在且未过期，从 localStorage 恢复用户信息（如果还没有）
      if (!user && typeof window !== "undefined") {
        const stored = localStorage.getItem("nowhere.user");
        if (stored) {
          try {
            const storedUser = JSON.parse(stored) as User;
            setUser(storedUser);
            console.log("✅ 从 localStorage 恢复用户状态", storedUser);
          } catch (e) {
            console.error("读取本地用户失败", e);
            localStorage.removeItem("nowhere.user");
          }
        }
      }

      setLastCheckTime(Date.now());
    } catch (error) {
      console.error("🚨 检查认证状态失败:", error);
      setUser(null);
      clearToken();
    } finally {
      setLoading(false);
    }
  };

  // 登出函数
  const logout = async () => {
    console.log("👋 开始登出流程");
    setLoading(true);

    try {
      const token = getToken();
      await fetch(buildApiUrl("/api/auth/logout"), {
        method: "POST",
        headers: token
          ? {
              Authorization: `Bearer ${token}`,
            }
          : {},
      });
      console.log("✅ 登出请求完成");
    } catch (error) {
      console.error("🚨 登出请求失败:", error);
    } finally {
      // 清除用户状态、token 和本地存储
      setUser(null);
      clearToken();
      if (typeof window !== "undefined") {
        localStorage.removeItem("nowhere.user");
      }

      // 延迟跳转，确保状态清理完成
      setTimeout(() => {
        setLoading(false);
        navigate("/login", { replace: true });
      }, 100);
    }
  };

  useEffect(() => {
    // 延迟执行初始身份验证检查，让localStorage恢复先完成
    const timeoutId = setTimeout(() => {
      checkAuth(true); // 初始检查强制执行
    }, 200);

    return () => clearTimeout(timeoutId);
  }, []);

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        logout,
        checkAuth,
        setUserDirectly: setUser,
        getToken,
        setToken,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}
