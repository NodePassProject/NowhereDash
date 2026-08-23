// 端点状态枚举
export const EndpointStatus = {
  ONLINE: "ONLINE",
  OFFLINE: "OFFLINE",
  FAIL: "FAIL",
  DISCONNECT: "DISCONNECT",
} as const;

export type EndpointStatusType =
  (typeof EndpointStatus)[keyof typeof EndpointStatus];

export interface TrafficStats {
  timestamp: number;
  tcp_in: number;
  tcp_out: number;
  udp_in: number;
  udp_out: number;
}

export interface TrafficHistory {
  timestamps: number[];
  tcp_in_rates: number[];
  tcp_out_rates: number[];
  udp_in_rates: number[];
  udp_out_rates: number[];
}

export interface Instance {
  id?: string;
  [key: string]: unknown;
}

// 接口定义
export interface Endpoint {
  id: string;
  url: string;
  ip: string;
  apiPath: string;
  apiKey: string;
  status: EndpointStatusType;
  lastCheck: Date;
  createdAt: Date;
  updatedAt: Date;
  tunnelCount: number;
}
