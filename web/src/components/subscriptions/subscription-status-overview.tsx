import {
  Button,
  Card,
  CardBody,
  Chip,
  Progress,
  Skeleton,
} from "@heroui/react";
import { Icon } from "@iconify/react/dist/offline";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";

import { listSubscriptions, PortalSubscription } from "@/lib/subscriptions-api";

type SubscriptionStatus = "active" | "expired" | "overLimit";

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

const getStatus = (subscription: PortalSubscription): SubscriptionStatus => {
  if (subscription.overLimit) return "overLimit";
  if (isExpired(subscription)) return "expired";

  return "active";
};

const statusPriority: Record<SubscriptionStatus, number> = {
  overLimit: 0,
  expired: 1,
  active: 2,
};

interface SubscriptionStatusOverviewProps {
  onCountChange?: (count: number | null) => void;
}

export function SubscriptionStatusOverview({
  onCountChange,
}: SubscriptionStatusOverviewProps) {
  const navigate = useNavigate();
  const { t, i18n } = useTranslation("dashboard");
  const mountedRef = useRef(true);
  const [subscriptions, setSubscriptions] = useState<PortalSubscription[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);

  const loadSubscriptions = useCallback(async () => {
    setLoading(true);
    setLoadFailed(false);

    try {
      const nextSubscriptions = await listSubscriptions();

      if (mountedRef.current) setSubscriptions(nextSubscriptions);
    } catch {
      if (mountedRef.current) setLoadFailed(true);
    } finally {
      if (mountedRef.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    void loadSubscriptions();

    return () => {
      mountedRef.current = false;
    };
  }, [loadSubscriptions]);

  useEffect(() => {
    onCountChange?.(loading || loadFailed ? null : subscriptions.length);
  }, [loadFailed, loading, onCountChange, subscriptions.length]);

  const summary = useMemo(() => {
    let active = 0;
    let portalCount = 0;

    subscriptions.forEach((subscription) => {
      if (getStatus(subscription) === "active") active += 1;
      portalCount += subscription.portalCount;
    });

    return {
      active,
      attention: subscriptions.length - active,
      portalCount,
    };
  }, [subscriptions]);

  const sortedSubscriptions = useMemo(
    () =>
      [...subscriptions].sort((left, right) => {
        const priority =
          statusPriority[getStatus(left)] - statusPriority[getStatus(right)];

        if (priority !== 0) return priority;

        return (
          new Date(right.updatedAt).getTime() -
          new Date(left.updatedAt).getTime()
        );
      }),
    [subscriptions],
  );

  const statusView = (status: SubscriptionStatus) => {
    const views = {
      active: {
        color: "success" as const,
        icon: "lucide:circle-check",
      },
      expired: {
        color: "warning" as const,
        icon: "lucide:calendar-x-2",
      },
      overLimit: {
        color: "danger" as const,
        icon: "lucide:gauge",
      },
    };

    return {
      ...views[status],
      label: t(`subscriptions.status.${status}`),
    };
  };

  const formatExpiry = (value: string | null) => {
    if (!value) return t("subscriptions.neverExpires");
    const date = new Date(value);

    if (Number.isNaN(date.getTime())) return "-";

    const formatted = new Intl.DateTimeFormat(i18n.language, {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    }).format(date);

    return t("subscriptions.expiresOn", { date: formatted });
  };

  const trafficView = (subscription: PortalSubscription) => {
    const hasLimit =
      subscription.trafficLimit != null && subscription.trafficLimit > 0;
    const percentage = hasLimit
      ? Math.min(
          (subscription.trafficUsed / (subscription.trafficLimit as number)) *
            100,
          100,
        )
      : 0;
    const progressColor = subscription.overLimit
      ? ("danger" as const)
      : percentage >= 80
        ? ("warning" as const)
        : ("primary" as const);

    return (
      <div className="mt-2 flex h-4 items-center gap-2">
        {hasLimit ? (
          <Progress
            aria-label={t("subscriptions.trafficUsage", {
              name: subscription.name,
            })}
            className="min-w-0 flex-1"
            color={progressColor}
            size="sm"
            value={percentage}
          />
        ) : (
          <div className="h-1 min-w-0 flex-1 rounded-full bg-default-100" />
        )}
        <span className="shrink-0 text-[11px] tabular-nums text-default-500">
          {formatBytes(subscription.trafficUsed)}
          {hasLimit
            ? ` / ${formatBytes(subscription.trafficLimit)}`
            : ` / ${t("subscriptions.unlimited")}`}
        </span>
      </div>
    );
  };

  const summaryItems = [
    {
      key: "active",
      value: summary.active,
      total: subscriptions.length,
      label: t("subscriptions.activeSummary"),
      icon: "lucide:circle-check",
      color: "text-success",
    },
    {
      key: "attention",
      value: summary.attention,
      label: t("subscriptions.attentionSummary"),
      icon: "lucide:circle-alert",
      color: summary.attention > 0 ? "text-warning" : "text-default-400",
    },
    {
      key: "portals",
      value: summary.portalCount,
      label: t("subscriptions.portalSummary"),
      icon: "lucide:waypoints",
      color: "text-primary",
    },
  ];

  return (
    <Card
      aria-labelledby="subscription-status-title"
      as="section"
      className="h-[469px] border border-transparent dark:border-default-100"
    >
      <CardBody className="flex min-h-0 flex-col p-5">
        <h2
          className="text-base font-semibold text-foreground"
          id="subscription-status-title"
        >
          {t("subscriptions.title")}
        </h2>

        {loading ? (
          <div className="mt-3 flex min-h-0 flex-1 flex-col">
            <div className="grid grid-cols-3 gap-2">
              {[0, 1, 2].map((item) => (
                <div
                  key={item}
                  className="flex h-[82px] flex-col justify-center gap-2 rounded-lg bg-default-100/70 p-3 dark:bg-default-50"
                >
                  <Skeleton className="h-6 w-10 rounded-md" />
                  <Skeleton className="h-3 w-14 rounded-md" />
                </div>
              ))}
            </div>
            <div className="mt-3 space-y-2 overflow-hidden">
              {[0, 1, 2].map((item) => (
                <div
                  key={item}
                  className="flex h-[96px] flex-col justify-center gap-2 rounded-lg border border-default-200/70 bg-content1 p-3 dark:border-default-100"
                >
                  <div className="flex justify-between gap-4">
                    <Skeleton className="h-4 w-28 rounded-md" />
                    <Skeleton className="h-5 w-16 rounded-full" />
                  </div>
                  <Skeleton className="h-3 w-36 rounded-md" />
                  <Skeleton className="h-1.5 w-full rounded-full" />
                </div>
              ))}
            </div>
          </div>
        ) : loadFailed ? (
          <div className="mt-3 flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg bg-default-100/60 text-center dark:bg-default-50">
            <Icon
              className="text-danger"
              icon="lucide:circle-alert"
              width={30}
            />
            <p className="mt-3 text-sm font-medium">
              {t("subscriptions.loadFailed")}
            </p>
            <Button
              className="mt-3"
              size="sm"
              startContent={<Icon icon="lucide:refresh-cw" width={14} />}
              variant="flat"
              onPress={() => void loadSubscriptions()}
            >
              {t("subscriptions.retry")}
            </Button>
          </div>
        ) : (
          <div className="mt-3 flex min-h-0 flex-1 flex-col">
            <div className="grid grid-cols-3 gap-2">
              {summaryItems.map((item) => (
                <div
                  key={item.key}
                  className="flex h-[82px] flex-col justify-between rounded-lg bg-default-100/70 p-3 dark:bg-default-50"
                >
                  <div className="flex items-start justify-between gap-1">
                    <p
                      className={`text-xl font-semibold leading-none tabular-nums ${item.color}`}
                    >
                      {item.value}
                      {item.total != null && (
                        <span className="ml-1 text-xs font-medium text-default-400">
                          / {item.total}
                        </span>
                      )}
                    </p>
                    <Icon
                      className={`shrink-0 ${item.color}`}
                      icon={item.icon}
                      width={16}
                    />
                  </div>
                  <p className="truncate text-[11px] text-default-500">
                    {item.label}
                  </p>
                </div>
              ))}
            </div>

            {subscriptions.length === 0 ? (
              <div className="mt-3 flex min-h-0 flex-1 flex-col items-center justify-center rounded-lg border border-default-200/70 px-4 text-center dark:border-default-100">
                <div className="flex h-11 w-11 items-center justify-center rounded-lg bg-primary-50 text-primary dark:bg-primary-900/20">
                  <Icon icon="lucide:rss" width={22} />
                </div>
                <p className="mt-3 text-sm font-medium">
                  {t("subscriptions.emptyTitle")}
                </p>
                <p className="mt-1 max-w-64 text-xs leading-5 text-default-500">
                  {t("subscriptions.emptyHint")}
                </p>
                <Button
                  className="mt-4"
                  color="primary"
                  size="sm"
                  startContent={<Icon icon="lucide:plus" width={15} />}
                  onPress={() => navigate("/subscriptions?create=1")}
                >
                  {t("subscriptions.create")}
                </Button>
              </div>
            ) : (
              <div className="scrollbar-hide mt-3 min-h-0 flex-1 space-y-2 overflow-y-auto pr-0.5">
                {sortedSubscriptions.map((subscription) => {
                  const status = getStatus(subscription);
                  const view = statusView(status);

                  return (
                    <article
                      key={subscription.id}
                      className="min-h-[96px] rounded-lg border border-default-200/70 bg-content1 p-3 dark:border-default-100"
                    >
                      <div className="flex min-w-0 items-center justify-between gap-3">
                        <p className="min-w-0 truncate text-sm font-medium text-foreground">
                          {subscription.name}
                        </p>
                        <Chip
                          className="shrink-0"
                          color={view.color}
                          size="sm"
                          startContent={<Icon icon={view.icon} width={13} />}
                          variant="flat"
                        >
                          {view.label}
                        </Chip>
                      </div>
                      <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[11px] text-default-400">
                        <span className="shrink-0">
                          {t("subscriptions.portalCount", {
                            count: subscription.portalCount,
                          })}
                        </span>
                        <span aria-hidden="true">·</span>
                        <span className="min-w-0 truncate">
                          {formatExpiry(subscription.expiresAt)}
                        </span>
                      </div>
                      {trafficView(subscription)}
                    </article>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </CardBody>
    </Card>
  );
}
