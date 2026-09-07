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
      <div className="mt-2.5 flex h-4 items-center gap-2.5">
        {hasLimit ? (
          <Progress
            aria-label={t("subscriptions.trafficUsage", {
              name: subscription.name,
            })}
            className="min-w-0 flex-1"
            classNames={{ track: "bg-default-200/70" }}
            color={progressColor}
            size="sm"
            value={percentage}
          />
        ) : (
          <div className="h-1 min-w-0 flex-1 rounded-full bg-default-200/70" />
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
      iconBackground: "bg-success/10",
    },
    {
      key: "attention",
      value: summary.attention,
      label: t("subscriptions.attentionSummary"),
      icon: "lucide:circle-alert",
      color: summary.attention > 0 ? "text-warning" : "text-default-400",
      iconBackground:
        summary.attention > 0 ? "bg-warning/10" : "bg-default-200/70",
    },
    {
      key: "portals",
      value: summary.portalCount,
      label: t("subscriptions.portalSummary"),
      icon: "lucide:waypoints",
      color: "text-primary",
      iconBackground: "bg-primary/10",
    },
  ];

  return (
    <Card
      aria-labelledby="subscription-status-title"
      as="section"
      className="h-[469px] border border-divider/60 bg-content1"
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
            <div className="grid grid-cols-3 overflow-hidden rounded-large bg-default-100/70">
              {[0, 1, 2].map((item) => (
                <div
                  key={item}
                  className="flex h-[88px] min-w-0 flex-col justify-between border-r border-divider/60 px-2.5 py-3 last:border-r-0 sm:px-3"
                >
                  <div className="flex items-center justify-between gap-2">
                    <Skeleton className="h-3 w-14 rounded-md" />
                    <Skeleton className="hidden size-6 shrink-0 rounded-md sm:block" />
                  </div>
                  <Skeleton className="h-6 w-10 rounded-md" />
                </div>
              ))}
            </div>
            <div className="mt-3 overflow-hidden rounded-large bg-default-100/55 px-3.5">
              {[0, 1, 2].map((item) => (
                <div
                  key={item}
                  className="flex h-[100px] flex-col justify-center gap-2 border-b border-divider/60 last:border-b-0"
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
          <div className="mt-3 flex min-h-0 flex-1 flex-col items-center justify-center rounded-large bg-default-100/55 px-5 text-center">
            <span className="flex size-11 items-center justify-center rounded-lg bg-danger/10 text-danger">
              <Icon icon="lucide:circle-alert" width={22} />
            </span>
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
            <div className="grid grid-cols-3 overflow-hidden rounded-large bg-default-100/70">
              {summaryItems.map((item) => (
                <div
                  key={item.key}
                  className="flex h-[88px] min-w-0 flex-col justify-between border-r border-divider/60 px-2.5 py-3 last:border-r-0 sm:px-3"
                >
                  <div className="flex min-w-0 items-center justify-between gap-2">
                    <p className="line-clamp-2 min-w-0 text-[10px] font-medium leading-3 text-default-500 sm:text-[11px] sm:leading-4">
                      {item.label}
                    </p>
                    <span
                      className={`hidden size-6 shrink-0 items-center justify-center rounded-md sm:flex ${item.iconBackground} ${item.color}`}
                    >
                      <Icon icon={item.icon} width={14} />
                    </span>
                  </div>
                  <p
                    className={`flex items-end text-2xl font-semibold leading-none tabular-nums ${item.color}`}
                  >
                    {item.value}
                    {item.total != null && (
                      <span className="ml-1.5 pb-px text-xs font-medium leading-none text-default-400">
                        / {item.total}
                      </span>
                    )}
                  </p>
                </div>
              ))}
            </div>

            {subscriptions.length === 0 ? (
              <div className="mt-3 flex min-h-0 flex-1 flex-col items-center justify-center rounded-large bg-default-100/55 px-4 text-center">
                <div className="flex size-11 items-center justify-center rounded-lg bg-primary/10 text-primary">
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
              <div className="scrollbar-hide mt-3 min-h-0 flex-1 overflow-y-auto">
                <div className="overflow-hidden rounded-large bg-default-100/55 px-3.5">
                  {sortedSubscriptions.map((subscription) => {
                    const status = getStatus(subscription);
                    const view = statusView(status);

                    return (
                      <article
                        key={subscription.id}
                        className="min-h-[100px] border-b border-divider/60 py-3 last:border-b-0"
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
              </div>
            )}
          </div>
        )}
      </CardBody>
    </Card>
  );
}
