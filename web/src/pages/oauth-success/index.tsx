import { useEffect } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Spinner } from "@heroui/react";
import { useTranslation } from "react-i18next";

import { useAuth } from "@/components/auth/auth-provider";

/**
 * OAuth 认证成功页面
 * 用于接收 OAuth2 回调返回的 token，并保存到 localStorage
 */
export default function OAuthSuccessPage() {
  const navigate = useNavigate();
  const { t } = useTranslation("oauth");
  const [searchParams] = useSearchParams();
  const { setToken, setUserDirectly } = useAuth();

  useEffect(() => {
    const handleOAuthSuccess = async () => {
      // 从 URL 参数中提取 token 和用户信息
      const token = searchParams.get("token");
      const expiresAt = searchParams.get("expiresAt");
      const username = searchParams.get("username");

      console.log("🔐 OAuth 认证成功，接收 token", {
        token: token ? `${token.substring(0, 20)}...` : null,
        expiresAt,
        username,
      });

      if (!token || !username) {
        console.error("❌ OAuth 回调缺少必要参数");
        navigate("/login", { replace: true });
        return;
      }

      // 保存 token
      setToken(token, expiresAt || undefined);

      // 保存用户信息
      const user = { username };
      setUserDirectly(user);
      localStorage.setItem("nowhere.user", JSON.stringify(user));

      console.log("✅ Token 和用户信息已保存，即将跳转到仪表盘");

      // 延迟跳转，确保状态更新完成
      setTimeout(() => {
        navigate("/dashboard", { replace: true });
      }, 500);
    };

    handleOAuthSuccess();
  }, [searchParams, navigate, setToken, setUserDirectly]);

  return (
    <div className="min-h-screen flex items-center justify-center bg-gradient-to-br from-background to-default-100">
      <div className="text-center">
        <Spinner size="lg" />
        <p className="mt-4 text-default-500">{t("success.processing")}</p>
      </div>
    </div>
  );
}
