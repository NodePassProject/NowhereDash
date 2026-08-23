import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  DatePicker,
  Divider,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Spinner,
  Switch,
  Tab,
  Tabs,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { parseDate } from "@internationalized/date";
import {
  type ComponentProps,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useSearchParams } from "react-router-dom";

import CellValue from "./cell-value";
import { FullscreenChartModal } from "./fullscreen-chart-modal";

import PortalVectorQrModal from "@/components/tunnels/portal-vector-qr-modal";
import PortalTagModal from "@/components/tunnels/portal-tag-modal";
import SimpleCreateTunnelModal from "@/components/tunnels/simple-create-tunnel-modal";
import { useSettings } from "@/components/providers/settings-provider";
import { ConnectionsChart } from "@/components/ui/connections-chart";
import { DetailedTrafficChart } from "@/components/ui/detailed-traffic-chart";
import { FileLogViewer } from "@/components/ui/file-log-viewer";
import { LatencyChart } from "@/components/ui/latency-chart";
import { Snippet } from "@/components/ui/snippet";
import { SpeedChart } from "@/components/ui/speed-chart";
import TunnelStatsCharts from "@/components/ui/tunnel-stats-charts";
import { useMetricsTrend } from "@/lib/hooks/use-metrics-trend";
import { useTunnelSSE } from "@/lib/hooks/use-sse";
import { buildPortalUrl, deriveVectorUrl } from "@/lib/portal-url";
import { buildApiUrl, maskAddress } from "@/lib/utils";

interface EndpointInfo {
  id: string | number;
  name: string;
  hostname?: string;
  url?: string;
  ver?: string;
  version?: string;
}

interface PeerMetadata {
  sid?: string | null;
  type?: string | null;
  alias?: string | null;
}

interface PortalTunnel {
  id: string | number;
  instanceId?: string;
  type: "portal";
  name: string;
  endpointId: string | number;
  status: "running" | "stopped" | "error" | "offline";
  listenHost: string;
  listenPort: string | number;
  sharedKey?: string;
  tlsMode?: string;
  certPath?: string;
  keyPath?: string;
  logLevel?: string;
  commandLine?: string | null;
  configLine?: string | null;
  restart?: boolean;
  network?: string;
  alpn?: string;
  rate?: number;
  etar?: number;
  dial?: string;
  socks?: string;
  sni?: string;
  tags?: Record<string, string> | null;
  peer?: PeerMetadata | null;
  tcpRx?: number;
  tcpTx?: number;
  udpRx?: number;
  udpTx?: number;
  tcps?: number | null;
  udps?: number | null;
  pool?: number | null;
  ping?: number | null;
  createdAt?: string;
  updatedAt?: string;
}

interface DetailResponse {
  tunnel: PortalTunnel;
  endpoint: EndpointInfo;
  commandURL?: string;
  configURL?: string;
  portalHost?: string;
  vectorUrl?: string;
}

interface MetricSeries {
  created_at?: number[];
  avg_delay?: number[];
}

interface PortalMetrics {
  traffic?: MetricSeries;
  ping?: MetricSeries;
  pool?: MetricSeries;
  tcps?: MetricSeries;
  udps?: MetricSeries;
  speed_in?: MetricSeries;
  speed_out?: MetricSeries;
  tcp_in?: MetricSeries;
  tcp_out?: MetricSeries;
  udp_in?: MetricSeries;
  udp_out?: MetricSeries;
}

type ChartTab = "traffic" | "speed" | "latency" | "connections";

interface FileLogViewerHandle {
  appendLog: (content: string) => void;
  clear: () => void;
  clearDisplay: () => void;
  exportLogs: () => boolean;
  scrollToBottom: () => void;
}

const getToday = () => {
  const now = new Date();
  const localTime = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);

  return localTime.toISOString().slice(0, 10);
};

const getFileLogViewer = () =>
  (
    window as Window & {
      fileLogViewerRef?: FileLogViewerHandle;
    }
  ).fileLogViewerRef;

const formatTrafficValue = (bytes = 0) => {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = Math.max(0, bytes);
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  return {
    value: value.toFixed(unitIndex === 0 ? 0 : 2),
    unit: units[unitIndex],
  };
};

const formatBytes = (bytes = 0) => {
  const { value, unit } = formatTrafficValue(bytes);

  return `${value} ${unit}`;
};

const valueOrDefault = (value: unknown, fallback = "none") => {
  if (value === undefined || value === null || value === "") return fallback;

  return String(value);
};

