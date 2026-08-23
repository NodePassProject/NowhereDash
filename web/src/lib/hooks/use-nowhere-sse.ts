import { useEffect, useRef, useState, useCallback } from "react";

import { buildApiUrl, getAuthToken } from "@/lib/utils";

interface NowhereSSEOptions {
  onMessage?: (data: any) => void;
  onError?: (error: any) => void;
  onConnected?: () => void;
  onDisconnected?: () => void;
  autoReconnect?: boolean; // 是否自动重连，默认false
}

interface NowhereEndpoint {
  url: string;
  apiPath: string;
  apiKey: string;
}

export function useNowhereSSE(
  endpoint: NowhereEndpoint | null,
  options: NowhereSSEOptions = {},
) {
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);
  const eventListenersRef = useRef<Map<string, (event: MessageEvent) => void>>(
    new Map(),
  );

  // 使用useCallback确保回调函数引用稳定
  const onMessage = useCallback(options.onMessage || (() => {}), [
    options.onMessage,
  ]);
  const onError = useCallback(options.onError || (() => {}), [options.onError]);
  const onConnected = useCallback(options.onConnected || (() => {}), [
    options.onConnected,
  ]);
  const onDisconnected = useCallback(options.onDisconnected || (() => {}), [
    options.onDisconnected,
  ]);

  const cleanup = useCallback(() => {
    // 强制清理所有事件监听器，防止内存泄漏
    if (eventSourceRef.current && eventListenersRef.current.size > 0) {
      eventListenersRef.current.forEach((listener, eventType) => {
        try {
          eventSourceRef.current?.removeEventListener(eventType, listener);
        } catch (error) {
          // 静默处理移除失败的情况
          console.debug(`[Nowhere SSE] 移除事件监听器失败: ${eventType}`);
        }
      });
      eventListenersRef.current.clear();
    }

    // 强制关闭EventSource连接
    if (eventSourceRef.current) {
      try {
        eventSourceRef.current.close();
      } catch (error) {
        // 静默处理关闭失败的情况
        console.debug("[Nowhere SSE] 关闭EventSource失败");
      } finally {
        eventSourceRef.current = null;
      }
    }

    // 清理AbortController
    if (abortControllerRef.current) {
      try {
        abortControllerRef.current.abort();
      } catch (error) {
        // 静默处理abort失败的情况
        console.debug("[Nowhere SSE] AbortController abort失败");
      } finally {
        abortControllerRef.current = null;
      }
    }
  }, []);

  const connect = useCallback(
    (endpoint: NowhereEndpoint) => {
      // 如果已经在连接中，避免重复连接
      if (isConnecting || eventSourceRef.current) {
        console.log("[Nowhere SSE] 连接已存在，跳过重复连接");

        return;
      }

      // 检查endpoint参数是否有效
      if (!endpoint || !endpoint.url || !endpoint.apiPath || !endpoint.apiKey) {
        console.error("[Nowhere SSE] endpoint参数无效:", endpoint);
        setError("endpoint配置无效");

        return;
      }

      try {
        cleanup();
        setIsConnecting(true);
        setError(null);

        // 使用后端代理接口连接Nowhere SSE
        const token = getAuthToken();
        const proxyUrl = buildApiUrl(
          `/api/sse/nowhere-proxy?endpointId=${btoa(
            JSON.stringify({
              url: endpoint.url,
              apiPath: endpoint.apiPath,
              apiKey: endpoint.apiKey,
            }),
          )}`,
        );

        console.log("[Nowhere SSE] 通过代理连接:", proxyUrl);

        const fullUrl = new URL(proxyUrl, window.location.origin);
        if (token) fullUrl.searchParams.set("token", token);

        console.log("[Nowhere SSE] 完整URL:", fullUrl.toString());

        const eventSource = new EventSource(fullUrl.toString());

        // 存储EventSource引用以便清理
        eventSourceRef.current = eventSource;
        abortControllerRef.current = {
          abort: () => eventSource.close(),
        } as AbortController;

        eventSource.onopen = () => {
          console.log("[Nowhere SSE] 代理连接已建立");
          setIsConnected(true);
          setIsConnecting(false);
          setError(null);

          // 触发连接成功回调
          onConnected();
        };

        const processEvent = (event: MessageEvent) => {
          console.log("[Nowhere SSE] ========== 新消息开始 ==========");
          console.log("[Nowhere SSE] 原始事件数据:", event.data);
          console.log("[Nowhere SSE] 数据类型:", typeof event.data);

          try {
            // 首先尝试解析为JSON
            const data = JSON.parse(event.data);

            console.log("[Nowhere SSE] JSON解析成功:", data);
            console.log("[Nowhere SSE] 消息类型:", data.type);

            console.log("[Nowhere SSE] 调用onMessage回调，传递数据:", data);
            onMessage(data);
            console.log("[Nowhere SSE] onMessage回调调用完成");

            // 检查是否为连接确认消息
            if (data.type === "connected") {
              console.log("[Nowhere SSE] 收到连接确认消息");
              setIsConnected(true);
              setIsConnecting(false);
              setError(null);

              return;
            }

            // 检查是否为错误消息
            if (data.type === "error") {
              console.error("[Nowhere SSE] 收到错误消息:", data.message);
              setError(data.message);
              setIsConnected(false);
              setIsConnecting(false);

              return;
            }
          } catch (parseError) {
            // 如果不是JSON，当作纯文本日志处理
            console.log(
              "[Nowhere SSE] JSON解析失败，作为文本处理:",
              parseError instanceof Error
                ? parseError.message
                : String(parseError),
            );
            console.log("[Nowhere SSE] 文本消息内容:", event.data);

            console.log("[Nowhere SSE] 调用onMessage回调处理文本消息");
            onMessage({
              type: "log",
              message: event.data,
            });
            console.log("[Nowhere SSE] 文本消息处理完成");
          }

          console.log("[Nowhere SSE] ========== 消息处理结束 ==========");
        };

        eventSource.onmessage = processEvent;

        // 注册统一事件处理器，兼容自定义事件类型（例如 instance、tunnel 等）
        const handleEvent = (event: MessageEvent) => {
          console.log("[Nowhere SSE] ==== 自定义事件 ====", event.type);
          console.log("[Nowhere SSE] 自定义事件数据:", event.data);
          processEvent(event);
        };

        // 监听常见的自定义事件类型，减少事件类型以降低内存使用
        const customEventTypes = ["instance", "tunnel", "stats"];

        customEventTypes.forEach((evt) => {
          console.log("[Nowhere SSE] 注册自定义事件监听器:", evt);
          try {
            eventSource.addEventListener(evt, handleEvent as EventListener);
            eventListenersRef.current.set(evt, handleEvent);
          } catch (error) {
            console.warn(`[Nowhere SSE] 注册事件监听器失败: ${evt}`, error);
          }
        });

        // 错误事件处理
        eventSource.onerror = (error) => {
          console.error("[Nowhere SSE] 连接错误:", error);
          setIsConnecting(false);
          setIsConnected(false);
          setError("连接失败");

          // 手动模式，直接关闭连接
          console.log("[Nowhere SSE] 手动模式，关闭连接");
          cleanup();

          onError(error);
          onDisconnected();
        };
      } catch (error) {
        console.error("[Nowhere SSE] 创建连接失败:", error);
        setIsConnecting(false);
        setIsConnected(false);

        const errorMessage =
          error instanceof Error ? error.message : String(error);

        setError(errorMessage);

        onError(error);
      }
    },
    [
      cleanup,
      onMessage,
      onError,
      onConnected,
      onDisconnected,
      options.autoReconnect,
      isConnecting,
    ],
  );

  // 手动连接功能
  const connectManually = useCallback(() => {
    if (endpoint && !isConnecting) {
      console.log("[Nowhere SSE] 手动连接");
      connect(endpoint);
    }
  }, [endpoint, isConnecting, connect]);

  // 手动断开功能
  const disconnect = useCallback(() => {
    console.log("[Nowhere SSE] 手动断开连接");
    cleanup();
    setIsConnected(false);
    setIsConnecting(false);
    setError(null);
  }, [cleanup]);

  // 手动重连功能
  const reconnect = useCallback(() => {
    if (endpoint && !isConnecting) {
      console.log("[Nowhere SSE] 手动重连");
      cleanup(); // 确保清理之前的连接
      setError(null);
      connect(endpoint);
    }
  }, [endpoint, isConnecting, cleanup, connect]);

  // 组件卸载时清理连接
  useEffect(() => {
    return () => {
      console.log("[Nowhere SSE] 组件卸载，清理连接");
      cleanup();
      setIsConnected(false);
    };
  }, [cleanup]);

  return {
    isConnected,
    isConnecting,
    error,
    connect: connectManually,
    disconnect,
    reconnect,
  };
}
