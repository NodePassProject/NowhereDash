import { buildApiUrl } from "@/lib/utils";

export type SubscriptionCarrier = "tcp" | "udp";

export interface PortalOption {
  id: number;
  name: string;
  instanceId?: string;
  listenHost: string;
  listenPort: string | number;
  status: "running" | "stopped" | "error" | "offline";
  type: "portal";
}

export interface PortalSubscription {
  id: number;
  name: string;
  profileTitle: string;
  token: string;
  expiresAt: string | null;
  trafficLimit: number | null;
  trafficUsed: number;
  overLimit: boolean;
  preferences: SubscriptionPreferences;
  tunnelIds: number[];
  portalCount: number;
  subscriptionUrl: string;
  createdAt: string;
  updatedAt: string;
}

export interface SubscriptionPreferences {
  expandCarrierCombos: boolean;
  upCarrier: SubscriptionCarrier;
  downCarrier: SubscriptionCarrier;
  includeIpv6: boolean;
}

export interface SubscriptionPayload {
  name: string;
  profileTitle: string;
  expiresAt: string | null;
  trafficLimit: number | null;
  preferences: SubscriptionPreferences;
  tunnelIds: number[];
}

export interface SubscriptionPreview {
  available: boolean;
  unavailableReason: string;
  content: string;
  portalCount: number;
  trafficUsed: number;
  headers: {
    "profile-title": string;
    "subscription-userinfo": string;
    "cache-control": string;
    "x-content-type-options": string;
  };
}

export interface SubscriptionTokenResult {
  token: string;
  subscriptionUrl: string;
  updatedAt: string;
}

type ApiEnvelope<T> = T | { data: T };

const unwrap = <T>(body: ApiEnvelope<T>): T => {
  if (typeof body === "object" && body !== null && "data" in body) {
    return body.data;
  }

  return body as T;
};

const readError = async (response: Response, fallback: string) => {
  try {
    const body = await response.json();

    return body.error || body.message || fallback;
  } catch {
    return fallback;
  }
};

const requestJson = async <T>(
  path: string,
  fallback: string,
  init?: RequestInit,
): Promise<T> => {
  const response = await fetch(buildApiUrl(path), init);

  if (!response.ok) throw new Error(await readError(response, fallback));

  return unwrap((await response.json()) as ApiEnvelope<T>);
};

export const listSubscriptions = () =>
  requestJson<PortalSubscription[]>(
    "/api/subscriptions",
    "Failed to load subscriptions",
  );

export const getSubscription = (id: number) =>
  requestJson<PortalSubscription>(
    `/api/subscriptions/${id}`,
    "Failed to load subscription",
  );

export const createSubscription = (payload: SubscriptionPayload) =>
  requestJson<PortalSubscription>(
    "/api/subscriptions",
    "Failed to create subscription",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  );

export const updateSubscription = (id: number, payload: SubscriptionPayload) =>
  requestJson<PortalSubscription>(
    `/api/subscriptions/${id}`,
    "Failed to update subscription",
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    },
  );

export const deleteSubscription = async (id: number) => {
  const response = await fetch(buildApiUrl(`/api/subscriptions/${id}`), {
    method: "DELETE",
  });

  if (!response.ok)
    throw new Error(await readError(response, "Failed to delete subscription"));
};

export const rotateSubscriptionToken = (id: number) =>
  requestJson<SubscriptionTokenResult>(
    `/api/subscriptions/${id}/token/rotate`,
    "Failed to rotate subscription token",
    { method: "POST" },
  );

export const previewSubscription = (id: number) =>
  requestJson<SubscriptionPreview>(
    `/api/subscriptions/${id}/preview`,
    "Failed to preview subscription",
    { cache: "no-store" },
  );

export const resetSubscriptionTraffic = (id: number) =>
  requestJson<PortalSubscription>(
    `/api/subscriptions/${id}/traffic/reset`,
    "Failed to reset subscription traffic",
    { method: "POST" },
  );

export const listPortalOptions = async (): Promise<PortalOption[]> => {
  const loadPage = async (page: number) => {
    const response = await fetch(
      buildApiUrl(
        `/api/tunnels?page=${page}&page_size=200&sort_by=name&sort_order=asc`,
      ),
    );

    if (!response.ok)
      throw new Error(await readError(response, "Failed to load Tunnels"));

    return (await response.json()) as {
      data?: PortalOption[];
      total_pages?: number;
    };
  };

  const first = await loadPage(1);
  const totalPages = Math.max(1, first.total_pages ?? 1);
  const remaining =
    totalPages > 1
      ? await Promise.all(
          Array.from({ length: totalPages - 1 }, (_, index) =>
            loadPage(index + 2),
          ),
        )
      : [];

  return [first, ...remaining]
    .flatMap((page) => page.data ?? [])
    .filter((tunnel) => tunnel.type === "portal");
};

export const toSubscriptionPayload = (
  subscription: PortalSubscription,
  overrides?: Partial<SubscriptionPayload>,
): SubscriptionPayload => ({
  name: subscription.name,
  profileTitle: subscription.profileTitle,
  expiresAt: subscription.expiresAt,
  trafficLimit: subscription.trafficLimit,
  preferences: subscription.preferences,
  tunnelIds: subscription.tunnelIds,
  ...overrides,
});

export const absoluteSubscriptionUrl = (value: string) => {
  if (!value) return "";

  try {
    return new URL(value, window.location.origin).toString();
  } catch {
    return value;
  }
};

export const anywhereImportUrl = (value: string) => {
  const importUrl = absoluteSubscriptionUrl(value);

  // Keep the nested subscription or Vector URL intact inside Anywhere's query.
  return importUrl
    ? `anywhere://add-proxy?link=${encodeURIComponent(importUrl)}`
    : "";
};