const portalUrlFromValue = (value?: string | null) =>
  value?.match(/portal:\/\/[^\s"']+/)?.[0] ?? "";

const formatDate = (value: string | undefined, locale: string) => {
  if (!value) return "-";

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) return value;

  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
};

const getMetricSeries = (
  metrics: PortalMetrics | undefined,
  key: keyof PortalMetrics,
) => ({
  timestamps: metrics?.[key]?.created_at ?? [],
  values: metrics?.[key]?.avg_delay ?? [],
});

const mergeTimestamps = (...series: MetricSeries[]) =>
  [...new Set(series.flatMap((item) => item.created_at ?? []))].sort(
    (left, right) => left - right,
  );

const valueAt = (series: MetricSeries | undefined, timestamp: number) => {
  const index = series?.created_at?.indexOf(timestamp) ?? -1;

  return index >= 0 ? (series?.avg_delay?.[index] ?? 0) : 0;
};

const transformTrafficData = (metrics: PortalMetrics | undefined) => {
  const { timestamps, values } = getMetricSeries(metrics, "traffic");

  return timestamps.map((timestamp, index) => ({
    timeStamp: new Date(timestamp).toISOString(),
    traffic: values[index] ?? 0,
  }));
};

const transformDetailedTrafficData = (metrics: PortalMetrics | undefined) => {
  const timestamps = mergeTimestamps(
    metrics?.tcp_in ?? {},
    metrics?.tcp_out ?? {},
    metrics?.udp_in ?? {},
    metrics?.udp_out ?? {},
  );

  return timestamps.map((timestamp) => ({
    timeStamp: new Date(timestamp).toISOString(),
    tcpIn: valueAt(metrics?.tcp_in, timestamp),
    tcpOut: valueAt(metrics?.tcp_out, timestamp),
    udpIn: valueAt(metrics?.udp_in, timestamp),
    udpOut: valueAt(metrics?.udp_out, timestamp),
  }));
};

const transformSpeedData = (metrics: PortalMetrics | undefined) => {
  const timestamps = mergeTimestamps(
    metrics?.speed_in ?? {},
    metrics?.speed_out ?? {},
  );

  return timestamps.map((timestamp) => ({
    timeStamp: new Date(timestamp).toISOString(),
    speed_in: valueAt(metrics?.speed_in, timestamp),
    speed_out: valueAt(metrics?.speed_out, timestamp),
  }));
};

const transformLatencyData = (metrics: PortalMetrics | undefined) => {
  const { timestamps, values } = getMetricSeries(metrics, "ping");

  return timestamps.map((timestamp, index) => ({
    timeStamp: new Date(timestamp).toISOString(),
    latency: values[index] ?? 0,
  }));
};

const transformConnectionsData = (metrics: PortalMetrics | undefined) => {
  const timestamps = mergeTimestamps(
    metrics?.pool ?? {},
    metrics?.tcps ?? {},
    metrics?.udps ?? {},
  );

  return timestamps.map((timestamp) => ({
    timeStamp: new Date(timestamp).toISOString(),
    pool: Math.round(valueAt(metrics?.pool, timestamp)),
    tcps: Math.round(valueAt(metrics?.tcps, timestamp)),
    udps: Math.round(valueAt(metrics?.udps, timestamp)),
  }));
};

export default function TunnelDetailPage() {
  const navigate = useNavigate();
  const { settings } = useSettings();
  const [searchParams] = useSearchParams();
  const id = searchParams.get("id") ?? "";
  const { i18n } = useTranslation();
  const zh = i18n.language.startsWith("zh");
  const [detail, setDetail] = useState<DetailResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [actionPending, setActionPending] = useState<
    "start" | "stop" | "restart" | null
  >(null);
  const [editOpen, setEditOpen] = useState(false);
  const [tagsOpen, setTagsOpen] = useState(false);
  const [qrOpen, setQrOpen] = useState(false);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [resetOpen, setResetOpen] = useState(false);
  const [resetting, setResetting] = useState(false);
  const [showKey, setShowKey] = useState(false);
  const [showConfigLine, setShowConfigLine] = useState(true);
  const [updatingRestart, setUpdatingRestart] = useState(false);
  const [selectedStatsTab, setSelectedStatsTab] = useState<ChartTab>("traffic");
  const [fullscreenOpen, setFullscreenOpen] = useState(false);
  const [logDate, setLogDate] = useState(getToday);
  const [logRefreshTrigger, setLogRefreshTrigger] = useState(0);
  const [logLoading, setLogLoading] = useState(false);
  const [logClearing, setLogClearing] = useState(false);
  const [logCount, setLogCount] = useState(0);
  const [clearLogsOpen, setClearLogsOpen] = useState(false);
  const [isRealtimeLogging, setIsRealtimeLogging] = useState(false);

  const copy = useMemo(
    () =>
      zh
        ? {
            back: "返回隧道管理",
            portal: "隧道",
            running: "运行中",
            stopped: "已停止",
            error: "异常",
            offline: "离线",
            edit: "编辑隧道",
            start: "启动",
            stop: "停止",
            restart: "重启",
            reset: "重置流量",
            delete: "删除",
            refresh: "刷新",
            actionSuccess: "操作成功",
            actionFailed: "操作失败",
            loadFailed: "加载隧道详情失败",
            invalidId: "缺少隧道 ID",
            traffic: "流量统计",
            tcpTraffic: "TCP 流量",
            udpTraffic: "UDP 流量",
            connections: "连接统计",
            tcpConnections: "TCP 连接",
            udpConnections: "UDP 连接",
            networkQuality: "网络质量",
            latency: "延迟",
            pool: "连接池",
            portalInfo: "隧道信息",
            instanceId: "实例 ID",
            endpoint: "节点",
            version: "版本",
            listen: "监听地址",
            sharedKey: "共享密钥",
            hideKey: "隐藏共享密钥",
            showKey: "显示共享密钥",
            transport: "传输模式",
            tls: "TLS 模式",
            tlsSelfSigned: "模式1：自签名证书",
            tlsCustom: "模式 2：自定义证书",
            cert: "证书路径",
            key: "私钥路径",
            logLevel: "日志级别",
            outbound: "出口地址",
            inboundRate: "入口限速",
            outboundRate: "出口限速",
            socks: "SOCKS 出口",
            autoRestart: "自动重启",
            createdAt: "创建时间",
            updatedAt: "更新时间",
            showCommandURL: "显示创建 URL",
            showConfigURL: "显示运行配置 URL",
            qr: "Vector 二维码",
            vectorUnavailable:
              "无法生成 Vector URL，请先配置节点公网 hostname。",
            tags: "Tags",
            manageTags: "管理 Tags",
            noTags: "暂无 Tags",
            charts: {
              traffic: "流量",
              speed: "速率",
              latency: "延迟",
              connections: "连接数",
            },
            legends: {
              tcpIn: "TCP 入站",
              tcpOut: "TCP 出站",
              udpIn: "UDP 入站",
              udpOut: "UDP 出站",
              upload: "上传",
              download: "下载",
              pool: "池",
              tcp: "TCP",
              udp: "UDP",
            },
            fullscreen: "放大图表",
            refreshChart: "刷新图表",
            logs: "日志",
            realtime: "实时",
            realtimeOutput: "实时输出",
            logDate: "日志日期",
            scrollBottom: "滚动到底部",
            exportLogs: "导出日志",
            clearLogs: "清空日志",
            confirmClearLogs: "确认清空日志",
            clearLogsWarning:
              "此操作将清空该隧道的全部已保存日志，且无法撤销。",
            clearRealtimeWarning: "此操作只会清空当前页面的实时输出。",
            clearNow: "确认清空",
            resetTitle: "重置隧道流量",
            resetMessage: "确定重置 {{name}} 的流量统计吗？",
            resetWarning: "累计流量将归零，此操作无法撤销。",
            confirmReset: "确认重置",
            resetSuccess: "流量统计已重置",
            deleteTitle: "删除隧道",
            deleteMessage: "确定删除 {{name}} 吗？",
            deleteWarning: "删除后将同时从 OpenCtrl 移除此隧道，且无法撤销。",
            cancel: "取消",
            confirmDelete: "确认删除",
          }
        : {
            back: "Back to tunnels",
            portal: "Tunnel",
            running: "Running",
            stopped: "Stopped",
            error: "Error",
            offline: "Offline",
            edit: "Edit Tunnel",
            start: "Start",
            stop: "Stop",
            restart: "Restart",
            reset: "Reset traffic",
            delete: "Delete",
            refresh: "Refresh",
            actionSuccess: "Action completed",
            actionFailed: "Action failed",
            loadFailed: "Failed to load Tunnel details",
            invalidId: "Tunnel ID is missing",
            traffic: "Traffic",
            tcpTraffic: "TCP traffic",
            udpTraffic: "UDP traffic",
            connections: "Connections",
            tcpConnections: "TCP connections",
            udpConnections: "UDP connections",
            networkQuality: "Network quality",
            latency: "Latency",
            pool: "Pool",
            portalInfo: "Tunnel information",
            instanceId: "Instance ID",
            endpoint: "Node",
            version: "Version",
            listen: "Listen address",
            sharedKey: "Shared key",
            hideKey: "Hide shared key",
            showKey: "Show shared key",
            transport: "Transport",
            tls: "TLS mode",
            tlsSelfSigned: "Mode 1: Self-signed certificate",
            tlsCustom: "Mode 2: Custom certificate",
            cert: "Certificate path",
            key: "Private key path",
            logLevel: "Log level",
            outbound: "Outbound address",
            inboundRate: "Ingress rate",
            outboundRate: "Egress rate",
            socks: "SOCKS egress",
            autoRestart: "Auto restart",
            createdAt: "Created",
            updatedAt: "Updated",
            showCommandURL: "Show creation URL",
            showConfigURL: "Show runtime config URL",
            qr: "Vector QR code",
            vectorUnavailable:
              "Vector URL is unavailable. Configure the node's public hostname first.",
            tags: "Tags",
            manageTags: "Manage tags",
            noTags: "No tags",
            charts: {
              traffic: "Traffic",
              speed: "Speed",
              latency: "Latency",
              connections: "Connections",
            },
            legends: {
              tcpIn: "TCP inbound",
              tcpOut: "TCP outbound",
              udpIn: "UDP inbound",
              udpOut: "UDP outbound",
              upload: "Upload",
              download: "Download",
              pool: "Pool",
              tcp: "TCP",
              udp: "UDP",
            },
            fullscreen: "Expand chart",
            refreshChart: "Refresh chart",
            logs: "Logs",
            realtime: "Live",
            realtimeOutput: "Live output",
            logDate: "Log date",
            scrollBottom: "Scroll to bottom",
            exportLogs: "Export logs",
            clearLogs: "Clear logs",
            confirmClearLogs: "Clear saved logs?",
            clearLogsWarning:
              "This clears every saved log for this Tunnel and cannot be undone.",
            clearRealtimeWarning:
              "This only clears the live output currently shown on this page.",
            clearNow: "Clear logs",
            resetTitle: "Reset Tunnel traffic",
            resetMessage: "Reset traffic counters for {{name}}?",
            resetWarning:
              "Accumulated traffic will be cleared and cannot be restored.",
            confirmReset: "Reset traffic",
            resetSuccess: "Traffic counters reset",
            deleteTitle: "Delete Tunnel",
            deleteMessage: "Delete {{name}}?",
            deleteWarning:
              "This removes the Tunnel from OpenCtrl and cannot be undone.",
            cancel: "Cancel",
            confirmDelete: "Delete Tunnel",
          },
    [zh],
  );

  const fetchDetail = useCallback(
    async (quiet = false) => {
      if (!id) {
        setLoading(false);
        setDetail(null);

        return;
      }

      quiet ? setRefreshing(true) : setLoading(true);
      try {
        const response = await fetch(buildApiUrl(`/api/tunnels/${id}/details`));
        const body = await response.json();

        if (!response.ok) throw new Error(body.error || copy.loadFailed);

        const tunnel = body.tunnel ?? body;
        const endpoint = body.endpoint ?? tunnel.endpoint ?? {};

        setDetail({
          tunnel,
          endpoint,
          commandURL: body.commandURL,
          configURL: body.configURL,
          portalHost: body.portalHost,
          vectorUrl: body.vectorUrl,
        });
      } catch (error) {
        addToast({
          title: copy.loadFailed,
          description: error instanceof Error ? error.message : copy.loadFailed,
          color: "danger",
        });
      } finally {
        setLoading(false);
        setRefreshing(false);
      }
    },
    [copy.loadFailed, id],
  );

  useEffect(() => {
    void fetchDetail();
  }, [fetchDetail]);

  useEffect(() => {
    if (!id) return;

    const timer = window.setInterval(() => void fetchDetail(true), 15000);

    return () => window.clearInterval(timer);
  }, [fetchDetail, id]);

  useEffect(() => {
    if (settings.isPrivacyMode) setShowKey(false);
  }, [settings.isPrivacyMode]);

  const metricsInstanceId = detail?.tunnel.instanceId ?? "";
  const {
    data: metricsData,
    loading: metricsLoading,
    error: metricsError,
    refresh: refreshMetrics,
  } = useMetricsTrend({
    tunnelId: metricsInstanceId,
    autoRefresh: Boolean(metricsInstanceId),
    refreshInterval: 15000,
  });

  const handleSSEMessage = useCallback((data: unknown) => {
    if (!data || typeof data !== "object") return;

    const event = data as { type?: string; logs?: unknown; message?: unknown };

    if (event.type !== "log") return;

    const content =
      typeof event.logs === "string"
        ? event.logs
        : typeof event.message === "string"
          ? event.message
          : "";

    if (content) getFileLogViewer()?.appendLog(content);
  }, []);

  useTunnelSSE(metricsInstanceId, {
    enabled: isRealtimeLogging,
    onMessage: handleSSEMessage,
  });

  const chartData = useMemo(() => {
    const metrics = metricsData?.data as unknown as PortalMetrics | undefined;

    return {
      traffic: transformTrafficData(metrics),
      detailedTraffic: transformDetailedTrafficData(metrics),
      speed: transformSpeedData(metrics),
      latency: transformLatencyData(metrics),
      connections: transformConnectionsData(metrics),
    };
  }, [metricsData]);

  const runAction = async (action: "start" | "stop" | "restart") => {
    if (!detail || actionPending) return;

    setActionPending(action);
    try {
      const response = await fetch(
        buildApiUrl(`/api/tunnels/${detail.tunnel.id}/action`),
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action }),
        },
      );
      const body = await response.json();

      if (!response.ok || body.success === false) {
        throw new Error(body.error || body.message || copy.actionFailed);
      }

      addToast({ title: copy.actionSuccess, color: "success" });
      await fetchDetail(true);
    } catch (error) {
      addToast({
        title: copy.actionFailed,
        description: error instanceof Error ? error.message : copy.actionFailed,
        color: "danger",
      });
    } finally {
      setActionPending(null);
    }
  };

  const updateRestart = async (restart: boolean) => {
    if (!detail) return;

    setUpdatingRestart(true);
    try {
      const response = await fetch(
        buildApiUrl(`/api/tunnels/${detail.tunnel.id}/restart`),
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ restart }),
        },
      );
      const body = await response.json();

      if (!response.ok || body.success === false) {
        throw new Error(body.error || body.message || copy.actionFailed);
      }

      setDetail((current) =>
        current
          ? { ...current, tunnel: { ...current.tunnel, restart } }
          : current,
      );
    } catch (error) {
      addToast({
        title: copy.actionFailed,
        description: error instanceof Error ? error.message : copy.actionFailed,
        color: "danger",
      });
    } finally {
      setUpdatingRestart(false);
    }
  };

  const confirmReset = async () => {
    if (!detail) return;

    setResetting(true);
    try {
      const response = await fetch(
        buildApiUrl(`/api/tunnels/${detail.tunnel.id}`),
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action: "reset" }),
        },
      );
      const body = await response.json();

      if (!response.ok || body.success === false) {
        throw new Error(body.error || body.message || copy.actionFailed);
      }

      addToast({ title: copy.resetSuccess, color: "success" });
      setResetOpen(false);
      await Promise.all([fetchDetail(true), refreshMetrics()]);
    } catch (error) {
      addToast({
        title: copy.actionFailed,
        description: error instanceof Error ? error.message : copy.actionFailed,
        color: "danger",
      });
    } finally {
      setResetting(false);
    }
  };

  const confirmDelete = async () => {
    if (!detail) return;

    setDeleting(true);
    try {
      const response = await fetch(
        buildApiUrl(`/api/tunnels/${detail.tunnel.id}`),
        { method: "DELETE" },
      );
      const body = await response.json();

      if (!response.ok || body.success === false) {
        throw new Error(body.error || body.message || copy.actionFailed);
      }

      navigate("/tunnels");
    } catch (error) {
      addToast({
        title: copy.actionFailed,
        description: error instanceof Error ? error.message : copy.actionFailed,
        color: "danger",
      });
      setDeleting(false);
    }
  };

  const handleRefresh = async () => {
    await Promise.all([fetchDetail(true), refreshMetrics()]);
  };

  const handleRealtimeLoggingToggle = (enabled: boolean) => {
    setIsRealtimeLogging(enabled);

    if (enabled) {
      getFileLogViewer()?.clearDisplay();

      return;
    }

    setLogDate(getToday());
    setLogRefreshTrigger((value) => value + 1);
  };

  if (loading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <Spinner />
      </div>
    );
  }

  if (!detail) {
    return (
      <div className="flex min-h-[400px] flex-col items-center justify-center gap-4">
        <p className="text-default-500">
          {id ? copy.loadFailed : copy.invalidId}
        </p>
        <Button variant="flat" onPress={() => navigate("/tunnels")}>
          {copy.back}
        </Button>
      </div>
    );
  }

  const { tunnel, endpoint } = detail;
  const isRunning = tunnel.status === "running";
  const portalUrl = detail.commandURL || buildPortalUrl(tunnel);
  const configURL =
    detail.configURL || portalUrlFromValue(tunnel.configLine) || "";
  const displayedPortalURL =
    showConfigLine && configURL ? configURL : portalUrl;
  const vectorUrl = detail.vectorUrl || deriveVectorUrl(tunnel, endpoint);
  const tagEntries = Object.entries(tunnel.tags ?? {});
  const listenAddress = `${tunnel.listenHost || "0.0.0.0"}:${tunnel.listenPort}`;
  const displayedListenAddress = maskAddress(
    listenAddress,
    settings.isPrivacyMode,
  );
  const statusText = copy[tunnel.status];
  const statusColor =
    tunnel.status === "running"
      ? "success"
      : tunnel.status === "error"
        ? "danger"
        : tunnel.status === "offline"
          ? "warning"
          : "default";
  const tlsText = tunnel.tlsMode === "2" ? copy.tlsCustom : copy.tlsSelfSigned;
  const fullscreenTitle = copy.charts[selectedStatsTab];
  const chartLegend =
    selectedStatsTab === "traffic"
      ? [
          [copy.legends.tcpIn, "hsl(217 91% 60%)"],
          [copy.legends.tcpOut, "hsl(142 76% 36%)"],
          [copy.legends.udpIn, "hsl(262 83% 58%)"],
          [copy.legends.udpOut, "hsl(25 95% 53%)"],
        ]
      : selectedStatsTab === "speed"
        ? [
            [copy.legends.upload, "hsl(220 70% 50%)"],
            [copy.legends.download, "hsl(280 65% 60%)"],
          ]
        : selectedStatsTab === "connections"
          ? [
              [copy.legends.pool, "hsl(340 75% 55%)"],
              [copy.legends.tcp, "hsl(24 70% 50%)"],
              [copy.legends.udp, "hsl(173 58% 39%)"],
            ]
          : [];

  return (
    <>
      <div className="space-y-4 p-4 md:space-y-6 md:p-0">
        <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between md:gap-0">
          <div className="flex min-w-0 items-center gap-2 md:gap-3">
            <Tooltip content={copy.back}>
              <Button
                isIconOnly
                aria-label={copy.back}
                className="shrink-0 bg-default-100 hover:bg-default-200"
                variant="flat"
                onPress={() => navigate(-1)}
              >
                <Icon icon="lucide:arrow-left" width={18} />
              </Button>
            </Tooltip>
            <h1 className="min-w-0 truncate text-lg font-bold md:text-2xl">
              {tunnel.name}
            </h1>
            <Chip className="shrink-0" color="primary" variant="flat">
              {copy.portal}
            </Chip>
            <Chip className="shrink-0" color={statusColor} variant="flat">
              {statusText}
            </Chip>
          </div>

          <DesktopActions
            actionPending={actionPending}
            isRunning={isRunning}
            metricsLoading={metricsLoading}
            refreshing={refreshing}
            resetting={resetting}
            text={copy}
            onDelete={() => setDeleteOpen(true)}
            onRefresh={() => void handleRefresh()}
            onReset={() => setResetOpen(true)}
            onRun={(action) => void runAction(action)}
          />
          <MobileActions
            actionPending={actionPending}
            isRunning={isRunning}
            metricsLoading={metricsLoading}
            refreshing={refreshing}
            text={copy}
            onDelete={() => setDeleteOpen(true)}
            onRefresh={() => void handleRefresh()}
            onRun={(action) => void runAction(action)}
          />
        </div>

        {settings.isExperimentalMode && (
          <div className="mb-4">
            <TunnelStatsCharts
              instanceId={tunnel.instanceId}
              isExperimentalMode={settings.isExperimentalMode}
            />
          </div>
        )}

        {!settings.isExperimentalMode && (
          <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
            <PortalStatsCard
              icon="lucide:activity"
              iconClassName="text-blue-500"
              left={{
                label: copy.tcpTraffic,
                value: `↑${formatBytes(tunnel.tcpTx)}  ↓${formatBytes(tunnel.tcpRx)}`,
                className:
                  "bg-blue-50 text-blue-700 dark:bg-blue-950/30 dark:text-blue-300",
              }}
              right={{
                label: copy.udpTraffic,
                value: `↑${formatBytes(tunnel.udpTx)}  ↓${formatBytes(tunnel.udpRx)}`,
                className:
                  "bg-green-50 text-green-700 dark:bg-green-950/30 dark:text-green-300",
              }}
              title={copy.traffic}
            />
            <PortalStatsCard
              icon="lucide:network"
              iconClassName="text-blue-500"
              left={{
                label: copy.tcpConnections,
                value: String(tunnel.tcps ?? 0),
                className:
                  "bg-purple-50 text-purple-700 dark:bg-purple-950/30 dark:text-purple-300",
              }}
              right={{
                label: copy.udpConnections,
                value: String(tunnel.udps ?? 0),
                className:
                  "bg-orange-50 text-orange-700 dark:bg-orange-950/30 dark:text-orange-300",
              }}
              title={copy.connections}
            />
            <PortalStatsCard
              icon="lucide:gauge"
              iconClassName="text-blue-500"
              left={{
                label: copy.latency,
                value: tunnel.ping == null ? "-" : `${tunnel.ping} ms`,
                className:
                  "bg-pink-50 text-pink-700 dark:bg-pink-950/30 dark:text-pink-300",
              }}
              right={{
                label: copy.pool,
                value: tunnel.pool == null ? "-" : String(tunnel.pool),
                className:
                  "bg-cyan-50 text-cyan-700 dark:bg-cyan-950/30 dark:text-cyan-300",
              }}
              title={copy.networkQuality}
            />
          </div>
        )}

        <Card className="p-2">
          <CardHeader className="flex items-center justify-between pb-0">
            <h2 className="text-lg font-semibold">{copy.portalInfo}</h2>
            <Tooltip content={copy.edit} placement="top">
              <Button
                isIconOnly
                aria-label={copy.edit}
                size="sm"
                variant="light"
                onPress={() => setEditOpen(true)}
              >
                <Icon icon="lucide:pencil" width={16} />
              </Button>
            </Tooltip>
          </CardHeader>
          <CardBody>
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2 md:gap-4 lg:grid-cols-4">
                <CellValue
                  icon={<InfoIcon icon="lucide:hash" />}
                  label={copy.instanceId}
                  value={
                    <span className="block truncate font-mono text-sm">
                      {tunnel.instanceId || `#${tunnel.id}`}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:server-cog" />}
                  label={copy.endpoint}
                  value={
                    <span className="block truncate">{endpoint.name}</span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:git-branch" />}
                  label={copy.version}
                  value={
                    <Chip color="secondary" size="sm" variant="flat">
                      {endpoint.ver || endpoint.version || "-"}
                    </Chip>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:radio-tower" />}
                  label={copy.listen}
                  value={
                    <Tooltip content={displayedListenAddress}>
                      <span className="block truncate font-mono text-sm">
                        {displayedListenAddress}
                      </span>
                    </Tooltip>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:key-round" />}
                  label={copy.sharedKey}
                  value={
                    <div className="flex min-w-0 items-center gap-1">
                      <span className="min-w-0 flex-1 truncate font-mono text-sm">
                        {showKey && !settings.isPrivacyMode
                          ? tunnel.sharedKey || "-"
                          : tunnel.sharedKey
                            ? "••••••••••••"
                            : "-"}
                      </span>
                      {tunnel.sharedKey && !settings.isPrivacyMode && (
                        <Tooltip
                          content={showKey ? copy.hideKey : copy.showKey}
                        >
                          <Button
                            isIconOnly
                            aria-label={showKey ? copy.hideKey : copy.showKey}
                            className="h-5 min-h-5 w-5 min-w-5"
                            size="sm"
                            variant="light"
                            onPress={() => setShowKey((value) => !value)}
                          >
                            <Icon
                              icon={showKey ? "lucide:eye-off" : "lucide:eye"}
                              width={14}
                            />
                          </Button>
                        </Tooltip>
                      )}
                    </div>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:network" />}
                  label={copy.transport}
                  value={
                    <Chip color="primary" size="sm" variant="flat">
                      {valueOrDefault(tunnel.network, "mix")}
                    </Chip>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:shield-check" />}
                  label={copy.tls}
                  value={
                    <Chip
                      color={tunnel.tlsMode === "2" ? "secondary" : "success"}
                      size="sm"
                      variant="flat"
                    >
                      {tlsText}
                    </Chip>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:file-text" />}
                  label={copy.logLevel}
                  value={
                    <Chip size="sm" variant="flat">
                      {valueOrDefault(tunnel.logLevel, "info").toUpperCase()}
                    </Chip>
                  }
                />
                {tunnel.tlsMode === "2" && (
                  <>
                    <CellValue
                      icon={<InfoIcon icon="lucide:file-key-2" />}
                      label={copy.cert}
                      value={
                        <Tooltip content={valueOrDefault(tunnel.certPath, "-")}>
                          <span className="block truncate font-mono text-sm">
                            {valueOrDefault(tunnel.certPath, "-")}
                          </span>
                        </Tooltip>
                      }
                    />
                    <CellValue
                      icon={<InfoIcon icon="lucide:key-round" />}
                      label={copy.key}
                      value={
                        <Tooltip content={valueOrDefault(tunnel.keyPath, "-")}>
                          <span className="block truncate font-mono text-sm">
                            {valueOrDefault(tunnel.keyPath, "-")}
                          </span>
                        </Tooltip>
                      }
                    />
                  </>
                )}
                <CellValue
                  icon={<InfoIcon icon="lucide:route" />}
                  label={copy.outbound}
                  value={
                    <span className="block truncate font-mono text-sm">
                      {valueOrDefault(tunnel.dial, "auto")}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:badge-check" />}
                  label="ALPN"
                  value={
                    <span className="block truncate font-mono text-sm">
                      {valueOrDefault(tunnel.alpn, "now/1")}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:arrow-down-to-line" />}
                  label={copy.inboundRate}
                  value={
                    <span className="font-mono text-sm">
                      {tunnel.rate ?? 0} Mbps
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:arrow-up-to-line" />}
                  label={copy.outboundRate}
                  value={
                    <span className="font-mono text-sm">
                      {tunnel.etar ?? 0} Mbps
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:shield" />}
                  label="SNI"
                  value={
                    <span className="block truncate font-mono text-sm">
                      {valueOrDefault(tunnel.sni)}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:waypoints" />}
                  label={copy.socks}
                  value={
                    <span className="block truncate font-mono text-sm">
                      {valueOrDefault(tunnel.socks)}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:rotate-ccw" />}
                  label={copy.autoRestart}
                  value={
                    <Switch
                      isDisabled={updatingRestart}
                      isSelected={Boolean(tunnel.restart)}
                      size="sm"
                      onValueChange={(value) => void updateRestart(value)}
                    />
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:file-text" />}
                  label={copy.createdAt}
                  value={
                    <span className="block truncate text-sm">
                      {formatDate(tunnel.createdAt, zh ? "zh-CN" : "en-US")}
                    </span>
                  }
                />
                <CellValue
                  icon={<InfoIcon icon="lucide:refresh-ccw" />}
                  label={copy.updatedAt}
                  value={
                    <span className="block truncate text-sm">
                      {formatDate(tunnel.updatedAt, zh ? "zh-CN" : "en-US")}
                    </span>
                  }
                />
                <CellValue
                  isInteractive
                  icon={<InfoIcon icon="lucide:tags" />}
                  label={copy.tags}
                  value={tagEntries.length ? copy.manageTags : copy.noTags}
                  onPress={() => setTagsOpen(true)}
                />
              </div>

              <Divider className="my-4" />

              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  {configURL && (
                    <Tooltip
                      content={
                        showConfigLine
                          ? copy.showCommandURL
                          : copy.showConfigURL
                      }
                      placement="top"
                    >
                      <Button
                        isIconOnly
                        aria-label={
                          showConfigLine
                            ? copy.showCommandURL
                            : copy.showConfigURL
                        }
                        className="shrink-0"
                        color={showConfigLine ? "primary" : "default"}
                        variant="flat"
                        onPress={() => setShowConfigLine((value) => !value)}
                      >
                        <Icon icon="lucide:terminal" width={18} />
                      </Button>
                    </Tooltip>
                  )}
                  <div className="min-w-0 flex-1">
                    <Snippet
                      color={
                        showConfigLine && configURL ? "primary" : "default"
                      }
                      hideCopyButton={false}
                      hideSymbol={true}
                    >
                      {displayedPortalURL || "-"}
                    </Snippet>
                  </div>
                  <Tooltip content={copy.qr}>
                    <Button
                      isIconOnly
                      aria-label={copy.qr}
                      className="shrink-0"
                      color="primary"
                      isDisabled={!vectorUrl}
                      variant="flat"
                      onPress={() => setQrOpen(true)}
                    >
                      <Icon icon="lucide:qr-code" width={18} />
                    </Button>
                  </Tooltip>
                </div>
                {!vectorUrl && (
                  <p className="text-sm text-warning">
                    {copy.vectorUnavailable}
                  </p>
                )}
              </div>
            </div>
          </CardBody>
        </Card>

        <Card className="p-4">
          <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="overflow-x-auto">
              <Tabs
                selectedKey={selectedStatsTab}
                variant="solid"
                onSelectionChange={(key) =>
                  setSelectedStatsTab(key as ChartTab)
                }
              >
                <Tab key="traffic" title={copy.charts.traffic} />
                <Tab key="speed" title={copy.charts.speed} />
                <Tab key="latency" title={copy.charts.latency} />
                <Tab key="connections" title={copy.charts.connections} />
              </Tabs>
            </div>

            <div className="flex items-center justify-between gap-3 sm:justify-end">
              <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                {chartLegend.map(([label, color]) => (
                  <div key={label} className="flex items-center gap-1">
                    <span
                      className="size-2 shrink-0 rounded-full"
                      style={{ backgroundColor: color }}
                    />
                    <span className="text-xs text-default-600">{label}</span>
                  </div>
                ))}
              </div>
              <Tooltip content={copy.refreshChart}>
                <Button
                  isIconOnly
                  aria-label={copy.refreshChart}
                  className="h-7 w-7 min-w-7"
                  isLoading={metricsLoading}
                  size="sm"
                  variant="light"
                  onPress={() => void refreshMetrics()}
                >
                  <Icon icon="lucide:refresh-cw" width={15} />
                </Button>
              </Tooltip>
              <Tooltip content={copy.fullscreen}>
                <Button
                  isIconOnly
                  aria-label={copy.fullscreen}
                  className="h-7 w-7 min-w-7"
                  size="sm"
                  variant="light"
                  onPress={() => setFullscreenOpen(true)}
                >
                  <Icon icon="lucide:external-link" width={15} />
                </Button>
              </Tooltip>
            </div>
          </div>

          <div className="h-[200px]">
            {selectedStatsTab === "traffic" && (
              <DetailedTrafficChart
                className="h-full w-full"
                data={chartData.detailedTraffic}
                error={metricsError || undefined}
                height={200}
                loading={metricsLoading && !metricsData}
              />
            )}
            {selectedStatsTab === "speed" && (
              <SpeedChart
                className="h-full w-full"
                data={chartData.speed}
                error={metricsError || undefined}
                height={200}
                loading={metricsLoading && !metricsData}
              />
            )}
            {selectedStatsTab === "latency" && (
              <LatencyChart
                className="h-full w-full"
                data={chartData.latency}
                error={metricsError || undefined}
                height={200}
                loading={metricsLoading && !metricsData}
              />
            )}
            {selectedStatsTab === "connections" && (
              <ConnectionsChart
                className="h-full w-full"
                data={chartData.connections}
                error={metricsError || undefined}
                height={200}
                loading={metricsLoading && !metricsData}
              />
            )}
          </div>
        </Card>

        <Card className="p-2">
          <CardHeader className="flex flex-col gap-2 pb-2 sm:flex-row sm:items-center sm:justify-between sm:gap-0 sm:pb-0">
            <div className="flex w-full items-center justify-between gap-3 sm:w-auto sm:justify-start">
              <div className="flex items-center gap-2">
                <Icon
                  className="text-default-600"
                  icon="lucide:terminal"
                  width={18}
                />
                <h2 className="text-lg font-semibold">{copy.logs}</h2>
              </div>
              <div className="flex items-center gap-2 sm:hidden">
                <span className="text-xs text-default-600">
                  {copy.realtime}
                </span>
                <Switch
                  color="primary"
                  isSelected={isRealtimeLogging}
                  size="sm"
                  onValueChange={handleRealtimeLoggingToggle}
                />
              </div>
            </div>
            <div className="flex items-center gap-2 overflow-x-auto">
              <div className="hidden shrink-0 items-center gap-2 sm:flex">
                <span className="text-sm text-default-600">
                  {copy.realtimeOutput}
                </span>
                <Switch
                  color="primary"
                  isSelected={isRealtimeLogging}
                  size="sm"
                  onValueChange={handleRealtimeLoggingToggle}
                />
              </div>
              <DatePicker
                showMonthAndYearPickers
                aria-label={copy.logDate}
                className="w-40 shrink-0"
                granularity="day"
                isDisabled={isRealtimeLogging}
                size="sm"
                value={
                  parseDate(logDate) as unknown as ComponentProps<
                    typeof DatePicker
                  >["value"]
                }
                onChange={(date) => {
                  if (date) setLogDate(date.toString());
                }}
              />
              <Tooltip content={copy.refresh} placement="top">
                <Button
                  isIconOnly
                  aria-label={copy.refresh}
                  className="h-8 w-8 min-w-8"
                  isDisabled={!metricsInstanceId || isRealtimeLogging}
                  isLoading={logLoading}
                  size="sm"
                  variant="flat"
                  onPress={() => setLogRefreshTrigger((value) => value + 1)}
                >
                  <Icon icon="lucide:refresh-cw" width={15} />
                </Button>
              </Tooltip>
              <Tooltip content={copy.scrollBottom} placement="top">
                <Button
                  isIconOnly
                  aria-label={copy.scrollBottom}
                  className="h-8 w-8 min-w-8"
                  size="sm"
                  variant="flat"
                  onPress={() => getFileLogViewer()?.scrollToBottom()}
                >
                  <Icon icon="lucide:arrow-down" width={15} />
                </Button>
              </Tooltip>
              <Tooltip content={copy.exportLogs} placement="top">
                <Button
                  isIconOnly
                  aria-label={copy.exportLogs}
                  className="h-8 w-8 min-w-8"
                  color="primary"
                  isDisabled={isRealtimeLogging || logCount === 0}
                  size="sm"
                  variant="flat"
                  onPress={() => getFileLogViewer()?.exportLogs()}
                >
                  <Icon icon="lucide:download" width={15} />
                </Button>
              </Tooltip>
              <Popover
                isOpen={clearLogsOpen}
                placement="bottom-end"
                onOpenChange={setClearLogsOpen}
              >
                <PopoverTrigger>
                  <Button
                    isIconOnly
                    aria-label={copy.clearLogs}
                    className="h-8 w-8 min-w-8"
                    color="danger"
                    isDisabled={!metricsInstanceId}
                    isLoading={logClearing}
                    size="sm"
                    variant="flat"
                  >
                    <Icon icon="lucide:trash-2" width={15} />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-72 p-3">
                  <div className="space-y-3">
                    <p className="text-sm font-medium">
                      {copy.confirmClearLogs}
                    </p>
                    <p className="text-xs text-default-500">
                      {isRealtimeLogging
                        ? copy.clearRealtimeWarning
                        : copy.clearLogsWarning}
                    </p>
                    <div className="flex gap-2">
                      <Button
                        className="flex-1"
                        color="danger"
                        size="sm"
                        onPress={() => {
                          if (isRealtimeLogging) {
                            getFileLogViewer()?.clearDisplay();
                          } else {
                            getFileLogViewer()?.clear();
                          }
                          setClearLogsOpen(false);
                        }}
                      >
                        {copy.clearNow}
                      </Button>
                      <Button
                        className="flex-1"
                        size="sm"
                        variant="flat"
                        onPress={() => setClearLogsOpen(false)}
                      >
                        {copy.cancel}
                      </Button>
                    </div>
                  </div>
                </PopoverContent>
              </Popover>
            </div>
          </CardHeader>
          <CardBody>
            <FileLogViewer
              date={logDate}
              endpointId={String(endpoint.id || "")}
              instanceId={metricsInstanceId}
              isRealtimeMode={isRealtimeLogging}
              triggerRefresh={logRefreshTrigger}
              onClearingChange={setLogClearing}
              onLoadingChange={setLogLoading}
              onLogsChange={(logs) => setLogCount(logs.length)}
            />
          </CardBody>
        </Card>
      </div>

      {editOpen && (
        <SimpleCreateTunnelModal
          isOpen
          instanceId={String(tunnel.id)}
          mode="edit"
          onOpenChange={setEditOpen}
          onSaved={() => void fetchDetail(true)}
        />
      )}
      <PortalTagModal
        currentTags={tunnel.tags}
        isOpen={tagsOpen}
        tunnelId={String(tunnel.id)}
        onOpenChange={setTagsOpen}
        onSaved={() => void fetchDetail(true)}
      />
      <PortalVectorQrModal
        isOpen={qrOpen}
        vectorUrl={vectorUrl}
        onOpenChange={setQrOpen}
      />
      <FullscreenChartModal
        chartType={selectedStatsTab}
        connectionsData={chartData.connections}
        error={metricsError || undefined}
        isOpen={fullscreenOpen}
        latencyData={chartData.latency}
        loading={metricsLoading}
        speedData={chartData.speed}
        title={fullscreenTitle}
        trafficData={chartData.traffic}
        onOpenChange={setFullscreenOpen}
        onRefresh={() => void refreshMetrics()}
      />
      <Modal isOpen={resetOpen} placement="center" onOpenChange={setResetOpen}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Icon
                  className="text-secondary"
                  icon="lucide:rotate-ccw"
                  width={18}
                />
                {copy.resetTitle}
              </ModalHeader>
              <ModalBody>
                <p className="text-default-600">
                  {copy.resetMessage.replace("{{name}}", tunnel.name)}
                </p>
                <p className="text-sm text-warning">{copy.resetWarning}</p>
              </ModalBody>
              <ModalFooter>
                <Button
                  isDisabled={resetting}
                  variant="light"
                  onPress={onClose}
                >
                  {copy.cancel}
                </Button>
                <Button
                  color="secondary"
                  isLoading={resetting}
                  startContent={<Icon icon="lucide:rotate-ccw" width={16} />}
                  onPress={() => void confirmReset()}
                >
                  {copy.confirmReset}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
      <Modal
        isOpen={deleteOpen}
        placement="center"
        onOpenChange={setDeleteOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Icon
                  className="text-danger"
                  icon="lucide:trash-2"
                  width={18}
                />
                {copy.deleteTitle}
              </ModalHeader>
              <ModalBody>
                <p className="text-default-600">
                  {copy.deleteMessage.replace("{{name}}", tunnel.name)}
                </p>
                <p className="text-sm text-warning">{copy.deleteWarning}</p>
              </ModalBody>
              <ModalFooter>
                <Button isDisabled={deleting} variant="light" onPress={onClose}>
                  {copy.cancel}
                </Button>
                <Button
                  color="danger"
                  isLoading={deleting}
                  startContent={<Icon icon="lucide:trash-2" width={16} />}
                  onPress={() => void confirmDelete()}
                >
                  {copy.confirmDelete}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}

type ActionCopy = {
  start: string;
  stop: string;
  restart: string;
  reset: string;
  delete: string;
  refresh: string;
};

type ActionProps = {
  actionPending: "start" | "stop" | "restart" | null;
  isRunning: boolean;
  metricsLoading: boolean;
  refreshing: boolean;
  resetting?: boolean;
  text: ActionCopy;
  onDelete: () => void;
  onRefresh: () => void;
  onReset?: () => void;
  onRun: (action: "start" | "stop" | "restart") => void;
};

function DesktopActions(props: ActionProps) {
  return (
    <div className="hidden items-center gap-2 overflow-x-auto pb-2 sm:flex md:pb-0">
      <ActionButtons {...props} />
    </div>
  );
}

function MobileActions(props: ActionProps) {
  return (
    <div className="flex items-center gap-2 overflow-x-auto pb-2 sm:hidden md:pb-0">
      <ActionButtons {...props} compact />
    </div>
  );
}

function ActionButtons({
  actionPending,
  isRunning,
  metricsLoading,
  refreshing,
  resetting = false,
  text,
  onDelete,
  onRefresh,
  onReset,
  onRun,
  compact = false,
}: ActionProps & { compact?: boolean }) {
  const size = compact ? "sm" : "md";
  const iconSize = compact ? 15 : 17;

  return (
    <>
      <Button
        className="shrink-0"
        color={isRunning ? "warning" : "success"}
        isLoading={actionPending === (isRunning ? "stop" : "start")}
        size={size}
        startContent={
          <Icon
            icon={isRunning ? "lucide:square" : "lucide:play"}
            width={iconSize}
          />
        }
        variant="flat"
        onPress={() => onRun(isRunning ? "stop" : "start")}
      >
        {isRunning ? text.stop : text.start}
      </Button>
      <Button
        className="shrink-0"
        color="primary"
        isDisabled={!isRunning}
        isLoading={actionPending === "restart"}
        size={size}
        startContent={<Icon icon="lucide:rotate-cw" width={iconSize} />}
        variant="flat"
        onPress={() => onRun("restart")}
      >
        {text.restart}
      </Button>
      <Button
        className="shrink-0"
        color="danger"
        size={size}
        startContent={<Icon icon="lucide:trash-2" width={iconSize} />}
        variant="flat"
        onPress={onDelete}
      >
        {text.delete}
      </Button>
      {!compact && onReset && (
        <Button
          className="shrink-0"
          color="secondary"
          isLoading={resetting}
          startContent={<Icon icon="lucide:rotate-ccw" width={iconSize} />}
          variant="flat"
          onPress={onReset}
        >
          {text.reset}
        </Button>
      )}
      <Button
        className="shrink-0"
        isLoading={refreshing || metricsLoading}
        size={size}
        startContent={<Icon icon="lucide:refresh-cw" width={iconSize} />}
        variant="flat"
        onPress={onRefresh}
      >
        {text.refresh}
      </Button>
    </>
  );
}

function InfoIcon({ icon }: { icon: string }) {
  return <Icon className="text-default-600" icon={icon} width={18} />;
}

function PortalStatsCard({
  title,
  icon,
  iconClassName,
  left,
  right,
}: {
  title: string;
  icon: string;
  iconClassName: string;
  left: { label: string; value: string; className: string };
  right: { label: string; value: string; className: string };
}) {
  return (
    <Card className="h-full p-2">
      <CardHeader className="flex items-center pb-0">
        <Icon className={`mr-1 ${iconClassName}`} icon={icon} width={20} />
        <h2 className="text-base font-semibold">{title}</h2>
      </CardHeader>
      <CardBody>
        <div className="flex w-full overflow-hidden rounded-lg">
          {[left, right].map((item) => (
            <div
              key={item.label}
              className={`flex min-w-0 flex-1 flex-col items-center justify-center p-3 text-center ${item.className}`}
            >
              <div className="max-w-full truncate whitespace-pre font-mono text-xs font-bold md:text-sm">
                {item.value}
              </div>
              <div className="mt-1 text-xs font-medium opacity-90">
                {item.label}
              </div>
            </div>
          ))}
        </div>
      </CardBody>
    </Card>
  );
}
