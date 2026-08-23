import {
  Button,
  Card,
  CardBody,
  Chip,
  Divider,
  Input,
  Progress,
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
import { Icon } from "@iconify/react/dist/offline";
import { addToast } from "@heroui/toast";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";

import SubscriptionFormModal from "@/components/subscriptions/subscription-form-modal";
import SubscriptionImportModal from "@/components/subscriptions/subscription-import-modal";
import { ConfirmationModal } from "@/components/ui/confirmation-modal";
import {
  deleteSubscription,
  listPortalOptions,
  listSubscriptions,
  PortalOption,
  PortalSubscription,
  resetSubscriptionTraffic,
  rotateSubscriptionToken,
} from "@/lib/subscriptions-api";

type StatusFilter = "all" | "overLimit" | "expired";
type ConfirmationAction = "rotate" | "reset" | "delete";

interface PendingConfirmation {
  action: ConfirmationAction;
  subscription: PortalSubscription;
}

const formatBytes = (bytes: number | null | undefined) => {
  if (!bytes || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  const index = Math.min(
    Math.floor(Math.log(bytes) / Math.log(1024)),
    units.length - 1,
  );

  return `${(bytes / 1024 ** index).toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
};

const isExpired = (subscription: PortalSubscription) =>
  Boolean(
    subscription.expiresAt &&
      new Date(subscription.expiresAt).getTime() <= Date.now(),
  );

const getRemainingDays = (expiresAt: string | null) => {
  if (!expiresAt) return null;

  const timestamp = new Date(expiresAt).getTime();

  if (Number.isNaN(timestamp)) return null;

  return Math.max(
    0,
    Math.ceil((timestamp - Date.now()) / (24 * 60 * 60 * 1000)),
  );
};

export default function SubscriptionsPage() {
  const { t, i18n } = useTranslation("subscriptions");
  const [searchParams, setSearchParams] = useSearchParams();
  const [subscriptions, setSubscriptions] = useState<PortalSubscription[]>([]);
  const [portals, setPortals] = useState<PortalOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<PortalSubscription | null>(null);
  const [pending, setPending] = useState<PendingConfirmation | null>(null);
  const [actionLoading, setActionLoading] = useState(false);
  const [importTarget, setImportTarget] = useState<PortalSubscription | null>(
    null,
  );

  const loadData = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [nextSubscriptions, nextPortals] = await Promise.all([
        listSubscriptions(),
        listPortalOptions(),
      ]);

      setSubscriptions(nextSubscriptions);
      setPortals(nextPortals);
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t("toast.loadFailed");

      setError(message);
      addToast({
        title: t("toast.loadFailed"),
        description: message,
        color: "danger",
      });
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    if (searchParams.get("create") !== "1") return;

    setCreateOpen(true);
    const nextParams = new URLSearchParams(searchParams);

    nextParams.delete("create");
    setSearchParams(nextParams, { replace: true });
  }, [searchParams, setSearchParams]);

  const filteredSubscriptions = useMemo(() => {
    const query = search.trim().toLocaleLowerCase();

    return subscriptions.filter((subscription) => {
      const matchesSearch =
        !query ||
        subscription.name.toLocaleLowerCase().includes(query) ||
        subscription.profileTitle.toLocaleLowerCase().includes(query);

      if (!matchesSearch) return false;
      if (status === "overLimit") return subscription.overLimit;
      if (status === "expired") return isExpired(subscription);

      return true;
    });
  }, [search, status, subscriptions]);

  const formatDate = useCallback(
    (value: string | null) => {
      if (!value) return t("values.never");
      const date = new Date(value);

      if (Number.isNaN(date.getTime())) return "-";

      return new Intl.DateTimeFormat(i18n.language, {
        year: "numeric",
        month: "short",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
      }).format(date);
    },
    [i18n.language, t],
  );

  const expirationView = (value: string | null) => {
    const remainingDays = getRemainingDays(value);

    return (
      <div className="space-y-0.5">
        <p className="text-sm text-default-500">{formatDate(value)}</p>
        {remainingDays !== null && (
          <p
            className={
              remainingDays === 0
                ? "text-xs text-danger"
                : "text-xs text-default-400"
            }
          >
            {t("values.remainingDays", { count: remainingDays })}
          </p>
        )}
      </div>
    );
  };

  const openImport = (subscription: PortalSubscription) => {
    setImportTarget(subscription);
  };

  const runConfirmedAction = async () => {
    if (!pending) return;
    setActionLoading(true);
    try {
      if (pending.action === "delete") {
        await deleteSubscription(pending.subscription.id);
        addToast({ title: t("toast.deleted"), color: "success" });
      } else if (pending.action === "rotate") {
        await rotateSubscriptionToken(pending.subscription.id);
        addToast({ title: t("toast.rotated"), color: "success" });
      } else {
        await resetSubscriptionTraffic(pending.subscription.id);
        addToast({ title: t("toast.trafficReset"), color: "success" });
      }
      setPending(null);
      await loadData();
    } catch (error) {
      addToast({
        title: t("toast.actionFailed"),
        description:
          error instanceof Error ? error.message : t("toast.actionFailed"),
        color: "danger",
      });
    } finally {
      setActionLoading(false);
    }
  };

  const trafficView = (subscription: PortalSubscription) => {
    const finite =
      subscription.trafficLimit != null && subscription.trafficLimit > 0;
    const percentage = finite
      ? Math.min(
          (subscription.trafficUsed / (subscription.trafficLimit as number)) *
            100,
          100,
        )
      : 0;

    return (
      <div className="min-w-32 space-y-1.5">
        <div className="flex items-baseline justify-between gap-2 text-xs">
          <span className="font-mono text-default-700">
            {formatBytes(subscription.trafficUsed)}
          </span>
          <span className="text-default-400">
            {finite
              ? `/ ${formatBytes(subscription.trafficLimit)}`
              : t("values.unlimited")}
          </span>
        </div>
        {finite && (
          <Progress
            aria-label={t("fields.traffic")}
            color={subscription.overLimit ? "danger" : "primary"}
            size="sm"
            value={percentage}
          />
        )}
      </div>
    );
  };

  const resetFilters = () => {
    setSearch("");
    setStatus("all");
  };

  const rowActions = (subscription: PortalSubscription) => {
    return (
      <div className="flex flex-wrap items-center justify-end gap-0.5">
        <Tooltip content={t("actions.scanImport")}>
          <Button
            isIconOnly
            aria-label={`${t("actions.scanImport")}: ${subscription.name}`}
            color="primary"
            size="sm"
            variant="light"
            onPress={() => openImport(subscription)}
          >
            <Icon icon="lucide:qr-code" width={16} />
          </Button>
        </Tooltip>
        <Tooltip content={t("actions.edit")}>
          <Button
            isIconOnly
            aria-label={`${t("actions.edit")}: ${subscription.name}`}
            color="warning"
            size="sm"
            variant="light"
            onPress={() => setEditing(subscription)}
          >
            <Icon icon="lucide:pencil" width={15} />
          </Button>
        </Tooltip>
        <Tooltip content={t("actions.resetTraffic")}>
          <Button
            isIconOnly
            aria-label={`${t("actions.resetTraffic")}: ${subscription.name}`}
            color="warning"
            size="sm"
            variant="light"
            onPress={() => setPending({ action: "reset", subscription })}
          >
            <Icon icon="lucide:refresh-ccw" width={16} />
          </Button>
        </Tooltip>
        <Tooltip content={t("actions.rotateToken")}>
          <Button
            isIconOnly
            aria-label={`${t("actions.rotateToken")}: ${subscription.name}`}
            color="secondary"
            size="sm"
            variant="light"
            onPress={() => setPending({ action: "rotate", subscription })}
          >
            <Icon icon="lucide:key-round" width={16} />
          </Button>
        </Tooltip>
        <Tooltip content={t("actions.delete")}>
          <Button
            isIconOnly
            aria-label={`${t("actions.delete")}: ${subscription.name}`}
            color="danger"
            size="sm"
            variant="light"
            onPress={() => setPending({ action: "delete", subscription })}
          >
            <Icon icon="lucide:trash-2" width={16} />
          </Button>
        </Tooltip>
      </div>
    );
  };

  const confirmationCopy = pending
    ? {
        title: t(`confirm.${pending.action}.title`),
        message: t(`confirm.${pending.action}.message`, {
          name: pending.subscription.name,
        }),
        confirm: t(`confirm.${pending.action}.confirm`),
        color:
          pending.action === "delete"
            ? ("danger" as const)
            : ("warning" as const),
        icon:
          pending.action === "delete"
            ? "lucide:trash-2"
            : pending.action === "rotate"
              ? "lucide:key-round"
              : "lucide:refresh-ccw",
      }
    : null;

  return (
    <div className="w-full">
      <header className="mb-4 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <h1 className="truncate text-2xl font-semibold text-foreground">
            {t("title")}
          </h1>
          {!loading && (
            <Chip
              className="tabular-nums text-default-500"
              size="sm"
              variant="flat"
            >
              {subscriptions.length}
            </Chip>
          )}
        </div>
        <Button
          className="shrink-0"
          color="primary"
          startContent={<Icon icon="lucide:plus" width={17} />}
          onPress={() => setCreateOpen(true)}
        >
          <span className="hidden sm:inline">{t("actions.create")}</span>
          <span className="sm:hidden">{t("actions.createShort")}</span>
        </Button>
      </header>

      <div className="mb-4 flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="grid flex-1 gap-2 sm:grid-cols-[minmax(220px,1fr)_180px] xl:max-w-[500px]">
          <Input
            isClearable
            aria-label={t("search.placeholder")}
            placeholder={t("search.placeholder")}
            size="sm"
            startContent={
              <Icon
                className="text-default-400"
                icon="lucide:search"
                width={16}
              />
            }
            value={search}
            onClear={() => setSearch("")}
            onValueChange={setSearch}
          />
          <Select
            aria-label={t("search.status")}
            selectedKeys={new Set([status])}
            size="sm"
            onSelectionChange={(keys) =>
              setStatus(String(Array.from(keys)[0] ?? "all") as StatusFilter)
            }
          >
            <SelectItem key="all">{t("filter.all")}</SelectItem>
            <SelectItem key="overLimit">{t("filter.overLimit")}</SelectItem>
            <SelectItem key="expired">{t("filter.expired")}</SelectItem>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Divider className="hidden h-5 xl:block" orientation="vertical" />
          <Button
            size="sm"
            startContent={<Icon icon="lucide:calendar-x" width={15} />}
            variant="flat"
            onPress={resetFilters}
          >
            {t("actions.resetFilters")}
          </Button>
          <Button
            isLoading={loading}
            size="sm"
            startContent={
              !loading ? (
                <Icon icon="lucide:refresh-cw" width={15} />
              ) : undefined
            }
            variant="flat"
            onPress={() => void loadData()}
          >
            <span className="hidden sm:inline">{t("actions.refresh")}</span>
          </Button>
        </div>
      </div>

      <Card className="border border-default-100 shadow-none">
        <CardBody className="gap-4 p-4">
          <div className="hidden overflow-x-auto lg:block">
            <Table removeWrapper aria-label={t("title")}>
              <TableHeader>
                <TableColumn minWidth={170}>{t("fields.name")}</TableColumn>
                <TableColumn minWidth={180}>{t("fields.portals")}</TableColumn>
                <TableColumn minWidth={220}>
                  {t("fields.expiresAt")}
                </TableColumn>
                <TableColumn width={160}>{t("fields.traffic")}</TableColumn>
                <TableColumn align="end" width={200}>
                  {t("fields.actions")}
                </TableColumn>
              </TableHeader>
              <TableBody
                emptyContent={
                  <div className="flex min-h-64 flex-col items-center justify-center gap-3 py-10 text-center">
                    <span className="flex size-16 items-center justify-center rounded-full bg-default-100 text-default-400">
                      <Icon
                        icon={error ? "lucide:circle-alert" : "lucide:rss"}
                        width={28}
                      />
                    </span>
                    <div>
                      <p
                        className={
                          error ? "font-medium text-danger" : "font-medium"
                        }
                      >
                        {error || t("empty.title")}
                      </p>
                      <p className="mt-1 text-sm text-default-400">
                        {error ? t("empty.loadFailed") : t("empty.hint")}
                      </p>
                    </div>
                    {error && (
                      <Button
                        size="sm"
                        variant="flat"
                        onPress={() => void loadData()}
                      >
                        {t("empty.retry")}
                      </Button>
                    )}
                  </div>
                }
                isLoading={loading}
                items={filteredSubscriptions}
                loadingContent={<Spinner label={t("empty.loading")} />}
              >
                {(subscription) => {
                  return (
                    <TableRow key={String(subscription.id)}>
                      <TableCell>
                        <p className="max-w-52 truncate font-medium">
                          {subscription.name}
                        </p>
                      </TableCell>
                      <TableCell>
                        <span className="text-sm text-default-600">
                          {t("values.portalCount", {
                            count: subscription.tunnelIds.length,
                          })}
                        </span>
                      </TableCell>
                      <TableCell>
                        {expirationView(subscription.expiresAt)}
                      </TableCell>
                      <TableCell>{trafficView(subscription)}</TableCell>
                      <TableCell>{rowActions(subscription)}</TableCell>
                    </TableRow>
                  );
                }}
              </TableBody>
            </Table>
          </div>

          <div className="grid min-w-0 gap-3 lg:hidden">
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
            ) : error || filteredSubscriptions.length === 0 ? (
              <div className="flex min-h-64 flex-col items-center justify-center gap-3 text-center">
                <Icon
                  className="text-default-400"
                  icon={error ? "lucide:circle-alert" : "lucide:rss"}
                  width={34}
                />
                <div>
                  <p
                    className={
                      error ? "font-medium text-danger" : "font-medium"
                    }
                  >
                    {error || t("empty.title")}
                  </p>
                  <p className="mt-1 text-sm text-default-400">
                    {error ? t("empty.loadFailed") : t("empty.hint")}
                  </p>
                </div>
                {error && (
                  <Button
                    size="sm"
                    variant="flat"
                    onPress={() => void loadData()}
                  >
                    {t("empty.retry")}
                  </Button>
                )}
              </div>
            ) : (
              filteredSubscriptions.map((subscription) => {
                return (
                  <article
                    key={subscription.id}
                    className="w-full min-w-0 max-w-full overflow-hidden rounded-lg border border-default-200 p-4"
                  >
                    <div className="flex items-start gap-3">
                      <div className="min-w-0">
                        <p className="truncate font-medium">
                          {subscription.name}
                        </p>
                      </div>
                    </div>

                    <dl className="mt-4 grid grid-cols-2 gap-4 text-sm">
                      <div className="min-w-0">
                        <dt className="text-xs text-default-400">
                          {t("fields.portals")}
                        </dt>
                        <dd className="mt-1 truncate">
                          {t("values.portalCount", {
                            count: subscription.tunnelIds.length,
                          })}
                        </dd>
                      </div>
                      <div className="min-w-0">
                        <dt className="text-xs text-default-400">
                          {t("fields.expiresAt")}
                        </dt>
                        <dd className="mt-1">
                          {expirationView(subscription.expiresAt)}
                        </dd>
                      </div>
                    </dl>

                    <div className="mt-4">{trafficView(subscription)}</div>

                    <Divider className="my-3" />
                    <div className="flex items-center justify-end">
                      {rowActions(subscription)}
                    </div>
                  </article>
                );
              })
            )}
          </div>
        </CardBody>
      </Card>

      <SubscriptionFormModal
        isOpen={createOpen}
        portals={portals}
        onOpenChange={setCreateOpen}
        onSaved={loadData}
      />
      {editing && (
        <SubscriptionFormModal
          isOpen
          portals={portals}
          subscription={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null);
          }}
          onSaved={loadData}
        />
      )}

      <SubscriptionImportModal
        isOpen={Boolean(importTarget)}
        subscription={importTarget}
        onOpenChange={(open) => {
          if (!open) {
            setImportTarget(null);
          }
        }}
        onSaved={loadData}
      />

      {pending && confirmationCopy && (
        <ConfirmationModal
          isOpen
          cancelText={t("actions.cancel")}
          confirmColor={confirmationCopy.color}
          confirmText={confirmationCopy.confirm}
          icon={confirmationCopy.icon}
          iconColor={
            pending.action === "delete" ? "text-danger" : "text-warning"
          }
          isLoading={actionLoading}
          message={confirmationCopy.message}
          title={confirmationCopy.title}
          onClose={() => {
            if (!actionLoading) setPending(null);
          }}
          onConfirm={() => void runConfirmedAction()}
        />
      )}
    </div>
  );
}
