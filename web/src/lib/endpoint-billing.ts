export interface EndpointBilling {
  amount?: string | null;
  price?: number | null;
  monthlyTrafficLimit?: number | null;
  trafficUsed?: number;
  trafficTotal?: number;
  trafficResetDay?: number;
  trafficResetTime?: string;
  trafficResetDate?: string;
  trafficResetTimezone?: string;
  autoResetTraffic?: boolean;
  nextTrafficResetAt?: string | null;
}

export interface EndpointBillingForm {
  trafficResetDate: string;
  amount: string;
  monthlyTrafficGiB: string;
  trafficResetDay: string;
  trafficResetTime: string;
  trafficResetTimezone: string;
  autoResetTraffic: boolean;
}

export const billingForm = (
  endpoint: EndpointBilling = {},
): EndpointBillingForm => ({
  trafficResetDate: endpoint.nextTrafficResetAt
    ? new Intl.DateTimeFormat("en-CA", {
        timeZone: endpoint.trafficResetTimezone || "UTC",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(new Date(endpoint.nextTrafficResetAt))
    : endpoint.trafficResetDate ||
      new Intl.DateTimeFormat("en-CA", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
      }).format(new Date()),
  amount:
    endpoint.amount ?? (endpoint.price == null ? "" : String(endpoint.price)),
  monthlyTrafficGiB:
    endpoint.monthlyTrafficLimit == null
      ? ""
      : String(endpoint.monthlyTrafficLimit / 1024 ** 3),
  trafficResetDay: String(
    endpoint.trafficResetDate || endpoint.nextTrafficResetAt
      ? endpoint.trafficResetDay || 1
      : new Date().getDate(),
  ),
  trafficResetTime: endpoint.trafficResetTime || "00:00",
  trafficResetTimezone:
    ((endpoint.trafficResetDate || endpoint.nextTrafficResetAt) &&
      endpoint.trafficResetTimezone) ||
    Intl.DateTimeFormat().resolvedOptions().timeZone ||
    "UTC",
  autoResetTraffic: endpoint.autoResetTraffic || false,
});

export const billingPayload = (form: EndpointBillingForm) => {
  const hasTrafficLimit = form.monthlyTrafficGiB.trim() !== "";

  return {
    trafficResetDate: hasTrafficLimit ? form.trafficResetDate : "",
    amount: form.amount,
    monthlyTrafficLimit: hasTrafficLimit
      ? Math.round(Number(form.monthlyTrafficGiB) * 1024 ** 3)
      : null,
    trafficResetDay: Number(form.trafficResetDay),
    trafficResetTime: form.trafficResetTime,
    trafficResetTimezone: form.trafficResetTimezone.trim(),
    autoResetTraffic: hasTrafficLimit && form.autoResetTraffic,
  };
};

export function billingError(form: EndpointBillingForm): string | null {
  const plan = billingPayload(form);

  if (
    plan.monthlyTrafficLimit !== null &&
    (!/^\d{4}-\d{2}-\d{2}$/.test(form.trafficResetDate) ||
      !Number.isFinite(new Date(form.trafficResetDate).getTime()))
  )
    return "billing.invalidSchedule";

  if (plan.amount.length > 200) return "billing.invalidAmount";
  if (
    (plan.monthlyTrafficLimit !== null &&
      (!Number.isSafeInteger(plan.monthlyTrafficLimit) ||
        plan.monthlyTrafficLimit <= 0)) ||
    (plan.autoResetTraffic && plan.monthlyTrafficLimit === null)
  )
    return "billing.invalidLimit";
  if (
    !Number.isInteger(plan.trafficResetDay) ||
    plan.trafficResetDay < 1 ||
    plan.trafficResetDay > 31 ||
    !/^([01]\d|2[0-3]):[0-5]\d$/.test(plan.trafficResetTime)
  )
    return "billing.invalidSchedule";
  try {
    new Intl.DateTimeFormat("en", { timeZone: plan.trafficResetTimezone });
  } catch {
    return "billing.invalidTimezone";
  }

  return null;
}

export function formatEndpointBytes(
  bytes: number,
  { compact = false, fractionDigits = 2 } = {},
): string {
  if (!bytes) return compact ? "0B" : "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"];
  const index = Math.min(
    Math.floor(Math.log(Math.max(1, bytes)) / Math.log(1024)),
    units.length - 1,
  );

  const value = (bytes / 1024 ** index).toFixed(index ? fractionDigits : 0);

  return `${compact ? Number(value) : value}${compact ? "" : " "}${units[index]}`;
}
