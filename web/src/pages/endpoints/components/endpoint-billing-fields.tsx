import type { ComponentProps } from "react";
import type { EndpointBillingForm } from "@/lib/endpoint-billing";

import { DatePicker, Input, Switch } from "@heroui/react";
import { parseDate } from "@internationalized/date";
import { useTranslation } from "react-i18next";

export default function EndpointBillingFields({
  value,
  onChange,
}: {
  value: EndpointBillingForm;
  onChange: (value: EndpointBillingForm) => void;
}) {
  const { t } = useTranslation("endpoints");
  const hasTrafficLimit = value.monthlyTrafficGiB.trim() !== "";
  const set = (key: keyof EndpointBillingForm, next: string | boolean) =>
    onChange({ ...value, [key]: next });
  let resetDate: ComponentProps<typeof DatePicker>["value"] = null;

  try {
    if (value.trafficResetDate) {
      resetDate = parseDate(
        value.trafficResetDate,
      ) as unknown as typeof resetDate;
    }
  } catch {
    resetDate = null;
  }

  return (
    <div className="space-y-3">
      <h3 className="text-sm font-medium">{t("billing.title")}</h3>
      <div className="grid min-w-0 grid-cols-2 gap-3">
        <Input
          label={t("billing.amount")}
          maxLength={200}
          value={value.amount}
          onValueChange={(next) => set("amount", next)}
        />
        <Input
          endContent={
            <span className="shrink-0 text-sm text-default-400">GiB</span>
          }
          label={t("billing.monthlyLimit")}
          min={0}
          step="any"
          type="number"
          value={value.monthlyTrafficGiB}
          onValueChange={(next) =>
            onChange({
              ...value,
              monthlyTrafficGiB: next,
              autoResetTraffic: next.trim() !== "" && value.autoResetTraffic,
            })
          }
        />
        {hasTrafficLimit && (
          <>
            <DatePicker
              showMonthAndYearPickers
              aria-label={t("billing.resetDate")}
              className="min-w-0"
              classNames={{
                inputWrapper: "max-[360px]:gap-1 max-[360px]:px-2",
                segment: "max-[360px]:px-0",
                selectorButton: "max-[360px]:min-w-6 max-[360px]:w-6",
              }}
              granularity="day"
              label={t("billing.resetDate")}
              value={resetDate}
              onChange={(next) =>
                onChange({
                  ...value,
                  trafficResetDate: next?.toString() || "",
                  trafficResetDay: String(next?.day || 1),
                  trafficResetTime: "00:00",
                  trafficResetTimezone:
                    Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
                })
              }
            />
            <Switch
              classNames={{
                base: "m-0 min-h-14 w-full max-w-none flex-row-reverse justify-between gap-3 rounded-medium bg-default-100 px-3 py-2 shadow-xs transition-colors data-[hover=true]:bg-default-200",
                wrapper: "m-0 shrink-0",
                label: "m-0 min-w-0 text-sm text-default-600",
              }}
              isSelected={value.autoResetTraffic}
              size="sm"
              onValueChange={(next) => set("autoResetTraffic", next)}
            >
              {t("billing.autoReset")}
            </Switch>
          </>
        )}
      </div>
    </div>
  );
}
