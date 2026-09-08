import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faCoins,
  faCalendarDays,
  faGaugeHigh,
} from "@fortawesome/free-solid-svg-icons";
import { useTranslation } from "react-i18next";
import { fromDate, toCalendarDate, today } from "@internationalized/date";

import {
  type EndpointBilling,
  formatEndpointBytes,
} from "@/lib/endpoint-billing";

export default function EndpointBillingSummary({
  endpoint,
}: {
  endpoint: EndpointBilling;
}) {
  const { t } = useTranslation("endpoints");
  const used = endpoint.trafficUsed || 0;
  const total = endpoint.trafficTotal || 0;
  const limit = endpoint.monthlyTrafficLimit;
  const amount =
    endpoint.amount ?? (endpoint.price == null ? "" : String(endpoint.price));
  const displayedAmount = /^0+(?:\.0+)?$/.test(amount.trim())
    ? t("billing.free")
    : amount;
  const resetDate =
    endpoint.autoResetTraffic && endpoint.nextTrafficResetAt
      ? new Date(endpoint.nextTrafficResetAt)
      : null;
  const resetTimezone = endpoint.trafficResetTimezone || "UTC";
  const remainingDays = resetDate
    ? Math.max(
        0,
        toCalendarDate(fromDate(resetDate, resetTimezone)).compare(
          today(resetTimezone),
        ),
      )
    : null;
  const usageLabel = limit ? t("billing.periodUsed") : t("billing.lifetime");
  const compactUsage = (fractionDigits: number) => {
    const options = { compact: true, fractionDigits };

    return limit
      ? `${formatEndpointBytes(used, options)}/${formatEndpointBytes(limit, options)}`
      : formatEndpointBytes(total, options);
  };

  return (
    <div className="@container min-w-0 text-xs text-default-500">
      <div className="flex min-w-0 items-center gap-1.5 whitespace-nowrap tabular-nums @[300px]:gap-2.5">
        {amount.trim() !== "" && (
          <span className="flex min-w-0 max-w-[25%] items-center gap-1">
            <FontAwesomeIcon
              className="shrink-0 text-base text-default-400"
              icon={faCoins}
            />
            <span className="sr-only">{t("billing.amount")}: </span>
            <span className="truncate whitespace-pre">{displayedAmount}</span>
          </span>
        )}
        <span className="flex min-w-0 items-center gap-1">
          <FontAwesomeIcon
            className="shrink-0 text-base text-default-400"
            icon={faGaugeHigh}
          />
          <span className="sr-only">{usageLabel}: </span>
          <span className="hidden truncate @[300px]:inline">
            {compactUsage(2)}
          </span>
          <span className="truncate @[300px]:hidden">{compactUsage(0)}</span>
        </span>
        {remainingDays !== null && (
          <span className="flex shrink-0 items-center gap-1">
            <span className="hidden @[260px]:inline-flex">
              <FontAwesomeIcon
                className="text-base text-default-400"
                icon={faCalendarDays}
              />
            </span>
            <span className="sr-only">{t("billing.resetDate")}: </span>
            {t("billing.remainingDays", { count: remainingDays })}
          </span>
        )}
      </div>
    </div>
  );
}
