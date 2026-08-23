import {
  Button,
  Card,
  CardBody,
  CardHeader,
  Chip,
  Divider,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Progress,
  Switch,
  useDisclosure,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";

import { apiGet, apiPost, apiPut } from "@/lib/api-client";

interface HistoryCleanupConfig {
  autoCleanupEnabled: boolean;
  serviceHistoryRetentionDays: number;
  summaryRetentionDays: number;
  dashboardRetentionDays: number;
  operationLogRetentionDays: number;
  batchSize: number;
  scheduleTime: string;
}

type EditableCleanupConfig = Pick<
  HistoryCleanupConfig,
  | "autoCleanupEnabled"
  | "serviceHistoryRetentionDays"
  | "summaryRetentionDays"
  | "dashboardRetentionDays"
  | "operationLogRetentionDays"
>;

interface HistoryTableStats {
  tableName: string;
  totalCount: number;
  expiredCount: number;
  oldestRecord: string | null;
  retentionDays: number;
}

interface HistoryCleanupStats {
  driver: string;
  databaseSizeBytes: number;
  reusableBytes: number;
  isRunning: boolean;
  lastCleanupTime: string | null;
  lastError: string;
  tables: HistoryTableStats[];
}

type HistoryCleanupStatus = Pick<
  HistoryCleanupStats,
  "isRunning" | "lastCleanupTime" | "lastError"
>;

interface ApiEnvelope<T> {
  success: boolean;
  data?: T;
  message?: string;
  error?: string;
}

const DEFAULT_CONFIG: HistoryCleanupConfig = {
  autoCleanupEnabled: true,
  serviceHistoryRetentionDays: 7,
  summaryRetentionDays: 365,
  dashboardRetentionDays: 365,
  operationLogRetentionDays: 90,
  batchSize: 1000,
  scheduleTime: "03:15",
};

const SQLITE_COMPACTION_THRESHOLD_BYTES = 256 * 1024 * 1024;
const SQLITE_MAINTENANCE_DOCS = {
  "zh-CN":
    "https://github.com/NodePassProject/Nowhere/blob/main/docs/operations.md",
  "en-US":
    "https://github.com/NodePassProject/Nowhere/blob/main/docs/operations.md",
} as const;

const toEditableConfig = (
  config: HistoryCleanupConfig,
): EditableCleanupConfig => ({
  autoCleanupEnabled: config.autoCleanupEnabled,
  serviceHistoryRetentionDays: config.serviceHistoryRetentionDays,
  summaryRetentionDays: config.summaryRetentionDays,
  dashboardRetentionDays: config.dashboardRetentionDays,
  operationLogRetentionDays: config.operationLogRetentionDays,
});

const normalizeConfig = (
  config: Partial<HistoryCleanupConfig>,
): HistoryCleanupConfig => ({
  autoCleanupEnabled:
    typeof config.autoCleanupEnabled === "boolean"
      ? config.autoCleanupEnabled
      : DEFAULT_CONFIG.autoCleanupEnabled,
  serviceHistoryRetentionDays:
    config.serviceHistoryRetentionDays ??
    DEFAULT_CONFIG.serviceHistoryRetentionDays,
  summaryRetentionDays:
    config.summaryRetentionDays ?? DEFAULT_CONFIG.summaryRetentionDays,
  dashboardRetentionDays:
    config.dashboardRetentionDays ?? DEFAULT_CONFIG.dashboardRetentionDays,
  operationLogRetentionDays:
    config.operationLogRetentionDays ??
    DEFAULT_CONFIG.operationLogRetentionDays,
  batchSize: config.batchSize ?? DEFAULT_CONFIG.batchSize,
  scheduleTime: config.scheduleTime || DEFAULT_CONFIG.scheduleTime,
});

const normalizeStats = (
  stats: Partial<HistoryCleanupStats>,
): HistoryCleanupStats => ({
  driver: stats.driver || "-",
  databaseSizeBytes: stats.databaseSizeBytes ?? 0,
  reusableBytes: stats.reusableBytes ?? 0,
  isRunning: stats.isRunning ?? false,
  lastCleanupTime: stats.lastCleanupTime || null,
  lastError: stats.lastError || "",
  tables: stats.tables ?? [],
});

async function readApiData<T>(response: Response): Promise<T> {
  let payload: ApiEnvelope<T>;

  try {
    payload = (await response.json()) as ApiEnvelope<T>;
  } catch {
    throw new Error(`HTTP ${response.status}`);
  }

  if (!response.ok || !payload.success || payload.data === undefined) {
    throw new Error(
      payload.error || payload.message || `HTTP ${response.status}`,
    );
  }

  return payload.data;
}

export default function HistoryCleanupSettings() {
  const { t, i18n } = useTranslation("settings");
  const [config, setConfig] = useState<HistoryCleanupConfig | null>(null);
  const [formConfig, setFormConfig] = useState<EditableCleanupConfig>(
    toEditableConfig(DEFAULT_CONFIG),
  );
  const [stats, setStats] = useState<HistoryCleanupStats | null>(null);
  const [preview, setPreview] = useState<HistoryCleanupStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [saving, setSaving] = useState(false);
  const [previewing, setPreviewing] = useState(false);
  const [triggering, setTriggering] = useState(false);
  const [awaitingRun, setAwaitingRun] = useState(false);
  const [errorMessage, setErrorMessage] = useState("");
  const triggeredHere = useRef(false);
  const { isOpen, onOpen, onClose } = useDisclosure();

  const locale = i18n.resolvedLanguage?.startsWith("zh") ? "zh-CN" : "en-US";

  const fetchConfig = useCallback(async () => {
    const response = await apiGet("/api/history-cleanup/config");
    const data = await readApiData<HistoryCleanupConfig>(response);

    return normalizeConfig(data);
  }, []);

  const fetchStats = useCallback(async () => {
    const response = await apiGet("/api/history-cleanup/stats");
    const data = await readApiData<HistoryCleanupStats>(response);

    return normalizeStats(data);
  }, []);

  const fetchStatus = useCallback(async () => {
    const response = await apiGet("/api/history-cleanup/status");

    return readApiData<HistoryCleanupStatus>(response);
  }, []);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setLoading(true);
      try {
        const [nextConfig, nextStats] = await Promise.all([
          fetchConfig(),
          fetchStats(),
        ]);

        if (cancelled) return;
        setConfig(nextConfig);
        setFormConfig(toEditableConfig(nextConfig));
        setStats(nextStats);
        setErrorMessage("");
      } catch (error) {
        if (cancelled) return;
        setErrorMessage(
          error instanceof Error
            ? error.message
            : t("historyCleanup.errors.load"),
        );
      } finally {
        if (!cancelled) setLoading(false);
      }
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, [fetchConfig, fetchStats, t]);

  const isBusy = Boolean(triggering || awaitingRun || stats?.isRunning);

  useEffect(() => {
    if (!isBusy || triggering) return;

    let cancelled = false;
    const poll = async () => {
      try {
        const nextStatus = await fetchStatus();

        if (cancelled) return;
        setStats((current) =>
          current ? { ...current, ...nextStatus } : current,
        );
        setErrorMessage("");

        if (!nextStatus.isRunning) {
          const nextStats = await fetchStats();

          if (cancelled) return;
          setStats(nextStats);
          setAwaitingRun(false);

          if (!triggeredHere.current) return;
          triggeredHere.current = false;
          if (nextStats.lastError) {
            addToast({
              title: t("historyCleanup.toast.cleanupFailed"),
              description: nextStats.lastError,
              color: "danger",
            });
          } else {
            addToast({
              title: t("historyCleanup.toast.cleanupCompleted"),
              description: t("historyCleanup.toast.cleanupCompletedDesc"),
              color: "success",
            });
          }
        }
      } catch (error) {
        if (!cancelled) {
          setErrorMessage(
            error instanceof Error
              ? error.message
              : t("historyCleanup.errors.refresh"),
          );
        }
      }
    };

    const firstPoll = window.setTimeout(() => void poll(), 1200);
    const interval = window.setInterval(() => void poll(), 3000);

    return () => {
      cancelled = true;
      window.clearTimeout(firstPoll);
      window.clearInterval(interval);
    };
  }, [fetchStats, fetchStatus, isBusy, t, triggering]);

  const validationErrors = useMemo(
    () => ({
      serviceHistory:
        Number.isInteger(formConfig.serviceHistoryRetentionDays) &&
        formConfig.serviceHistoryRetentionDays >= 2 &&
        formConfig.serviceHistoryRetentionDays <= 30
          ? ""
          : t("historyCleanup.validation.serviceHistory"),
      summary:
        Number.isInteger(formConfig.summaryRetentionDays) &&
        formConfig.summaryRetentionDays >= 8 &&
        formConfig.summaryRetentionDays <= 3650
          ? ""
          : t("historyCleanup.validation.summary"),
      dashboard:
        Number.isInteger(formConfig.dashboardRetentionDays) &&
        formConfig.dashboardRetentionDays >= 1 &&
        formConfig.dashboardRetentionDays <= 3650
          ? ""
          : t("historyCleanup.validation.dashboard"),
      operationLog:
        Number.isInteger(formConfig.operationLogRetentionDays) &&
        formConfig.operationLogRetentionDays >= 1 &&
        formConfig.operationLogRetentionDays <= 3650
          ? ""
          : t("historyCleanup.validation.operationLog"),
    }),
    [formConfig, t],
  );

  const isValid = Object.values(validationErrors).every((message) => !message);
  const isDirty = Boolean(
    config &&
      (Object.keys(formConfig) as Array<keyof EditableCleanupConfig>).some(
        (key) => formConfig[key] !== config[key],
      ),
  );

  const updateNumber = (
    key: keyof Omit<EditableCleanupConfig, "autoCleanupEnabled">,
    value: string,
  ) => {
    const parsed = Number.parseInt(value, 10);

    setFormConfig((current) => ({
      ...current,
      [key]: Number.isNaN(parsed) ? 0 : parsed,
    }));
  };

  const getFriendlyError = (error: unknown, fallbackKey: string) => {
    if (error instanceof Error && error.message === "cleanup_in_progress") {
      return t("historyCleanup.errors.busy");
    }
    if (error instanceof Error && error.message) return error.message;

    return t(fallbackKey);
  };

  const handleRefresh = async () => {
    setRefreshing(true);
    try {
      const [nextConfig, nextStats] = await Promise.all([
        fetchConfig(),
        fetchStats(),
      ]);

      setConfig(nextConfig);
      if (!isDirty) setFormConfig(toEditableConfig(nextConfig));
      setStats(nextStats);
      setErrorMessage("");
      addToast({
        title: t("historyCleanup.toast.refreshed"),
        description: t("historyCleanup.toast.refreshedDesc"),
        color: "success",
      });
    } catch (error) {
      const message = getFriendlyError(error, "historyCleanup.errors.refresh");

      setErrorMessage(message);
      addToast({
        title: t("historyCleanup.toast.refreshFailed"),
        description: message,
        color: "danger",
      });
    } finally {
      setRefreshing(false);
    }
  };

  const handleSave = async () => {
    if (!isValid || !isDirty) return;

    setSaving(true);
    try {
      const response = await apiPut("/api/history-cleanup/config", formConfig);
      const nextConfig = normalizeConfig(
        await readApiData<HistoryCleanupConfig>(response),
      );

      setConfig(nextConfig);
      setFormConfig(toEditableConfig(nextConfig));
      setErrorMessage("");

      try {
        setStats(await fetchStats());
      } catch (statsError) {
        setErrorMessage(
          getFriendlyError(statsError, "historyCleanup.errors.refresh"),
        );
      }

      addToast({
        title: t("historyCleanup.toast.saved"),
        description: t("historyCleanup.toast.savedDesc"),
        color: "success",
      });
    } catch (error) {
      const message = getFriendlyError(error, "historyCleanup.errors.save");

      setErrorMessage(message);
      addToast({
        title: t("historyCleanup.toast.saveFailed"),
        description: message,
        color: "danger",
      });
    } finally {
      setSaving(false);
    }
  };

  const handlePreview = async () => {
    setPreviewing(true);
    try {
      const response = await apiPost("/api/history-cleanup/preview");
      const data = normalizeStats(
        await readApiData<HistoryCleanupStats>(response),
      );

      setPreview(data);
      setErrorMessage("");
      onOpen();
    } catch (error) {
      const message = getFriendlyError(error, "historyCleanup.errors.preview");

      setErrorMessage(message);
      addToast({
        title: t("historyCleanup.toast.previewFailed"),
        description: message,
        color: "danger",
      });
    } finally {
      setPreviewing(false);
    }
  };

  const handleTrigger = async () => {
    setTriggering(true);
    try {
      const response = await apiPost("/api/history-cleanup/trigger");

      await readApiData<{ started: boolean }>(response);
      triggeredHere.current = true;
      setAwaitingRun(true);
      setStats((current) =>
        current ? { ...current, isRunning: true, lastError: "" } : current,
      );
      setErrorMessage("");
      onClose();
      addToast({
        title: t("historyCleanup.toast.cleanupStarted"),
        description: t("historyCleanup.toast.cleanupStartedDesc"),
        color: "success",
      });
    } catch (error) {
      const message = getFriendlyError(error, "historyCleanup.errors.trigger");

      setErrorMessage(message);
      addToast({
        title: t("historyCleanup.toast.cleanupFailed"),
        description: message,
        color: "danger",
      });
    } finally {
      setTriggering(false);
    }
  };

  const formatNumber = (value: number) =>
    new Intl.NumberFormat(locale).format(value || 0);

  const formatBytes = (value: number) => {
    if (!Number.isFinite(value) || value <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB", "TB"];
    const unitIndex = Math.min(
      Math.floor(Math.log(value) / Math.log(1024)),
      units.length - 1,
    );
    const amount = value / 1024 ** unitIndex;

    return `${amount.toLocaleString(locale, { maximumFractionDigits: 1 })} ${units[unitIndex]}`;
  };

  const formatDate = (value: string | null) => {
    if (!value) return t("historyCleanup.overview.never");
    const date = new Date(
      value.includes("T") ? value : value.replace(" ", "T"),
    );

    return Number.isNaN(date.getTime())
      ? value
      : new Intl.DateTimeFormat(locale, {
          dateStyle: "medium",
          timeStyle: "short",
        }).format(date);
  };

  const shouldSuggestSQLiteCompaction = Boolean(
    stats?.driver.toLowerCase() === "sqlite" &&
      stats.reusableBytes > SQLITE_COMPACTION_THRESHOLD_BYTES,
  );
  const sqliteMaintenanceDocsUrl = SQLITE_MAINTENANCE_DOCS[locale];

  const getTableLabel = (name: string) => {
    const labels: Record<string, string> = {
      service_history: t("historyCleanup.tables.serviceHistory"),
      traffic_hourly_summary: t("historyCleanup.tables.hourlySummary"),
      dashboard_traffic_summary: t("historyCleanup.tables.dashboardSummary"),
      tunnel_operation_logs: t("historyCleanup.tables.operationLogs"),
      endpoint_sse: t("historyCleanup.tables.endpointEvents"),
    };

    return (
      labels[name] ||
      name
        .split("_")
        .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
        .join(" ")
    );
  };

  const previewTotal =
    preview?.tables.reduce((total, table) => total + table.expiredCount, 0) ??
    0;

  if (loading) {
    return (
      <Card className="p-2">
        <CardBody className="flex h-48 items-center justify-center">
          <Progress
            isIndeterminate
            className="max-w-md"
            label={t("historyCleanup.loading")}
            size="sm"
          />
        </CardBody>
      </Card>
    );
  }

  const statusColor = isBusy
    ? "primary"
    : stats?.lastError
      ? "danger"
      : config?.autoCleanupEnabled
        ? "success"
        : "default";
  const statusLabel = isBusy
    ? t("historyCleanup.status.running")
    : stats?.lastError
      ? t("historyCleanup.status.error")
      : config?.autoCleanupEnabled
        ? t("historyCleanup.status.enabled")
        : t("historyCleanup.status.disabled");

  return (
    <Card className="p-2 overflow-hidden">
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <div className="flex min-w-0 flex-1 items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-success/10 text-success">
            <Icon icon="solar:database-bold" width={22} />
          </div>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-lg font-semibold">
                {t("historyCleanup.title")}
              </p>
              <Chip color={statusColor} size="sm" variant="flat">
                {statusLabel}
              </Chip>
            </div>
            <p className="text-sm text-default-500">
              {t("historyCleanup.description")}
            </p>
          </div>
        </div>
        <Button
          className="w-full sm:w-auto"
          isDisabled={isBusy}
          isLoading={refreshing}
          size="sm"
          startContent={
            !refreshing && <Icon icon="solar:refresh-bold" width={18} />
          }
          variant="flat"
          onPress={handleRefresh}
        >
          {t("historyCleanup.actions.refresh")}
        </Button>
      </CardHeader>

      {(errorMessage || stats?.lastError) && (
        <div className="mx-3 mb-3 flex items-start gap-3 rounded-lg border border-danger/20 bg-danger/10 p-3 text-danger sm:mx-5">
          <Icon
            className="mt-0.5 shrink-0"
            icon="solar:danger-triangle-bold"
            width={18}
          />
          <div className="min-w-0">
            <p className="text-sm font-medium">
              {t("historyCleanup.errors.title")}
            </p>
            <p className="break-words text-xs opacity-90">
              {errorMessage || stats?.lastError}
            </p>
          </div>
        </div>
      )}

      {isBusy && (
        <Progress
          isIndeterminate
          aria-label={t("historyCleanup.status.running")}
          classNames={{ base: "px-3 sm:px-5", track: "h-1" }}
          color="primary"
          size="sm"
        />
      )}

      <Divider className="mt-2" />
      <CardBody className="gap-7 px-3 py-5 sm:px-5">
        <section aria-labelledby="history-cleanup-overview">
          <h3
            className="mb-3 text-sm font-semibold text-default-700"
            id="history-cleanup-overview"
          >
            {t("historyCleanup.overview.title")}
          </h3>
          <dl className="grid grid-cols-2 overflow-hidden rounded-lg border border-divider lg:grid-cols-5">
            <div className="border-b border-r border-divider p-3 lg:border-b-0">
              <dt className="text-xs text-default-500">
                {t("historyCleanup.overview.driver")}
              </dt>
              <dd className="mt-1 truncate text-sm font-semibold uppercase">
                {stats?.driver || "-"}
              </dd>
            </div>
            <div className="border-b border-divider p-3 lg:border-b-0 lg:border-r">
              <dt className="text-xs text-default-500">
                {t("historyCleanup.overview.databaseSize")}
              </dt>
              <dd className="mt-1 text-sm font-semibold">
                {formatBytes(stats?.databaseSizeBytes ?? 0)}
              </dd>
            </div>
            <div className="border-b border-r border-divider p-3 lg:border-b-0">
              <dt className="text-xs text-default-500">
                {t("historyCleanup.overview.reusableSpace")}
              </dt>
              <dd className="mt-1 text-sm font-semibold">
                {formatBytes(stats?.reusableBytes ?? 0)}
              </dd>
            </div>
            <div className="border-b border-divider p-3 lg:border-b-0 lg:border-r">
              <dt className="text-xs text-default-500">
                {t("historyCleanup.overview.schedule")}
              </dt>
              <dd className="mt-1 text-sm font-semibold">
                {config?.scheduleTime || "-"}
              </dd>
            </div>
            <div className="col-span-2 p-3 lg:col-span-1">
              <dt className="text-xs text-default-500">
                {t("historyCleanup.overview.lastCleanup")}
              </dt>
              <dd className="mt-1 truncate text-sm font-semibold">
                {formatDate(stats?.lastCleanupTime ?? null)}
              </dd>
            </div>
          </dl>

          {shouldSuggestSQLiteCompaction && (
            <div className="mt-3 flex flex-col gap-3 rounded-lg border border-warning/30 bg-warning/10 p-3 sm:flex-row sm:items-center sm:justify-between">
              <div className="flex min-w-0 items-start gap-3">
                <Icon
                  className="mt-0.5 shrink-0 text-warning"
                  icon="solar:info-circle-bold"
                  width={19}
                />
                <div className="min-w-0">
                  <p className="text-sm font-medium text-warning-700 dark:text-warning-400">
                    {t("historyCleanup.compaction.title")}
                  </p>
                  <p className="mt-0.5 text-xs text-default-600">
                    {t("historyCleanup.compaction.description", {
                      size: formatBytes(stats?.reusableBytes ?? 0),
                    })}
                  </p>
                </div>
              </div>
              <Button
                as="a"
                className="w-full shrink-0 sm:w-auto"
                color="warning"
                endContent={<Icon icon="lucide:external-link" width={15} />}
                href={sqliteMaintenanceDocsUrl}
                rel="noopener noreferrer"
                size="sm"
                target="_blank"
                variant="flat"
              >
                {t("historyCleanup.compaction.action")}
              </Button>
            </div>
          )}
        </section>

        <section aria-labelledby="history-cleanup-policy">
          <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 className="text-sm font-semibold" id="history-cleanup-policy">
                {t("historyCleanup.config.title")}
              </h3>
              <p className="text-xs text-default-500">
                {t("historyCleanup.config.description", {
                  count: config?.batchSize ?? DEFAULT_CONFIG.batchSize,
                })}
              </p>
            </div>
            <Switch
              isSelected={formConfig.autoCleanupEnabled}
              size="sm"
              onValueChange={(autoCleanupEnabled) =>
                setFormConfig((current) => ({
                  ...current,
                  autoCleanupEnabled,
                }))
              }
            >
              <span className="text-sm">
                {t("historyCleanup.config.autoCleanup")}
              </span>
            </Switch>
          </div>

          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <Input
              description={t("historyCleanup.config.serviceHistoryDesc")}
              endContent={
                <span className="text-xs text-default-400">
                  {t("historyCleanup.config.days")}
                </span>
              }
              errorMessage={validationErrors.serviceHistory}
              isInvalid={Boolean(validationErrors.serviceHistory)}
              label={t("historyCleanup.config.serviceHistory")}
              max={30}
              min={2}
              type="number"
              value={String(formConfig.serviceHistoryRetentionDays)}
              variant="bordered"
              onValueChange={(value) =>
                updateNumber("serviceHistoryRetentionDays", value)
              }
            />
            <Input
              description={t("historyCleanup.config.summaryDesc")}
              endContent={
                <span className="text-xs text-default-400">
                  {t("historyCleanup.config.days")}
                </span>
              }
              errorMessage={validationErrors.summary}
              isInvalid={Boolean(validationErrors.summary)}
              label={t("historyCleanup.config.summary")}
              max={3650}
              min={8}
              type="number"
              value={String(formConfig.summaryRetentionDays)}
              variant="bordered"
              onValueChange={(value) =>
                updateNumber("summaryRetentionDays", value)
              }
            />
            <Input
              description={t("historyCleanup.config.dashboardDesc")}
              endContent={
                <span className="text-xs text-default-400">
                  {t("historyCleanup.config.days")}
                </span>
              }
              errorMessage={validationErrors.dashboard}
              isInvalid={Boolean(validationErrors.dashboard)}
              label={t("historyCleanup.config.dashboard")}
              max={3650}
              min={1}
              type="number"
              value={String(formConfig.dashboardRetentionDays)}
              variant="bordered"
              onValueChange={(value) =>
                updateNumber("dashboardRetentionDays", value)
              }
            />
            <Input
              description={t("historyCleanup.config.operationLogDesc")}
              endContent={
                <span className="text-xs text-default-400">
                  {t("historyCleanup.config.days")}
                </span>
              }
              errorMessage={validationErrors.operationLog}
              isInvalid={Boolean(validationErrors.operationLog)}
              label={t("historyCleanup.config.operationLog")}
              max={3650}
              min={1}
              type="number"
              value={String(formConfig.operationLogRetentionDays)}
              variant="bordered"
              onValueChange={(value) =>
                updateNumber("operationLogRetentionDays", value)
              }
            />
          </div>

          <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p className="text-xs text-default-500">
              {t("historyCleanup.config.executionPlan", {
                time: config?.scheduleTime || DEFAULT_CONFIG.scheduleTime,
                count: config?.batchSize ?? DEFAULT_CONFIG.batchSize,
              })}
            </p>
            <Button
              className="w-full sm:w-auto"
              color="primary"
              isDisabled={!isDirty || !isValid || isBusy}
              isLoading={saving}
              size="sm"
              startContent={
                !saving && <Icon icon="solar:diskette-bold" width={17} />
              }
              onPress={handleSave}
            >
              {t("historyCleanup.actions.save")}
            </Button>
          </div>
        </section>

        <section aria-labelledby="history-cleanup-data">
          <div className="mb-3 flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <h3 className="text-sm font-semibold" id="history-cleanup-data">
                {t("historyCleanup.stats.title")}
              </h3>
              <p className="text-xs text-default-500">
                {t("historyCleanup.stats.description")}
              </p>
            </div>
            {isDirty && (
              <p className="text-xs text-warning">
                {t("historyCleanup.config.saveBeforePreview")}
              </p>
            )}
          </div>

          {stats?.tables.length ? (
            <div className="overflow-hidden rounded-lg border border-divider">
              <div className="hidden grid-cols-[minmax(190px,1.4fr)_minmax(90px,.7fr)_minmax(90px,.7fr)_minmax(160px,1fr)_minmax(90px,.65fr)] gap-4 bg-default-100 px-4 py-2 text-xs font-medium text-default-500 lg:grid">
                <span>{t("historyCleanup.stats.table")}</span>
                <span>{t("historyCleanup.stats.total")}</span>
                <span>{t("historyCleanup.stats.expired")}</span>
                <span>{t("historyCleanup.stats.oldest")}</span>
                <span>{t("historyCleanup.stats.retention")}</span>
              </div>
              {stats.tables.map((table) => (
                <div
                  key={table.tableName}
                  className="grid grid-cols-2 gap-x-4 gap-y-3 border-t border-divider px-4 py-4 first:border-t-0 lg:grid-cols-[minmax(190px,1.4fr)_minmax(90px,.7fr)_minmax(90px,.7fr)_minmax(160px,1fr)_minmax(90px,.65fr)] lg:items-center lg:gap-4 lg:first:border-t"
                >
                  <div className="col-span-2 flex min-w-0 items-center gap-2 lg:col-span-1">
                    <Icon
                      className="shrink-0 text-default-400"
                      icon="solar:database-linear"
                      width={18}
                    />
                    <span className="truncate text-sm font-medium">
                      {getTableLabel(table.tableName)}
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs text-default-400 lg:hidden">
                      {t("historyCleanup.stats.total")}
                    </span>
                    <span className="text-sm tabular-nums">
                      {formatNumber(table.totalCount)}
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs text-default-400 lg:hidden">
                      {t("historyCleanup.stats.expired")}
                    </span>
                    <Chip
                      color={table.expiredCount > 0 ? "warning" : "default"}
                      size="sm"
                      variant="flat"
                    >
                      {formatNumber(table.expiredCount)}
                    </Chip>
                  </div>
                  <div>
                    <span className="block text-xs text-default-400 lg:hidden">
                      {t("historyCleanup.stats.oldest")}
                    </span>
                    <span className="text-xs text-default-600">
                      {formatDate(table.oldestRecord)}
                    </span>
                  </div>
                  <div>
                    <span className="block text-xs text-default-400 lg:hidden">
                      {t("historyCleanup.stats.retention")}
                    </span>
                    <span className="text-sm tabular-nums">
                      {t("historyCleanup.stats.retentionValue", {
                        days: table.retentionDays,
                      })}
                    </span>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="rounded-lg border border-dashed border-divider px-4 py-8 text-center text-sm text-default-500">
              {t("historyCleanup.stats.noData")}
            </div>
          )}

          <div className="mt-4 flex justify-end">
            <Button
              className="w-full sm:w-auto"
              color="warning"
              isDisabled={!config || isDirty || isBusy}
              isLoading={previewing}
              startContent={
                !previewing && <Icon icon="solar:eye-bold" width={18} />
              }
              variant="flat"
              onPress={handlePreview}
            >
              {t("historyCleanup.actions.preview")}
            </Button>
          </div>
        </section>
      </CardBody>

      <Modal
        isDismissable={!triggering}
        isKeyboardDismissDisabled={triggering}
        isOpen={isOpen}
        scrollBehavior="inside"
        size="2xl"
        onClose={onClose}
      >
        <ModalContent>
          <ModalHeader className="flex items-center gap-2">
            <Icon className="text-warning" icon="solar:eye-bold" width={22} />
            {t("historyCleanup.preview.title")}
          </ModalHeader>
          <ModalBody>
            <div className="rounded-lg border border-warning/20 bg-warning/10 p-3">
              <p className="text-sm font-medium text-warning-700 dark:text-warning-400">
                {t("historyCleanup.preview.total", {
                  count: previewTotal,
                })}
              </p>
              <p className="mt-1 text-xs text-default-600">
                {t("historyCleanup.preview.warning")}
              </p>
            </div>

            <div className="divide-y divide-divider rounded-lg border border-divider">
              {preview?.tables.map((table) => (
                <div
                  key={table.tableName}
                  className="flex items-center justify-between gap-4 px-4 py-3"
                >
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">
                      {getTableLabel(table.tableName)}
                    </p>
                    <p className="text-xs text-default-500">
                      {t("historyCleanup.stats.retentionValue", {
                        days: table.retentionDays,
                      })}
                    </p>
                  </div>
                  <span className="shrink-0 text-sm font-semibold tabular-nums text-warning-600">
                    {formatNumber(table.expiredCount)}
                  </span>
                </div>
              ))}
              {!preview?.tables.length && (
                <p className="px-4 py-6 text-center text-sm text-default-500">
                  {t("historyCleanup.preview.noExpired")}
                </p>
              )}
            </div>
          </ModalBody>
          <ModalFooter>
            <Button isDisabled={triggering} variant="light" onPress={onClose}>
              {t("historyCleanup.actions.cancel")}
            </Button>
            <Button
              color="danger"
              isDisabled={previewTotal === 0}
              isLoading={triggering}
              startContent={
                !triggering && (
                  <Icon icon="solar:trash-bin-trash-bold" width={18} />
                )
              }
              onPress={handleTrigger}
            >
              {t("historyCleanup.actions.cleanupNow")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Card>
  );
}
