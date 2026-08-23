import type { SortDescriptor } from "@heroui/react";
import type { Selection } from "@react-types/shared";

import {
  Button,
  ButtonGroup,
  Card,
  CardBody,
  Chip,
  Code,
  Divider,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Pagination,
  Select,
  SelectItem,
  Skeleton,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import GroupManagementDrawer from "@/components/tunnels/group-management-drawer";
import PortalVectorQrModal from "@/components/tunnels/portal-vector-qr-modal";
import SimpleCreateTunnelModal from "@/components/tunnels/simple-create-tunnel-modal";
import { buildPortalUrl, deriveVectorUrl } from "@/lib/portal-url";
import { buildApiUrl } from "@/lib/utils";
import { copyToClipboard } from "@/lib/utils/clipboard";

interface EndpointSummary {
  id: string | number;
  name: string;
  hostname?: string;
  url?: string;
}

interface TunnelGroup {
  id: number;
  name: string;
}

interface PortalTunnel {
  id: string | number;
  instanceId?: string;
  type: "portal";
  name: string;
  endpoint?: string | EndpointSummary;
  endpointId: string | number;
  endpointName?: string;
  endpointVersion?: string;
  status: "running" | "stopped" | "error" | "offline";
  listenHost: string;
  listenPort: string | number;
  sharedKey?: string;
  network?: string;
  tlsMode?: string;
  commandLine?: string;
  configLine?: string;
  alpn?: string;
  rate?: number;
  etar?: number;
  dial?: string;
  logLevel?: string;
  portalHost?: string;
  vectorUrl?: string;
  totalRx?: number;
  totalTx?: number;
  sorts?: number;
  tags?: Record<string, string>;
}

type PortalAction = "start" | "stop" | "restart";

const formatBytes = (bytes = 0) => {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let value = bytes / 1024;
  let unit = units[0];

  for (let index = 1; index < units.length && value >= 1024; index += 1) {
    value /= 1024;
    unit = units[index];
  }

  return `${value.toFixed(value >= 100 ? 0 : 1)} ${unit}`;
};

const statusColor = (status: PortalTunnel["status"]) => {
  if (status === "running") return "success" as const;
  if (status === "stopped") return "default" as const;
  if (status === "offline") return "warning" as const;

  return "danger" as const;
};

const formatListenAddress = (host: string, port: string | number) => {
  if (!host || host === "0.0.0.0" || host === "::") return `:${port}`;
  if (host.includes(":") && !host.startsWith("[")) return `[${host}]:${port}`;

  return `${host}:${port}`;
};

export default function TunnelsPage() {
  const navigate = useNavigate();
  const { i18n } = useTranslation();
  const zh = i18n.language.startsWith("zh");
  const [tunnels, setTunnels] = useState<PortalTunnel[]>([]);
  const [endpoints, setEndpoints] = useState<EndpointSummary[]>([]);
  const [groups, setGroups] = useState<TunnelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [groupsLoading, setGroupsLoading] = useState(false);
  const [error, setError] = useState("");
  const [page, setPage] = useState(1);
  const [rowsPerPage, setRowsPerPage] = useState(() => {
    const saved = window.localStorage.getItem("tunnels-rows-per-page");

    return saved ? Number(saved) : 10;
  });
  const [totalPages, setTotalPages] = useState(1);
  const [totalItems, setTotalItems] = useState(0);
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState("all");
  const [endpointId, setEndpointId] = useState("all");
  const [groupId, setGroupId] = useState("all");
  const [sortDescriptor, setSortDescriptor] = useState<SortDescriptor>({
    column: undefined,
    direction: "descending",
  });
  const [selectedKeys, setSelectedKeys] = useState<Selection>(
    new Set<string>(),
  );
  const [busyIds, setBusyIds] = useState<Set<string>>(new Set());
  const [createOpen, setCreateOpen] = useState(false);
  const [urlCreateOpen, setUrlCreateOpen] = useState(false);
  const [urlCreating, setUrlCreating] = useState(false);
  const [urlCreate, setUrlCreate] = useState({
    endpointId: "",
    name: "",
    url: "",
  });
  const [editTunnel, setEditTunnel] = useState<PortalTunnel | null>(null);
  const [deleteTunnel, setDeleteTunnel] = useState<PortalTunnel | null>(null);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [qrTunnel, setQrTunnel] = useState<PortalTunnel | null>(null);
  const [exportOpen, setExportOpen] = useState(false);
  const [groupDrawerOpen, setGroupDrawerOpen] = useState(false);

  const copy = useMemo(
    () =>
      zh
        ? {
            title: "隧道管理",
            create: "创建隧道",
            createOptions: "隧道创建方式",
            createFromUrl: "通过隧道 URL 创建",
            createFromUrlHint:
              "粘贴完整的 portal:// URL，参数将按 Nowhere 规则校验。",
            search: "搜索隧道名称...",
            allStatus: "全部状态",
            allEndpoints: "全部节点",
            allGroups: "全部",
            name: "名称",
            endpoint: "节点",
            instanceId: "实例 ID",
            sharedKey: "密钥",
            listen: "监听地址",
            dial: "出站地址",
            tls: "TLS",
            tlsMode1: "TLS1",
            tlsMode2: "TLS2",
            network: "网络",
            status: "状态",
            traffic: "流量统计",
            vector: "Vector",
            actions: "操作",
            view: "查看详情",
            edit: "编辑",
            start: "启动",
            stop: "停止",
            restart: "重启",
            copyKey: "复制密钥",
            showVector: "导入节点",
            delete: "删除",
            deleteTitle: "删除隧道",
            batchDeleteTitle: "批量删除隧道",
            deleteMessage:
              "删除后将同时从 OpenCtrl 移除此隧道，此操作无法撤销。",
            batchDeleteMessage:
              "将删除选中的隧道，并同步从 OpenCtrl 移除。此操作无法撤销。",
            cancel: "取消",
            confirm: "确认删除",
            empty: "暂无隧道",
            emptyHint: "创建第一条隧道以接入 Vector。",
            loading: "正在加载隧道...",
            loadFailed: "隧道列表加载失败",
            retry: "重试",
            actionFailed: "操作失败",
            deleted: "隧道已删除",
            selected: "已选择",
            bulkActions: "批量操作",
            bulkStart: "批量启动",
            bulkStop: "批量停止",
            bulkRestart: "批量重启",
            bulkExport: "导出配置",
            bulkDelete: "批量删除",
            bulkNoStart: "选中的隧道均已运行",
            bulkNoStop: "选中的隧道均未运行",
            bulkNoRestart: "选中的隧道均不可重启",
            exportTitle: "导出隧道配置",
            exportHint: "以下内容仅包含当前选择的 Nowhere 隧道 URL。",
            copy: "复制",
            close: "关闭",
            sort: "排序",
            sortId: "创建顺序",
            sortWeight: "权重",
            sortName: "名称",
            sortEndpoint: "节点",
            sortStatus: "状态",
            sortPort: "监听端口",
            reset: "重置",
            refresh: "刷新",
            groupManagement: "分组管理",
            rows: "条 / 页",
            url: "隧道 URL",
            optionalName: "名称（可选）",
            required: "请选择节点并填写隧道 URL",
            created: "隧道已创建",
            createFailed: "隧道创建失败",
            running: "运行中",
            stopped: "已停止",
            offline: "已离线",
            error: "有错误",
          }
        : {
            title: "Tunnel Management",
            create: "Create Tunnel",
            createOptions: "Tunnel creation options",
            createFromUrl: "Create from Tunnel URL",
            createFromUrlHint:
              "Paste a complete portal:// URL. Parameters are validated using Nowhere rules.",
            search: "Search tunnel name...",
            allStatus: "All Status",
            allEndpoints: "All Nodes",
            allGroups: "All",
            name: "Name",
            endpoint: "Node",
            instanceId: "Instance ID",
            sharedKey: "Key",
            listen: "Listen Address",
            dial: "Outbound Address",
            tls: "TLS",
            tlsMode1: "TLS1",
            tlsMode2: "TLS2",
            network: "Network",
            status: "Status",
            traffic: "Traffic Stats",
            vector: "Vector",
            actions: "Actions",
            view: "View details",
            edit: "Edit",
            start: "Start",
            stop: "Stop",
            restart: "Restart",
            copyKey: "Copy key",
            showVector: "Import node",
            delete: "Delete",
            deleteTitle: "Delete Tunnel",
            batchDeleteTitle: "Delete selected Tunnels",
            deleteMessage:
              "This removes the Tunnel from OpenCtrl and cannot be undone.",
            batchDeleteMessage:
              "The selected Tunnels will be removed from OpenCtrl. This cannot be undone.",
            cancel: "Cancel",
            confirm: "Delete",
            empty: "No Tunnels",
            emptyHint: "Create your first Tunnel to connect Vector.",
            loading: "Loading Tunnels...",
            loadFailed: "Failed to load Tunnels",
            retry: "Retry",
            actionFailed: "Action failed",
            deleted: "Tunnel deleted",
            selected: "Selected",
            bulkActions: "Bulk actions",
            bulkStart: "Start selected",
            bulkStop: "Stop selected",
            bulkRestart: "Restart selected",
            bulkExport: "Export configuration",
            bulkDelete: "Delete selected",
            bulkNoStart: "All selected Tunnels are already running",
            bulkNoStop: "None of the selected Tunnels are running",
            bulkNoRestart: "None of the selected Tunnels can be restarted",
            exportTitle: "Export Tunnel configuration",
            exportHint:
              "This export contains only the selected Nowhere Tunnel URLs.",
            copy: "Copy",
            close: "Close",
            sort: "Sort",
            sortId: "Created order",
            sortWeight: "Weight",
            sortName: "Name",
            sortEndpoint: "Node",
            sortStatus: "Status",
            sortPort: "Listen port",
            reset: "Reset",
            refresh: "Refresh",
            groupManagement: "Group Management",
            rows: "per page",
            url: "Tunnel URL",
            optionalName: "Name (optional)",
            required: "Select a node and enter a Tunnel URL",
            created: "Tunnel created",
            createFailed: "Failed to create Tunnel",
            running: "Running",
            stopped: "Stopped",
            offline: "Offline",
            error: "Error",
          },
    [zh],
  );

  const sortOptions = useMemo(
    () => [
      { key: "id", label: copy.sortId },
      { key: "sorts", label: copy.sortWeight },
      { key: "name", label: copy.sortName },
      { key: "endpoint_id", label: copy.sortEndpoint },
      { key: "status", label: copy.sortStatus },
      { key: "listen_port", label: copy.sortPort },
    ],
    [copy],
  );

  const endpointMap = useMemo(
    () => new Map(endpoints.map((endpoint) => [String(endpoint.id), endpoint])),
    [endpoints],
  );

  const getEndpoint = useCallback(
    (tunnel: PortalTunnel) => {
      if (typeof tunnel.endpoint === "object") return tunnel.endpoint;

      return endpointMap.get(String(tunnel.endpointId));
    },
    [endpointMap],
  );

  const getEndpointName = useCallback(
    (tunnel: PortalTunnel) => {
      if (tunnel.endpointName) return tunnel.endpointName;
      if (typeof tunnel.endpoint === "string") return tunnel.endpoint;

      return getEndpoint(tunnel)?.name ?? "-";
    },
    [getEndpoint],
  );

  const getStatusText = useCallback(
    (value: PortalTunnel["status"]) => copy[value] ?? value,
    [copy],
  );

  const getTlsModeText = useCallback(
    (value?: string) => (value === "2" ? copy.tlsMode2 : copy.tlsMode1),
    [copy],
  );

  const selectedTunnels = useMemo(() => {
    if (selectedKeys === "all") return tunnels;
    const selected = new Set(Array.from(selectedKeys).map(String));

    return tunnels.filter((tunnel) => selected.has(String(tunnel.id)));
  }, [selectedKeys, tunnels]);

  const selectedCount = useMemo(() => {
    if (selectedKeys === "all") return tunnels.length;
    if (selectedKeys instanceof Set) return selectedKeys.size;

    return 0;
  }, [selectedKeys, tunnels.length]);

  const selectedIds = useMemo(() => {
    if (selectedKeys === "all")
      return tunnels.map((tunnel) => String(tunnel.id));
    if (selectedKeys instanceof Set)
      return Array.from(selectedKeys).map(String);

    return [];
  }, [selectedKeys, tunnels]);

  const exportConfig = useMemo(
    () => selectedTunnels.map((tunnel) => buildPortalUrl(tunnel)).join("\n"),
    [selectedTunnels],
  );

  const fetchEndpoints = useCallback(async () => {
    try {
      const response = await fetch(buildApiUrl("/api/endpoints/simple"));

      if (!response.ok) return;
      const data = (await response.json()) as EndpointSummary[];

      setEndpoints(data);
      setUrlCreate((current) => ({
        ...current,
        endpointId: current.endpointId || String(data[0]?.id ?? ""),
      }));
    } catch {
      setEndpoints([]);
    }
  }, []);

  const fetchGroups = useCallback(async () => {
    setGroupsLoading(true);
    try {
      const response = await fetch(buildApiUrl("/api/groups"));
      const body = await response.json();

      if (!response.ok) throw new Error(body.error);
      setGroups(body.groups ?? []);
    } catch {
      setGroups([]);
    } finally {
      setGroupsLoading(false);
    }
  }, []);

  const fetchTunnels = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const params = new URLSearchParams({
        page: String(page),
        page_size: String(rowsPerPage),
      });

      if (search) params.set("search", search);
      if (status !== "all") params.set("status", status);
      if (endpointId !== "all") params.set("endpoint_id", endpointId);
      if (groupId !== "all") params.set("group_id", groupId);
      if (sortDescriptor.column) {
        params.set("sort_by", String(sortDescriptor.column));
        params.set(
          "sort_order",
          sortDescriptor.direction === "ascending" ? "asc" : "desc",
        );
      }

      const response = await fetch(
        buildApiUrl(`/api/tunnels?${params.toString()}`),
      );
      const body = await response.json();

      if (!response.ok) throw new Error(body.error || copy.loadFailed);
      setTunnels(
        (body.data ?? []).filter(
          (item: PortalTunnel) => item.type === "portal",
        ),
      );
      setTotalItems(body.total ?? 0);
      setTotalPages(Math.max(1, body.total_pages ?? 1));
    } catch (fetchError) {
      const message =
        fetchError instanceof Error ? fetchError.message : copy.loadFailed;

      setError(message);
    } finally {
      setLoading(false);
    }
  }, [
    copy.loadFailed,
    endpointId,
    groupId,
    page,
    rowsPerPage,
    search,
    sortDescriptor.column,
    sortDescriptor.direction,
    status,
  ]);

  useEffect(() => {
    void Promise.all([fetchEndpoints(), fetchGroups()]);
  }, [fetchEndpoints, fetchGroups]);

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 400);

    return () => window.clearTimeout(timer);
  }, [searchInput]);

  useEffect(() => {
    void fetchTunnels();
  }, [fetchTunnels]);

  useEffect(() => {
    const timer = window.setInterval(() => void fetchTunnels(), 15000);

    return () => window.clearInterval(timer);
  }, [fetchTunnels]);

  const markBusy = (id: string, value: boolean) => {
    setBusyIds((current) => {
      const next = new Set(current);

      if (value) next.add(id);
      else next.delete(id);

      return next;
    });
  };

  const performAction = async (tunnel: PortalTunnel, action: PortalAction) => {
    const response = await fetch(
      buildApiUrl(`/api/tunnels/${tunnel.id}/action`),
      {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action }),
      },
    );
    const body = await response.json();

    if (!response.ok || body.success === false)
      throw new Error(body.error || body.message || copy.actionFailed);
  };

  const runAction = async (tunnel: PortalTunnel, action: PortalAction) => {
    const id = String(tunnel.id);

    markBusy(id, true);
    try {
      await performAction(tunnel, action);
      await fetchTunnels();
    } catch (actionError) {
      addToast({
        title: copy.actionFailed,
        description:
          actionError instanceof Error
            ? actionError.message
            : copy.actionFailed,
        color: "danger",
      });
    } finally {
      markBusy(id, false);
    }
  };

  const runBatchAction = async (action: PortalAction) => {
    const actionTargets = selectedTunnels.filter((tunnel) => {
      if (action === "start") return tunnel.status !== "running";

      return tunnel.status === "running";
    });

    if (!actionTargets.length) {
      const message =
        action === "start"
          ? copy.bulkNoStart
          : action === "stop"
            ? copy.bulkNoStop
            : copy.bulkNoRestart;

      addToast({
        title: copy.actionFailed,
        description: message,
        color: "warning",
      });

      return;
    }

    actionTargets.forEach((tunnel) => markBusy(String(tunnel.id), true));
    try {
      await Promise.all(
        actionTargets.map((tunnel) => performAction(tunnel, action)),
      );
      await fetchTunnels();
    } catch (actionError) {
      addToast({
        title: copy.actionFailed,
        description:
          actionError instanceof Error
            ? actionError.message
            : copy.actionFailed,
        color: "danger",
      });
    } finally {
      actionTargets.forEach((tunnel) => markBusy(String(tunnel.id), false));
    }
  };

  const deleteOne = async (tunnelId: string) => {
    const response = await fetch(buildApiUrl(`/api/tunnels/${tunnelId}`), {
      method: "DELETE",
    });
    const body = await response.json();

    if (!response.ok || body.success === false)
      throw new Error(body.error || body.message || copy.actionFailed);
  };

  const confirmDelete = async () => {
    const targetIds = deleteTunnel ? [String(deleteTunnel.id)] : selectedIds;

    if (!targetIds.length) return;
    setDeleting(true);
    try {
      await Promise.all(targetIds.map(deleteOne));
      addToast({ title: copy.deleted, color: "success" });
      setDeleteTunnel(null);
      setBatchDeleteOpen(false);
      if (!deleteTunnel) setSelectedKeys(new Set<string>());
      if (targetIds.length === tunnels.length && page > 1) setPage(page - 1);
      else await fetchTunnels();
    } catch (deleteError) {
      addToast({
        title: copy.actionFailed,
        description:
          deleteError instanceof Error
            ? deleteError.message
            : copy.actionFailed,
        color: "danger",
      });
    } finally {
      setDeleting(false);
    }
  };

  const submitUrlCreate = async () => {
    if (!urlCreate.endpointId || !urlCreate.url.trim()) {
      addToast({ title: copy.required, color: "warning" });

      return;
    }
    setUrlCreating(true);
    try {
      const response = await fetch(buildApiUrl("/api/tunnels/create_by_url"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          endpointId: Number(urlCreate.endpointId),
          name: urlCreate.name.trim(),
          url: urlCreate.url.trim(),
        }),
      });
      const body = await response.json();

      if (!response.ok || body.success === false)
        throw new Error(body.error || body.message || copy.createFailed);
      addToast({ title: copy.created, color: "success" });
      setUrlCreateOpen(false);
      setUrlCreate((current) => ({ ...current, name: "", url: "" }));
      await fetchTunnels();
    } catch (createError) {
      addToast({
        title: copy.createFailed,
        description:
          createError instanceof Error
            ? createError.message
            : copy.createFailed,
        color: "danger",
      });
    } finally {
      setUrlCreating(false);
    }
  };

  const resetFilters = () => {
    setSearchInput("");
    setSearch("");
    setStatus("all");
    setEndpointId("all");
    setGroupId("all");
    setSortDescriptor({ column: undefined, direction: "descending" });
    setPage(1);
  };

  const openDetails = (tunnel: PortalTunnel) =>
    navigate(`/tunnels/details?id=${tunnel.id}`);

  const groupFilter = (
    <div className="flex min-h-8 flex-wrap items-center gap-2">
      <Button
        color={groupId === "all" ? "primary" : "default"}
        size="sm"
        variant={groupId === "all" ? "solid" : "flat"}
        onPress={() => {
          setGroupId("all");
          setPage(1);
        }}
      >
        {copy.allGroups}
      </Button>
      {groupsLoading ? (
        <Spinner size="sm" />
      ) : (
        groups.map((group) => (
          <Button
            key={group.id}
            color={groupId === String(group.id) ? "primary" : "default"}
            size="sm"
            variant={groupId === String(group.id) ? "solid" : "flat"}
            onPress={() => {
              setGroupId(String(group.id));
              setPage(1);
            }}
          >
            {group.name}
          </Button>
        ))
      )}
    </div>
  );

  const pagination = totalPages > 1 && (
    <div className="flex w-full justify-center py-2">
      <Pagination
        showControls
        page={page}
        size="sm"
        total={totalPages}
        onChange={setPage}
      />
    </div>
  );

  const trafficInfo = (tunnel: PortalTunnel) => (
    <span className="whitespace-nowrap font-mono text-sm text-default-600">
      {formatBytes((tunnel.totalTx ?? 0) + (tunnel.totalRx ?? 0))}
    </span>
  );

  const sharedKeyInfo = (tunnel: PortalTunnel) => {
    const sharedKey = tunnel.sharedKey?.trim();

    if (!sharedKey) return <span className="text-default-400">-</span>;

    return (
      <div className="flex min-w-0 items-center gap-1">
        <Tooltip
          content={
            <span className="break-all font-mono text-xs">{sharedKey}</span>
          }
        >
          <span className="min-w-0 truncate font-mono text-xs text-default-600">
            {sharedKey}
          </span>
        </Tooltip>
        <Tooltip content={copy.copyKey}>
          <Button
            isIconOnly
            aria-label={copy.copyKey}
            className="shrink-0"
            size="sm"
            variant="light"
            onPress={() => copyToClipboard(sharedKey, copy.copyKey)}
          >
            <Icon icon="lucide:copy" width={14} />
          </Button>
        </Tooltip>
      </div>
    );
  };

  const rowActions = (tunnel: PortalTunnel) => {
    const busy = busyIds.has(String(tunnel.id));

    return (
      <div className="flex items-center justify-end gap-0.5">
        <Tooltip content={copy.view}>
          <Button
            isIconOnly
            aria-label={copy.view}
            color="primary"
            size="sm"
            variant="light"
            onPress={() => openDetails(tunnel)}
          >
            <Icon icon="lucide:eye" width={16} />
          </Button>
        </Tooltip>
        <Tooltip content={tunnel.status === "running" ? copy.stop : copy.start}>
          <Button
            isIconOnly
            aria-label={tunnel.status === "running" ? copy.stop : copy.start}
            color={tunnel.status === "running" ? "warning" : "success"}
            isLoading={busy}
            size="sm"
            variant="light"
            onPress={() =>
              void runAction(
                tunnel,
                tunnel.status === "running" ? "stop" : "start",
              )
            }
          >
            {!busy && (
              <Icon
                icon={
                  tunnel.status === "running" ? "lucide:square" : "lucide:play"
                }
                width={15}
              />
            )}
          </Button>
        </Tooltip>
        <Tooltip content={copy.restart}>
          <Button
            isIconOnly
            aria-label={copy.restart}
            color="secondary"
            isDisabled={busy || tunnel.status !== "running"}
            size="sm"
            variant="light"
            onPress={() => void runAction(tunnel, "restart")}
          >
            <Icon icon="lucide:rotate-cw" width={15} />
          </Button>
        </Tooltip>
        <Tooltip content={copy.edit}>
          <Button
            isIconOnly
            aria-label={copy.edit}
            color="warning"
            size="sm"
            variant="light"
            onPress={() => setEditTunnel(tunnel)}
          >
            <Icon icon="lucide:pencil" width={15} />
          </Button>
        </Tooltip>
        <Tooltip content={copy.showVector}>
          <Button
            isIconOnly
            aria-label={copy.showVector}
            color="primary"
            size="sm"
            variant="light"
            onPress={() => setQrTunnel(tunnel)}
          >
            <Icon icon="lucide:qr-code" width={16} />
          </Button>
        </Tooltip>
        <Tooltip content={copy.delete}>
          <Button
            isIconOnly
            aria-label={copy.delete}
            color="danger"
            size="sm"
            variant="light"
            onPress={() => setDeleteTunnel(tunnel)}
          >
            <Icon icon="lucide:trash-2" width={16} />
          </Button>
        </Tooltip>
      </div>
    );
  };

  return (
    <div className="w-full">
      <header className="mb-4 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <h1 className="truncate text-2xl font-semibold text-foreground">
            {copy.title}
          </h1>
          {!loading && (
            <Chip className="text-default-500" size="sm" variant="flat">
              {totalItems}
            </Chip>
          )}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Button
            className="hidden sm:flex"
            color="warning"
            startContent={<Icon icon="lucide:tags" width={17} />}
            variant="flat"
            onPress={() => setGroupDrawerOpen(true)}
          >
            {copy.groupManagement}
          </Button>
          <ButtonGroup>
            <Button
              color="primary"
              startContent={<Icon icon="lucide:plus" width={17} />}
              onPress={() => setCreateOpen(true)}
            >
              <span className="hidden sm:inline">{copy.create}</span>
              <span className="sm:hidden">{zh ? "隧道" : "Tunnel"}</span>
            </Button>
            <Dropdown placement="bottom-end">
              <DropdownTrigger>
                <Button
                  isIconOnly
                  aria-label={copy.createOptions}
                  color="primary"
                >
                  <Icon icon="lucide:chevron-down" width={16} />
                </Button>
              </DropdownTrigger>
              <DropdownMenu aria-label={copy.createOptions}>
                <DropdownItem
                  key="url"
                  startContent={<Icon icon="lucide:terminal" width={17} />}
                  onPress={() => setUrlCreateOpen(true)}
                >
                  {copy.createFromUrl}
                </DropdownItem>
                <DropdownItem
                  key="groups"
                  className="sm:hidden"
                  startContent={<Icon icon="lucide:tags" width={17} />}
                  onPress={() => setGroupDrawerOpen(true)}
                >
                  {copy.groupManagement}
                </DropdownItem>
              </DropdownMenu>
            </Dropdown>
          </ButtonGroup>
        </div>
      </header>

      <div className="mb-4 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="grid flex-1 gap-2 sm:grid-cols-[minmax(220px,1fr)_150px_180px] xl:max-w-[620px]">
          <Input
            isClearable
            aria-label={copy.search}
            placeholder={copy.search}
            size="sm"
            startContent={
              <Icon
                className="text-default-400"
                icon="lucide:search"
                width={16}
              />
            }
            value={searchInput}
            onClear={() => setSearchInput("")}
            onValueChange={setSearchInput}
          />
          <Select
            aria-label={copy.allStatus}
            selectedKeys={new Set([status])}
            size="sm"
            onSelectionChange={(keys) => {
              setStatus(String(Array.from(keys)[0] ?? "all"));
              setPage(1);
            }}
          >
            <SelectItem key="all">{copy.allStatus}</SelectItem>
            <SelectItem key="running">{copy.running}</SelectItem>
            <SelectItem key="stopped">{copy.stopped}</SelectItem>
            <SelectItem key="error">{copy.error}</SelectItem>
            <SelectItem key="offline">{copy.offline}</SelectItem>
          </Select>
          <Select
            aria-label={copy.allEndpoints}
            selectedKeys={new Set([endpointId])}
            size="sm"
            onSelectionChange={(keys) => {
              setEndpointId(String(Array.from(keys)[0] ?? "all"));
              setPage(1);
            }}
          >
            <SelectItem key="all">{copy.allEndpoints}</SelectItem>
            <>
              {endpoints.map((endpoint) => (
                <SelectItem key={String(endpoint.id)}>
                  {endpoint.name}
                </SelectItem>
              ))}
            </>
          </Select>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Divider className="hidden h-5 xl:block" orientation="vertical" />
          <Dropdown placement="bottom-end">
            <DropdownTrigger>
              <Button
                size="sm"
                startContent={
                  <Icon
                    icon={
                      sortDescriptor.column
                        ? sortDescriptor.direction === "ascending"
                          ? "lucide:arrow-up"
                          : "lucide:arrow-down"
                        : "lucide:arrow-up-down"
                    }
                    width={15}
                  />
                }
                variant="flat"
              >
                {sortDescriptor.column
                  ? sortOptions.find(
                      (option) => option.key === sortDescriptor.column,
                    )?.label
                  : copy.sort}
              </Button>
            </DropdownTrigger>
            <DropdownMenu
              aria-label={copy.sort}
              onAction={(key) => {
                const column = String(key);
                const direction =
                  sortDescriptor.column === column &&
                  sortDescriptor.direction === "descending"
                    ? "ascending"
                    : "descending";

                setSortDescriptor({ column, direction });
                setPage(1);
              }}
            >
              {sortOptions.map((option) => (
                <DropdownItem
                  key={option.key}
                  startContent={
                    sortDescriptor.column === option.key ? (
                      <Icon
                        className="text-primary"
                        icon={
                          sortDescriptor.direction === "ascending"
                            ? "lucide:arrow-up"
                            : "lucide:arrow-down"
                        }
                        width={14}
                      />
                    ) : (
                      <span className="w-3.5" />
                    )
                  }
                >
                  {option.label}
                </DropdownItem>
              ))}
            </DropdownMenu>
          </Dropdown>
          <Button
            size="sm"
            startContent={<Icon icon="lucide:calendar-x" width={15} />}
            variant="flat"
            onPress={resetFilters}
          >
            {copy.reset}
          </Button>
          <Button
            isLoading={loading}
            size="sm"
            startContent={
              !loading && <Icon icon="lucide:refresh-cw" width={15} />
            }
            variant="flat"
            onPress={() => void fetchTunnels()}
          >
            <span className="hidden sm:inline">{copy.refresh}</span>
          </Button>
          {selectedCount > 0 && (
            <Dropdown placement="bottom-end">
              <DropdownTrigger>
                <Button size="sm" variant="flat">
                  {copy.selected} {selectedCount}
                  <Icon icon="lucide:chevron-down" width={14} />
                </Button>
              </DropdownTrigger>
              <DropdownMenu
                aria-label={copy.bulkActions}
                onAction={(key) => {
                  if (key === "export") setExportOpen(true);
                  else if (key === "delete") setBatchDeleteOpen(true);
                  else void runBatchAction(key as PortalAction);
                }}
              >
                <DropdownItem
                  key="start"
                  className="text-success"
                  startContent={<Icon icon="lucide:play" width={16} />}
                >
                  {copy.bulkStart}
                </DropdownItem>
                <DropdownItem
                  key="stop"
                  className="text-warning"
                  startContent={<Icon icon="lucide:square" width={16} />}
                >
                  {copy.bulkStop}
                </DropdownItem>
                <DropdownItem
                  key="restart"
                  className="text-secondary"
                  startContent={<Icon icon="lucide:rotate-cw" width={16} />}
                >
                  {copy.bulkRestart}
                </DropdownItem>
                <DropdownItem
                  key="export"
                  startContent={<Icon icon="lucide:download" width={16} />}
                >
                  {copy.bulkExport}
                </DropdownItem>
                <DropdownItem
                  key="delete"
                  className="text-danger"
                  color="danger"
                  startContent={<Icon icon="lucide:trash-2" width={16} />}
                >
                  {copy.bulkDelete}
                </DropdownItem>
              </DropdownMenu>
            </Dropdown>
          )}
          <Select
            aria-label={copy.rows}
            className="w-28"
            selectedKeys={new Set([String(rowsPerPage)])}
            size="sm"
            onSelectionChange={(keys) => {
              const value = Number(Array.from(keys)[0] ?? 10);

              setRowsPerPage(value);
              window.localStorage.setItem(
                "tunnels-rows-per-page",
                String(value),
              );
              setPage(1);
            }}
          >
            {[10, 20, 50].map((value) => (
              <SelectItem
                key={String(value)}
                textValue={`${value} ${copy.rows}`}
              >
                {value} {copy.rows}
              </SelectItem>
            ))}
          </Select>
        </div>
      </div>

      <Card className="border border-default-100 shadow-none">
        <CardBody className="gap-4 p-4">
          {groupFilter}

          <div className="hidden overflow-x-auto md:block [&_td:first-child]:!w-10 [&_td:first-child]:!max-w-10 [&_th:first-child]:!w-10 [&_th:first-child]:!max-w-10">
            <Table
              removeWrapper
              aria-label={copy.title}
              selectedKeys={selectedKeys}
              selectionMode="multiple"
              sortDescriptor={sortDescriptor}
              onSelectionChange={setSelectedKeys}
              onSortChange={(descriptor) => {
                setSortDescriptor(descriptor);
                setPage(1);
              }}
            >
              <TableHeader>
                <TableColumn key="name" allowsSorting minWidth={170}>
                  {copy.name}
                </TableColumn>
                <TableColumn key="instance_id" minWidth={112}>
                  {copy.instanceId}
                </TableColumn>
                <TableColumn key="endpoint_id" allowsSorting minWidth={120}>
                  {copy.endpoint}
                </TableColumn>
                <TableColumn key="listen_port" allowsSorting minWidth={150}>
                  {copy.listen}
                </TableColumn>
                <TableColumn key="dial" minWidth={110}>
                  {copy.dial}
                </TableColumn>
                <TableColumn key="tls_mode" minWidth={84}>
                  {copy.tls}
                </TableColumn>
                <TableColumn key="network" minWidth={88}>
                  {copy.network}
                </TableColumn>
                <TableColumn key="shared_key" minWidth={160}>
                  {copy.sharedKey}
                </TableColumn>
                <TableColumn key="status" allowsSorting width={100}>
                  {copy.status}
                </TableColumn>
                <TableColumn key="traffic" width={110}>
                  {copy.traffic}
                </TableColumn>
                <TableColumn key="actions" align="end" width={225}>
                  {copy.actions}
                </TableColumn>
              </TableHeader>
              <TableBody
                emptyContent={
                  <div className="flex min-h-64 flex-col items-center justify-center gap-3 py-10 text-center">
                    <span className="flex size-16 items-center justify-center rounded-full bg-default-100 text-default-400">
                      <Icon
                        icon={
                          error ? "lucide:circle-alert" : "lucide:radio-tower"
                        }
                        width={28}
                      />
                    </span>
                    <div>
                      <p
                        className={
                          error ? "font-medium text-danger" : "font-medium"
                        }
                      >
                        {error || copy.empty}
                      </p>
                      <p className="mt-1 text-sm text-default-400">
                        {error ? copy.loadFailed : copy.emptyHint}
                      </p>
                    </div>
                    {error && (
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => void fetchTunnels()}
                      >
                        {copy.retry}
                      </Button>
                    )}
                  </div>
                }
                isLoading={loading}
                items={tunnels}
                loadingContent={<Spinner label={copy.loading} />}
              >
                {(tunnel) => (
                  <TableRow
                    key={String(tunnel.id)}
                    onDoubleClick={() => openDetails(tunnel)}
                  >
                    <TableCell>
                      <Tooltip
                        content={
                          <div className="max-w-64 text-xs">
                            <p className="font-medium">{tunnel.name}</p>
                            <p className="text-default-400">
                              {tunnel.instanceId || `#${tunnel.id}`}
                            </p>
                          </div>
                        }
                      >
                        <button
                          className="block max-w-64 text-left"
                          type="button"
                          onClick={() => openDetails(tunnel)}
                        >
                          <span className="line-clamp-2 break-all text-sm font-semibold leading-tight hover:text-primary">
                            {tunnel.name}
                          </span>
                        </button>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-xs text-default-600">
                        {tunnel.instanceId || `#${tunnel.id}`}
                      </span>
                    </TableCell>
                    <TableCell>
                      <Tooltip
                        content={
                          tunnel.endpointVersion
                            ? `${getEndpointName(tunnel)} · ${tunnel.endpointVersion}`
                            : getEndpointName(tunnel)
                        }
                      >
                        <span className="block max-w-40 truncate text-sm text-default-600">
                          {getEndpointName(tunnel)}
                        </span>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      <div className="font-mono text-sm text-default-600">
                        {formatListenAddress(
                          tunnel.listenHost,
                          tunnel.listenPort,
                        )}
                      </div>
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-sm text-default-600">
                        {tunnel.dial || "auto"}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="text-sm text-default-600">
                        {getTlsModeText(tunnel.tlsMode)}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span className="font-mono text-sm text-default-600">
                        {tunnel.network || "mix"}
                      </span>
                    </TableCell>
                    <TableCell>
                      <div className="max-w-44">{sharedKeyInfo(tunnel)}</div>
                    </TableCell>
                    <TableCell>
                      <Chip
                        color={statusColor(tunnel.status)}
                        size="sm"
                        variant="flat"
                      >
                        {getStatusText(tunnel.status)}
                      </Chip>
                    </TableCell>
                    <TableCell>{trafficInfo(tunnel)}</TableCell>
                    <TableCell>{rowActions(tunnel)}</TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </div>

          <div className="grid gap-3 md:hidden">
            {loading ? (
              Array.from({ length: 4 }).map((_, index) => (
                <div
                  key={index}
                  className="space-y-3 rounded-lg border border-default-200 p-4"
                >
                  <Skeleton className="h-5 w-2/3 rounded-md" />
                  <Skeleton className="h-4 w-1/2 rounded-md" />
                  <Skeleton className="h-9 w-full rounded-md" />
                </div>
              ))
            ) : error || tunnels.length === 0 ? (
              <div className="flex min-h-64 flex-col items-center justify-center gap-3 text-center">
                <Icon
                  className="text-default-400"
                  icon={error ? "lucide:circle-alert" : "lucide:radio-tower"}
                  width={34}
                />
                <div>
                  <p
                    className={
                      error ? "font-medium text-danger" : "font-medium"
                    }
                  >
                    {error || copy.empty}
                  </p>
                  <p className="mt-1 text-sm text-default-400">
                    {error ? copy.loadFailed : copy.emptyHint}
                  </p>
                </div>
              </div>
            ) : (
              tunnels.map((tunnel) => (
                <article
                  key={String(tunnel.id)}
                  className="rounded-lg border border-default-200 p-4"
                >
                  <div className="flex items-start justify-between gap-3">
                    <button
                      className="min-w-0 text-left"
                      type="button"
                      onClick={() => openDetails(tunnel)}
                    >
                      <p className="line-clamp-2 break-all font-semibold">
                        {tunnel.name}
                      </p>
                    </button>
                    <Chip
                      className="shrink-0"
                      color={statusColor(tunnel.status)}
                      size="sm"
                      variant="flat"
                    >
                      {getStatusText(tunnel.status)}
                    </Chip>
                  </div>

                  <dl className="mt-4 grid grid-cols-2 gap-x-3 gap-y-4 text-sm">
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.instanceId}
                      </dt>
                      <dd className="mt-1 truncate font-mono text-xs">
                        {tunnel.instanceId || `#${tunnel.id}`}
                      </dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.endpoint}
                      </dt>
                      <dd className="mt-1 truncate">
                        {getEndpointName(tunnel)}
                      </dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.listen}
                      </dt>
                      <dd className="mt-1 truncate font-mono">
                        {formatListenAddress(
                          tunnel.listenHost,
                          tunnel.listenPort,
                        )}
                      </dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">{copy.dial}</dt>
                      <dd className="mt-1 truncate font-mono">
                        {tunnel.dial || "auto"}
                      </dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">{copy.tls}</dt>
                      <dd className="mt-1">{getTlsModeText(tunnel.tlsMode)}</dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.network}
                      </dt>
                      <dd className="mt-1 truncate font-mono">
                        {tunnel.network || "mix"}
                      </dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.sharedKey}
                      </dt>
                      <dd className="mt-1">{sharedKeyInfo(tunnel)}</dd>
                    </div>
                    <div className="min-w-0">
                      <dt className="text-xs text-default-400">
                        {copy.traffic}
                      </dt>
                      <dd className="mt-1">{trafficInfo(tunnel)}</dd>
                    </div>
                  </dl>

                  <Divider className="my-3" />
                  <div className="flex items-center justify-end gap-2">
                    {rowActions(tunnel)}
                  </div>
                </article>
              ))
            )}
          </div>

          {pagination}
        </CardBody>
      </Card>

      <SimpleCreateTunnelModal
        isOpen={createOpen}
        onOpenChange={setCreateOpen}
        onSaved={fetchTunnels}
      />
      {editTunnel && (
        <SimpleCreateTunnelModal
          isOpen
          instanceId={String(editTunnel.id)}
          mode="edit"
          onOpenChange={(open) => {
            if (!open) setEditTunnel(null);
          }}
          onSaved={fetchTunnels}
        />
      )}

      <Modal
        isOpen={urlCreateOpen}
        placement="center"
        size="xl"
        onOpenChange={setUrlCreateOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2 pb-0">
                <Icon
                  className="text-warning"
                  icon="lucide:terminal"
                  width={18}
                />
                {copy.createFromUrl}
              </ModalHeader>
              <ModalBody className="gap-4 py-5">
                <p className="text-sm text-default-500">
                  {copy.createFromUrlHint}
                </p>
                <Select
                  isRequired
                  label={copy.endpoint}
                  selectedKeys={
                    urlCreate.endpointId
                      ? new Set([urlCreate.endpointId])
                      : new Set()
                  }
                  onSelectionChange={(keys) =>
                    setUrlCreate((current) => ({
                      ...current,
                      endpointId: String(Array.from(keys)[0] ?? ""),
                    }))
                  }
                >
                  {endpoints.map((endpoint) => (
                    <SelectItem key={String(endpoint.id)}>
                      {endpoint.name}
                    </SelectItem>
                  ))}
                </Select>
                <Input
                  label={copy.optionalName}
                  value={urlCreate.name}
                  onValueChange={(name) =>
                    setUrlCreate((current) => ({ ...current, name }))
                  }
                />
                <Input
                  isRequired
                  label={copy.url}
                  placeholder="portal://shared-key@:2077?net=mix&tls=1"
                  value={urlCreate.url}
                  onValueChange={(url) =>
                    setUrlCreate((current) => ({ ...current, url }))
                  }
                />
              </ModalBody>
              <ModalFooter>
                <Button
                  isDisabled={urlCreating}
                  variant="light"
                  onPress={onClose}
                >
                  {copy.cancel}
                </Button>
                <Button
                  color="primary"
                  isLoading={urlCreating}
                  onPress={() => void submitUrlCreate()}
                >
                  {copy.create}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        isOpen={Boolean(deleteTunnel) || batchDeleteOpen}
        placement="center"
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTunnel(null);
            setBatchDeleteOpen(false);
          }
        }}
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
                {deleteTunnel ? copy.deleteTitle : copy.batchDeleteTitle}
              </ModalHeader>
              <ModalBody>
                {deleteTunnel && (
                  <p className="font-medium">{deleteTunnel.name}</p>
                )}
                {!deleteTunnel && (
                  <p className="font-medium">
                    {copy.selected} {selectedCount}
                  </p>
                )}
                <p className="text-sm text-warning">
                  {deleteTunnel ? copy.deleteMessage : copy.batchDeleteMessage}
                </p>
              </ModalBody>
              <ModalFooter>
                <Button isDisabled={deleting} variant="light" onPress={onClose}>
                  {copy.cancel}
                </Button>
                <Button
                  color="danger"
                  isLoading={deleting}
                  onPress={() => void confirmDelete()}
                >
                  {copy.confirm}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal isOpen={exportOpen} size="2xl" onOpenChange={setExportOpen}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Icon
                  className="text-primary"
                  icon="lucide:download"
                  width={18}
                />
                {copy.exportTitle}
              </ModalHeader>
              <ModalBody>
                <p className="text-sm text-default-500">{copy.exportHint}</p>
                <Code className="max-h-80 w-full overflow-auto p-4">
                  <pre className="whitespace-pre-wrap break-all text-xs">
                    {exportConfig}
                  </pre>
                </Code>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {copy.close}
                </Button>
                <Button
                  color="primary"
                  startContent={<Icon icon="lucide:copy" width={16} />}
                  onPress={() => copyToClipboard(exportConfig, copy.copy)}
                >
                  {copy.copy}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <GroupManagementDrawer
        isOpen={groupDrawerOpen}
        onOpenChange={setGroupDrawerOpen}
        onSaved={() => void fetchGroups()}
      />

      <PortalVectorQrModal
        isOpen={Boolean(qrTunnel)}
        vectorUrl={
          qrTunnel
            ? qrTunnel.vectorUrl ||
              deriveVectorUrl(qrTunnel, getEndpoint(qrTunnel))
            : null
        }
        onOpenChange={(open) => {
          if (!open) setQrTunnel(null);
        }}
      />
    </div>
  );
}
