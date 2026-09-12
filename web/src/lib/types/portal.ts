import type { PortalEndpointLike, PortalTunnelLike } from "@/lib/portal-url";

export interface PortalEndpoint extends PortalEndpointLike {
  id: string | number;
  name: string;
}

export interface PeerMetadata {
  sid?: string | null;
  type?: string | null;
  alias?: string | null;
}

export interface PortalTunnel extends PortalTunnelLike {
  id: string | number;
  instanceId?: string;
  type: "portal";
  name: string;
  endpointId: string | number;
  endpoint?: PortalEndpoint | string;
  endpointName?: string;
  status: "running" | "stopped" | "error" | "offline";
  listenHost: string;
  listenPort: string | number;
  restart?: boolean;
  enableLogStore?: boolean;
  enable_log_store?: boolean;
  tags?: Record<string, string> | null;
  peer?: PeerMetadata | null;
  alpn?: string;
  portalHost?: string;
  vectorUrl?: string;
  totalRx?: number;
  totalTx?: number;
  tcpRx?: number;
  tcpTx?: number;
  udpRx?: number;
  udpTx?: number;
  tcps?: number | null;
  udps?: number | null;
  pool?: number | null;
  ping?: number | null;
  sorts?: number;
  createdAt?: string;
  updatedAt?: string;
}
