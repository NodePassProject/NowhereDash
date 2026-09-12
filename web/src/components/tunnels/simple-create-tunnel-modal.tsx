import {
  Button,
  Divider,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Spinner,
  Switch,
  Tab,
  Tabs,
  Tooltip,
} from "@heroui/react";
import { Icon } from "@iconify/react/dist/offline";
import { addToast } from "@heroui/toast";
import { AnimatePresence, motion } from "framer-motion";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useTranslation } from "react-i18next";

import { buildApiUrl } from "@/lib/utils";
import {
  parseNextEndpoint,
  portalServiceEndpoint,
  validateServiceEndpoint,
  validateSocksEndpoint,
  validatePortalSNI,
} from "@/lib/portal-url";

interface EndpointSimple {
  id: string | number;
  name: string;
  hostname?: string;
  url?: string;
}

interface PeerMetadata {
  sid?: string | null;
  type?: string | null;
  alias?: string | null;
}

interface PortalForm {
  apiEndpoint: string;
  tunnelName: string;
  listenHost: string;
  tcpPort: string;
  udpPort: string;
  tcpFamily: string;
  udpFamily: string;
  morph: string;
  sharedKey: string;
  network: string;
  tlsMode: string;
  certPath: string;
  keyPath: string;
  rate: string;
  etar: string;
  dial: string;
  socks: string;
  next: string;
  up: string;
  down: string;
  mux: string;
  sni: string;
  pin: string;
  logLevel: string;
  restart: boolean;
  enableLogStore: boolean;
  tagsText: string;
  peer: PeerMetadata | null;
}

interface SimpleCreateTunnelModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved?: () => void;
  mode?: "create" | "edit";
  instanceId?: string;
}

const INITIAL_FORM: PortalForm = {
  apiEndpoint: "",
  tunnelName: "",
  listenHost: "",
  tcpPort: "",
  udpPort: "",
  tcpFamily: "any",
  udpFamily: "any",
  morph: "0",
  sharedKey: "",
  network: "mix",
  tlsMode: "1",
  certPath: "",
  keyPath: "",
  rate: "",
  etar: "",
  dial: "",
  socks: "",
  next: "",
  up: "",
  down: "",
  mux: "0",
  sni: "",
  pin: "",
  logLevel: "info",
  restart: true,
  enableLogStore: true,
  tagsText: "",
  peer: null,
};

const tagsToText = (tags?: Record<string, string> | null) =>
  Object.entries(tags ?? {})
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");

const textToTags = (value: string) => {
  const tags: Record<string, string> = {};

  value.split("\n").forEach((line) => {
    const trimmed = line.trim();

    if (!trimmed) return;
    const separator = trimmed.indexOf("=");

    if (separator < 1) throw new Error(`Invalid tag: ${trimmed}`);
    tags[trimmed.slice(0, separator).trim()] = trimmed
      .slice(separator + 1)
      .trim();
  });

  return tags;
};

const randomSharedKey = () => {
  const bytes = crypto.getRandomValues(new Uint8Array(18));

  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=/g, "");
};

const randomListenPort = () => {
  const [value] = crypto.getRandomValues(new Uint16Array(1));

  return String(1024 + (value % 64512));
};

interface FormFieldProps {
  children: ReactNode;
  label: ReactNode;
  className?: string;
  hint?: string;
  required?: boolean;
}

interface LabelWithHelpProps {
  label: string;
  help: string;
}

function LabelWithHelp({ label, help }: LabelWithHelpProps) {
  const [isHelpOpen, setIsHelpOpen] = useState(false);

  return (
    <span className="inline-flex items-center gap-1">
      {label}
      <Tooltip
        closeDelay={0}
        content={help}
        delay={0}
        isOpen={isHelpOpen}
        placement="top"
      >
        <button
          aria-label={help}
          className="inline-flex cursor-help text-default-400"
          type="button"
          onBlur={() => setIsHelpOpen(false)}
          onFocus={() => setIsHelpOpen(true)}
          onPointerEnter={() => setIsHelpOpen(true)}
          onPointerLeave={() => setIsHelpOpen(false)}
        >
          <Icon icon="lucide:circle-help" width={13} />
        </button>
      </Tooltip>
    </span>
  );
}

function FormField({
  children,
  label,
  className = "",
  hint,
  required = false,
}: FormFieldProps) {
  return (
    <div className={`min-w-0 space-y-1 ${className}`}>
      <div className="flex min-h-5 items-center gap-1 px-1 text-sm text-foreground-600">
        <span>{label}</span>
        {required && (
          <span aria-hidden="true" className="text-danger">
            *
          </span>
        )}
      </div>
      {children}
      {hint && (
        <p className="px-1 text-xs leading-4 text-default-400">{hint}</p>
      )}
    </div>
  );
}

export default function SimpleCreateTunnelModal({
  isOpen,
  onOpenChange,
  onSaved,
  mode = "create",
  instanceId,
}: SimpleCreateTunnelModalProps) {
  const { i18n } = useTranslation();
  const zh = i18n.language.startsWith("zh");
  const [endpoints, setEndpoints] = useState<EndpointSimple[]>([]);
  const [form, setForm] = useState<PortalForm>(INITIAL_FORM);
  const [realTunnelId, setRealTunnelId] = useState("");
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [showSharedKey, setShowSharedKey] = useState(false);

  const copy = useMemo(
    () =>
      zh
        ? {
            create: "创建隧道",
            edit: "编辑隧道",
            endpoint: "节点",
            name: "名称",
            namePlaceholder: "例如：新加坡入口",
            listenHost: "监听地址",
            listenHostHint: "留空表示同时监听 IPv4 与 IPv6 通配地址",
            listenPort: "监听端口",
            randomPort: "随机生成监听端口",
            sharedKey: "共享密钥",
            generate: "生成密钥",
            network: "监听载体",
            tcpPort: "TCP 端口",
            udpPort: "UDP 端口",
            family: "地址族",
            anyFamily: "IPv4 / IPv6",
            automatic: "自动",
            morph: "Morph",
            morphHint: "同一跳两端须使用相同 Morph 设置；也应用于原生下级隧道",
            tls: "TLS 模式",
            log: "日志级别",
            cert: "证书路径",
            key: "私钥路径",
            optional: "可选配置",
            dial: "出口地址",
            rate: "入口限速 (Mbps)",
            etar: "出口限速 (Mbps)",
            socks: "SOCKS 出口",
            socksHint: "与下级隧道互斥",
            next: "下级隧道",
            nextHint:
              "shared-key@host:port 或 shared-key@host/tcp:port/udp:port；与 SOCKS 出口互斥",
            up: "上行载体",
            down: "下行载体",
            mux: "TLS Mux",
            muxHint: "复用下级 Portal 的 TLS 连接；纯 UDP 模式下不可用",
            muxDisabled: "纯 UDP 模式会固定使用独立连接",
            disabled: "关闭",
            enabled: "启用",
            sni: "SNI",
            pin: "证书指纹 (SHA-256)",
            tlsMemory: "模式1：自签名证书",
            tlsFiles: "模式 2：自定义证书",
            showSharedKey: "显示共享密钥",
            hideSharedKey: "隐藏共享密钥",
            metadata: "Metadata 标签",
            metadataHint: "每行一个 key=value；OpenCtrl peer 信息会原样保留",
            cancel: "取消",
            save: "保存",
            saving: "保存中",
            required: "请填写节点、名称、监听端口和共享密钥",
            invalid: "隧道参数无效",
            success: mode === "edit" ? "隧道已更新" : "隧道已创建",
            failure: "保存隧道失败",
          }
        : {
            create: "Create Tunnel",
            edit: "Edit Tunnel",
            endpoint: "Node",
            name: "Name",
            namePlaceholder: "e.g. Singapore gateway",
            listenHost: "Listen host",
            listenHostHint:
              "Leave empty to bind IPv4 and IPv6 wildcard sockets",
            listenPort: "Listen port",
            randomPort: "Generate a random listen port",
            sharedKey: "Shared key",
            generate: "Generate key",
            network: "Listener carriers",
            tcpPort: "TCP port",
            udpPort: "UDP port",
            family: "Address family",
            anyFamily: "IPv4 / IPv6",
            automatic: "Auto",
            morph: "Morph",
            morphHint:
              "Both ends of each hop must use the same Morph setting; also applies to the native next hop",
            tls: "TLS mode",
            log: "Log level",
            cert: "Certificate path",
            key: "Private key path",
            optional: "Optional Configuration",
            dial: "Outbound address",
            rate: "Ingress rate (Mbps)",
            etar: "Egress rate (Mbps)",
            socks: "SOCKS egress",
            socksHint: "Mutually exclusive with Next Tunnel",
            next: "Next Tunnel",
            nextHint:
              "shared-key@host:port or shared-key@host/tcp:port/udp:port; mutually exclusive with SOCKS",
            up: "Up carrier",
            down: "Down carrier",
            mux: "TLS Mux",
            muxHint:
              "Reuse TLS connections to the next Portal; unavailable for UDP-only routing",
            muxDisabled: "UDP-only routing always uses dedicated connections",
            disabled: "Off",
            enabled: "On",
            sni: "SNI",
            pin: "Certificate pin (SHA-256)",
            tlsMemory: "Mode 1: Self-signed certificate",
            tlsFiles: "Mode 2: Custom certificate",
            showSharedKey: "Show shared key",
            hideSharedKey: "Hide shared key",
            metadata: "Metadata tags",
            metadataHint:
              "One key=value per line; OpenCtrl peer metadata is preserved",
            cancel: "Cancel",
            save: "Save",
            saving: "Saving",
            required: "Node, name, listen port, and shared key are required",
            invalid: "Invalid Tunnel configuration",
            success: mode === "edit" ? "Tunnel updated" : "Tunnel created",
            failure: "Failed to save Tunnel",
          },
    [mode, zh],
  );

  const update = useCallback(
    <K extends keyof PortalForm>(key: K, value: PortalForm[K]) => {
      setForm((current) => ({ ...current, [key]: value }));
    },
    [],
  );

  const updateCarrier = useCallback((key: "up" | "down", value: string) => {
    setForm((current) => {
      const next = { ...current, [key]: value };

      if (next.up === "udp" && next.down === "udp") next.mux = "0";

      return next;
    });
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    const controller = new AbortController();
    const { signal } = controller;

    const load = async () => {
      setLoading(true);
      setAdvancedOpen(false);
      setShowSharedKey(false);
      setForm(INITIAL_FORM);
      setRealTunnelId("");

      try {
        const endpointResponse = await fetch(
          buildApiUrl("/api/endpoints/simple?excludeFailed=true"),
          { signal },
        );

        if (!endpointResponse.ok) throw new Error("Failed to load nodes");
        const endpointData =
          (await endpointResponse.json()) as EndpointSimple[];

        if (signal.aborted) return;

        setEndpoints(endpointData);

        if (mode === "edit" && instanceId) {
          const response = await fetch(
            buildApiUrl(`/api/tunnels/${instanceId}/details`),
            { signal },
          );
          const body = await response.json();

          if (signal.aborted) return;

          if (!response.ok)
            throw new Error(body.error || "Failed to load Tunnel");
          const tunnel = body.tunnel ?? body;
          const listener = portalServiceEndpoint(tunnel);

          setRealTunnelId(String(tunnel.id ?? instanceId));
          setForm({
            apiEndpoint: String(tunnel.endpointId ?? body.endpoint?.id ?? ""),
            tunnelName: tunnel.name ?? "",
            listenHost: listener.host,
            tcpPort: listener.tcpPort,
            udpPort: listener.udpPort,
            tcpFamily: listener.tcpFamily,
            udpFamily: listener.udpFamily,
            morph: String(tunnel.morph ?? "0"),
            sharedKey: tunnel.sharedKey ?? "",
            network: !listener.tcpPort
              ? "udp"
              : !listener.udpPort
                ? "tcp"
                : "mix",
            tlsMode: String(tunnel.tlsMode ?? "1"),
            certPath: tunnel.certPath ?? "",
            keyPath: tunnel.keyPath ?? "",
            rate: String(tunnel.rate ?? 0),
            etar: String(tunnel.etar ?? 0),
            dial: tunnel.dial ?? "auto",
            socks: tunnel.socks ?? "none",
            next:
              tunnel.next && tunnel.next.toLowerCase() !== "none"
                ? tunnel.next
                : "",
            up: tunnel.up ?? "",
            down: tunnel.down ?? "",
            mux: String(tunnel.mux ?? "0"),
            sni: tunnel.sni ?? "none",
            pin:
              tunnel.pin && tunnel.pin.toLowerCase() !== "none"
                ? tunnel.pin
                : "",
            logLevel: tunnel.logLevel ?? "info",
            restart: tunnel.restart ?? true,
            enableLogStore:
              tunnel.enableLogStore ?? tunnel.enable_log_store ?? true,
            tagsText: tagsToText(tunnel.tags),
            peer: tunnel.peer ?? null,
          });
        } else {
          const port = randomListenPort();

          setForm({
            ...INITIAL_FORM,
            apiEndpoint: endpointData.length ? String(endpointData[0].id) : "",
            tcpPort: port,
            udpPort: port,
            sharedKey: randomSharedKey(),
          });
        }
      } catch (error) {
        if (signal.aborted) return;
        addToast({
          title: copy.failure,
          description: error instanceof Error ? error.message : copy.failure,
          color: "danger",
        });
      } finally {
        if (!signal.aborted) setLoading(false);
      }
    };

    void load();

    return () => controller.abort();
  }, [copy.failure, instanceId, isOpen, mode]);

  const validate = () => {
    if (
      !form.apiEndpoint ||
      !form.tunnelName.trim() ||
      (form.network !== "udp" && !form.tcpPort) ||
      (form.network !== "tcp" && !form.udpPort) ||
      !form.sharedKey
    ) {
      throw new Error(copy.required);
    }

    validateServiceEndpoint({
      host: form.listenHost.trim() || "*",
      tcpPort: form.network === "udp" ? "" : form.tcpPort,
      udpPort: form.network === "tcp" ? "" : form.udpPort,
      tcpFamily: form.tcpFamily,
      udpFamily: form.udpFamily,
    });
    if (new TextEncoder().encode(form.sharedKey).length > 255)
      throw new Error(copy.invalid);
    if (form.tlsMode === "2" && (!form.certPath.trim() || !form.keyPath.trim()))
      throw new Error(copy.invalid);

    const rate = Number(form.rate || 0);
    const etar = Number(form.etar || 0);

    if (
      ![rate, etar].every(
        (value) => Number.isInteger(value) && value >= 0 && value <= 2147483647,
      )
    )
      throw new Error(copy.invalid);
    if (form.mux !== "0" && form.mux !== "1") throw new Error(copy.invalid);
    const socksConfigured =
      form.socks.trim() !== "" && form.socks.trim().toLowerCase() !== "none";
    const nextConfigured =
      form.next.trim() !== "" && form.next.trim().toLowerCase() !== "none";

    if (socksConfigured && nextConfigured) throw new Error(copy.invalid);
    if (socksConfigured) validateSocksEndpoint(form.socks.trim());
    if (nextConfigured) {
      const endpoint = parseNextEndpoint(form.next.trim());

      validatePortalSNI(form.sni.trim());

      for (const carrier of [form.up, form.down]) {
        if (
          carrier &&
          ((carrier !== "udp" && !endpoint.tcpPort) ||
            (carrier !== "tcp" && !endpoint.udpPort))
        )
          throw new Error(copy.invalid);
      }
    }
    if (
      nextConfigured &&
      form.pin.trim() !== "" &&
      form.pin.trim().toLowerCase() !== "none" &&
      !/^[a-f0-9]{64}$/.test(form.pin.trim())
    )
      throw new Error(copy.invalid);
  };

  const submit = async () => {
    if (loading || submitting || (mode === "edit" && !realTunnelId)) return;
    try {
      validate();
      const tags = textToTags(form.tagsText);

      setSubmitting(true);

      const response = await fetch(
        buildApiUrl(
          mode === "edit"
            ? `/api/tunnels/${realTunnelId || instanceId}`
            : "/api/tunnels",
        ),
        {
          method: mode === "edit" ? "PUT" : "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            endpointId: Number(form.apiEndpoint),
            name: form.tunnelName.trim(),
            listenHost: form.listenHost.trim(),
            tcpPort: form.network === "udp" ? "" : form.tcpPort,
            udpPort: form.network === "tcp" ? "" : form.udpPort,
            tcpFamily: form.tcpFamily,
            udpFamily: form.udpFamily,
            morph: form.morph,
            sharedKey: form.sharedKey,
            tlsMode: form.tlsMode,
            certPath: form.tlsMode === "2" ? form.certPath.trim() : "",
            keyPath: form.tlsMode === "2" ? form.keyPath.trim() : "",
            rate: form.rate !== "" ? Number(form.rate) : undefined,
            etar: form.etar !== "" ? Number(form.etar) : undefined,
            dial: form.dial.trim() || undefined,
            socks: form.socks.trim() || undefined,
            next: form.next.trim() || "none",
            up: form.up,
            down: form.down,
            mux: udpOnlyNext ? "0" : form.mux,
            sni: form.sni.trim() || undefined,
            pin: form.pin.trim() || "none",
            logLevel: form.logLevel,
            restart: form.restart,
            enableLogStore: form.enableLogStore,
            tags,
            peer: form.peer,
          }),
        },
      );
      const body = await response.json();

      if (!response.ok || body.success === false)
        throw new Error(body.error || body.message || copy.failure);

      addToast({ title: copy.success, color: "success" });
      onOpenChange(false);
      onSaved?.();
    } catch (error) {
      addToast({
        title: copy.failure,
        description: error instanceof Error ? error.message : copy.failure,
        color: "danger",
      });
    } finally {
      setSubmitting(false);
    }
  };

  const nextEnabled =
    form.next.trim() !== "" && form.next.trim().toLowerCase() !== "none";
  let nextEndpoint = null;

  try {
    if (nextEnabled) nextEndpoint = parseNextEndpoint(form.next.trim());
  } catch {
    /* Validated on submit. */
  }
  const defaultCarrier = nextEndpoint && !nextEndpoint.tcpPort ? "udp" : "tcp";
  const udpOnlyNext =
    (form.up || defaultCarrier) === "udp" &&
    (form.down || defaultCarrier) === "udp";
  const unavailableCarriers = nextEndpoint
    ? new Set(
        [
          !nextEndpoint.tcpPort ? "tcp" : "",
          !nextEndpoint.udpPort ? "udp" : "",
          !nextEndpoint.tcpPort || !nextEndpoint.udpPort ? "mix" : "",
        ].filter(Boolean),
      )
    : new Set<string>();

  return (
    <Modal
      isOpen={isOpen}
      placement="center"
      scrollBehavior="inside"
      size="xl"
      onOpenChange={(open) => {
        if (!submitting) onOpenChange(open);
      }}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2 pb-0">
              <Icon
                className="shrink-0 text-primary"
                icon="lucide:radio-tower"
                width={19}
              />
              <span className="text-base font-semibold">
                {mode === "edit" ? copy.edit : copy.create}
              </span>
            </ModalHeader>
            <ModalBody className="space-y-3 py-4">
              {loading ? (
                <div className="flex min-h-48 items-center justify-center">
                  <Spinner />
                </div>
              ) : (
                <>
                  <section className="grid grid-cols-1 gap-x-3 gap-y-2 sm:grid-cols-2">
                    <FormField required label={copy.endpoint}>
                      <Select
                        isRequired
                        aria-label={copy.endpoint}
                        isDisabled={mode === "edit"}
                        selectedKeys={
                          form.apiEndpoint
                            ? new Set([form.apiEndpoint])
                            : new Set()
                        }
                        onSelectionChange={(keys) =>
                          update(
                            "apiEndpoint",
                            String(Array.from(keys)[0] ?? ""),
                          )
                        }
                      >
                        {endpoints.map((endpoint) => (
                          <SelectItem
                            key={String(endpoint.id)}
                            textValue={endpoint.name}
                          >
                            <div className="flex min-w-0 flex-col">
                              <span className="truncate">{endpoint.name}</span>
                              {(endpoint.hostname || endpoint.url) && (
                                <span className="truncate text-xs text-default-400">
                                  {endpoint.hostname || endpoint.url}
                                </span>
                              )}
                            </div>
                          </SelectItem>
                        ))}
                      </Select>
                    </FormField>

                    <FormField required label={copy.name}>
                      <Input
                        isRequired
                        aria-label={copy.name}
                        placeholder={copy.namePlaceholder}
                        value={form.tunnelName}
                        onValueChange={(value) => update("tunnelName", value)}
                      />
                    </FormField>

                    <FormField
                      label={
                        <LabelWithHelp
                          help={copy.listenHostHint}
                          label={copy.listenHost}
                        />
                      }
                    >
                      <Input
                        aria-label={copy.listenHost}
                        placeholder="*"
                        value={form.listenHost}
                        onValueChange={(value) => update("listenHost", value)}
                      />
                    </FormField>

                    <FormField label={copy.network}>
                      <Tabs
                        fullWidth
                        aria-label={copy.network}
                        classNames={{ tabList: "h-10", tab: "h-8" }}
                        color="secondary"
                        selectedKey={form.network}
                        size="sm"
                        onSelectionChange={(key) =>
                          update("network", String(key))
                        }
                      >
                        <Tab key="mix" title="TCP + UDP" />
                        <Tab key="tcp" title="TCP" />
                        <Tab key="udp" title="UDP" />
                      </Tabs>
                    </FormField>

                    <FormField
                      required
                      className="sm:col-span-2"
                      label={copy.sharedKey}
                    >
                      <Input
                        isRequired
                        aria-label={copy.sharedKey}
                        endContent={
                          <div className="flex shrink-0 items-center gap-1">
                            <Tooltip
                              content={
                                showSharedKey
                                  ? copy.hideSharedKey
                                  : copy.showSharedKey
                              }
                            >
                              <Button
                                isIconOnly
                                aria-label={
                                  showSharedKey
                                    ? copy.hideSharedKey
                                    : copy.showSharedKey
                                }
                                size="sm"
                                variant="light"
                                onPress={() =>
                                  setShowSharedKey((value) => !value)
                                }
                              >
                                <Icon
                                  icon={
                                    showSharedKey
                                      ? "lucide:eye-off"
                                      : "lucide:eye"
                                  }
                                  width={17}
                                />
                              </Button>
                            </Tooltip>
                            <Tooltip content={copy.generate}>
                              <Button
                                isIconOnly
                                aria-label={copy.generate}
                                size="sm"
                                variant="light"
                                onPress={() =>
                                  update("sharedKey", randomSharedKey())
                                }
                              >
                                <Icon icon="lucide:dices" width={17} />
                              </Button>
                            </Tooltip>
                          </div>
                        }
                        type={showSharedKey ? "text" : "password"}
                        value={form.sharedKey}
                        onValueChange={(value) => update("sharedKey", value)}
                      />
                    </FormField>

                    {(["tcp", "udp"] as const).map((carrier) => {
                      const disabled =
                        form.network !== "mix" && form.network !== carrier;
                      const portKey = `${carrier}Port` as const;
                      const familyKey = `${carrier}Family` as const;
                      const familyLabel = `${carrier.toUpperCase()} ${copy.family}`;

                      return (
                        <div key={carrier} className="min-w-0 space-y-2">
                          <FormField label={copy[portKey]} required={!disabled}>
                            <Input
                              aria-label={copy[portKey]}
                              endContent={
                                <Tooltip content={copy.randomPort}>
                                  <Button
                                    isIconOnly
                                    aria-label={`${carrier.toUpperCase()} ${copy.randomPort}`}
                                    isDisabled={disabled}
                                    size="sm"
                                    variant="light"
                                    onPress={() =>
                                      update(portKey, randomListenPort())
                                    }
                                  >
                                    <Icon icon="lucide:dices" width={17} />
                                  </Button>
                                </Tooltip>
                              }
                              isDisabled={disabled}
                              isRequired={!disabled}
                              max={65535}
                              min={1}
                              type="number"
                              value={form[portKey]}
                              onValueChange={(value) => update(portKey, value)}
                            />
                          </FormField>
                          <Select
                            aria-label={familyLabel}
                            isDisabled={disabled}
                            label={familyLabel}
                            selectedKeys={new Set([form[familyKey]])}
                            onSelectionChange={(keys) =>
                              update(
                                familyKey,
                                String(Array.from(keys)[0] ?? "any"),
                              )
                            }
                          >
                            <SelectItem key="any">{copy.anyFamily}</SelectItem>
                            <SelectItem key="4">IPv4</SelectItem>
                            <SelectItem key="6">IPv6</SelectItem>
                          </Select>
                        </div>
                      );
                    })}

                    <FormField label={copy.tls}>
                      <Select
                        aria-label={copy.tls}
                        selectedKeys={new Set([form.tlsMode])}
                        onSelectionChange={(keys) =>
                          update("tlsMode", String(Array.from(keys)[0] ?? "1"))
                        }
                      >
                        <SelectItem key="1">{copy.tlsMemory}</SelectItem>
                        <SelectItem key="2">{copy.tlsFiles}</SelectItem>
                      </Select>
                    </FormField>

                    <FormField
                      label={
                        <LabelWithHelp
                          help={copy.morphHint}
                          label={copy.morph}
                        />
                      }
                    >
                      <div className="flex h-10 items-center px-1">
                        <Switch
                          aria-label={copy.morph}
                          isSelected={form.morph === "1"}
                          onValueChange={(enabled) =>
                            update("morph", enabled ? "1" : "0")
                          }
                        >
                          {form.morph === "1" ? copy.enabled : copy.disabled}
                        </Switch>
                      </div>
                    </FormField>

                    {form.tlsMode === "2" && (
                      <>
                        <FormField required label={copy.cert}>
                          <Input
                            isRequired
                            aria-label={copy.cert}
                            placeholder="/etc/nowhere/cert.pem"
                            value={form.certPath}
                            onValueChange={(value) => update("certPath", value)}
                          />
                        </FormField>
                        <FormField required label={copy.key}>
                          <Input
                            isRequired
                            aria-label={copy.key}
                            placeholder="/etc/nowhere/key.pem"
                            value={form.keyPath}
                            onValueChange={(value) => update("keyPath", value)}
                          />
                        </FormField>
                      </>
                    )}

                    <FormField label={copy.log}>
                      <Select
                        aria-label={copy.log}
                        selectedKeys={new Set([form.logLevel])}
                        onSelectionChange={(keys) =>
                          update(
                            "logLevel",
                            String(Array.from(keys)[0] ?? "info"),
                          )
                        }
                      >
                        {[
                          "none",
                          "debug",
                          "info",
                          "warn",
                          "error",
                          "event",
                        ].map((level) => (
                          <SelectItem key={level}>{level}</SelectItem>
                        ))}
                      </Select>
                    </FormField>

                    <FormField label={copy.dial}>
                      <Input
                        aria-label={copy.dial}
                        placeholder="auto"
                        value={form.dial}
                        onValueChange={(value) => update("dial", value)}
                      />
                    </FormField>
                  </section>

                  <div className="relative">
                    <Divider />
                    <div className="absolute left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 bg-background px-4 dark:bg-[#18181B]">
                      <button
                        aria-controls="portal-optional-configuration"
                        aria-expanded={advancedOpen}
                        className="flex items-center gap-2 whitespace-nowrap text-sm text-default-600 transition-colors hover:text-default-800"
                        type="button"
                        onClick={() => setAdvancedOpen((value) => !value)}
                      >
                        {copy.optional}
                        <Icon
                          className={`transition-transform duration-200 ${
                            advancedOpen ? "" : "rotate-180"
                          }`}
                          icon="lucide:chevron-down"
                          width={13}
                        />
                      </button>
                    </div>
                  </div>

                  <AnimatePresence initial={false}>
                    {advancedOpen && (
                      <motion.div
                        animate={{ height: "auto", opacity: 1 }}
                        className="shrink-0 overflow-hidden"
                        exit={{ height: 0, opacity: 0 }}
                        id="portal-optional-configuration"
                        initial={{ height: 0, opacity: 0 }}
                        transition={{
                          duration: 0.3,
                          ease: "easeInOut",
                          height: { duration: 0.3, ease: "easeInOut" },
                        }}
                      >
                        <section className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                          <Input
                            aria-label={copy.rate}
                            label={copy.rate}
                            min={0}
                            placeholder="0"
                            type="number"
                            value={form.rate}
                            onValueChange={(value) => update("rate", value)}
                          />
                          <Input
                            aria-label={copy.etar}
                            label={copy.etar}
                            min={0}
                            placeholder="0"
                            type="number"
                            value={form.etar}
                            onValueChange={(value) => update("etar", value)}
                          />
                        </section>

                        <section className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                          <Input
                            aria-label={copy.socks}
                            label={copy.socks}
                            placeholder="none"
                            value={form.socks}
                            onValueChange={(value) =>
                              setForm((current) => ({
                                ...current,
                                socks: value,
                                next:
                                  value.trim() &&
                                  value.trim().toLowerCase() !== "none"
                                    ? ""
                                    : current.next,
                              }))
                            }
                          />
                          <Input
                            aria-label={copy.next}
                            classNames={{ label: "!pointer-events-auto" }}
                            label={
                              <LabelWithHelp
                                help={copy.nextHint}
                                label={copy.next}
                              />
                            }
                            placeholder="key@host/tcp:2006/udp:2017"
                            value={form.next}
                            onValueChange={(value) =>
                              setForm((current) => ({
                                ...current,
                                next: value,
                                socks:
                                  value.trim() &&
                                  value.trim().toLowerCase() !== "none"
                                    ? ""
                                    : current.socks,
                              }))
                            }
                          />
                        </section>

                        {nextEnabled && (
                          <>
                            <section className="mt-2 grid grid-cols-1 items-start gap-2 sm:grid-cols-3">
                              <Select
                                aria-label={copy.mux}
                                classNames={{ label: "!pointer-events-auto" }}
                                disabledKeys={
                                  udpOnlyNext ? new Set(["1"]) : new Set()
                                }
                                label={
                                  <LabelWithHelp
                                    help={
                                      udpOnlyNext
                                        ? copy.muxDisabled
                                        : copy.muxHint
                                    }
                                    label={copy.mux}
                                  />
                                }
                                labelPlacement="inside"
                                selectedKeys={
                                  new Set([udpOnlyNext ? "0" : form.mux])
                                }
                                onSelectionChange={(keys) =>
                                  update(
                                    "mux",
                                    String(Array.from(keys)[0] ?? "0"),
                                  )
                                }
                              >
                                <SelectItem key="0">{copy.disabled}</SelectItem>
                                <SelectItem key="1">{copy.enabled}</SelectItem>
                              </Select>
                              <Select
                                aria-label={copy.up}
                                disabledKeys={unavailableCarriers}
                                label={copy.up}
                                labelPlacement="inside"
                                selectedKeys={new Set([form.up || "auto"])}
                                onSelectionChange={(keys) =>
                                  updateCarrier(
                                    "up",
                                    String(
                                      Array.from(keys)[0] ?? "auto",
                                    ).replace(/^auto$/, ""),
                                  )
                                }
                              >
                                <SelectItem key="auto">
                                  {copy.automatic}
                                </SelectItem>
                                <SelectItem key="mix">mix</SelectItem>
                                <SelectItem key="tcp">tcp</SelectItem>
                                <SelectItem key="udp">udp</SelectItem>
                              </Select>
                              <Select
                                aria-label={copy.down}
                                disabledKeys={unavailableCarriers}
                                label={copy.down}
                                labelPlacement="inside"
                                selectedKeys={new Set([form.down || "auto"])}
                                onSelectionChange={(keys) =>
                                  updateCarrier(
                                    "down",
                                    String(
                                      Array.from(keys)[0] ?? "auto",
                                    ).replace(/^auto$/, ""),
                                  )
                                }
                              >
                                <SelectItem key="auto">
                                  {copy.automatic}
                                </SelectItem>
                                <SelectItem key="mix">mix</SelectItem>
                                <SelectItem key="tcp">tcp</SelectItem>
                                <SelectItem key="udp">udp</SelectItem>
                              </Select>
                            </section>

                            <section className="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-3">
                              <Input
                                aria-label={copy.sni}
                                label={copy.sni}
                                placeholder="none"
                                value={form.sni}
                                onValueChange={(value) => update("sni", value)}
                              />
                              <Input
                                aria-label={copy.pin}
                                className="sm:col-span-2"
                                label={copy.pin}
                                placeholder="none"
                                value={form.pin}
                                onValueChange={(value) => update("pin", value)}
                              />
                            </section>
                          </>
                        )}
                      </motion.div>
                    )}
                  </AnimatePresence>
                </>
              )}
            </ModalBody>
            <ModalFooter className="pt-0">
              <Button isDisabled={submitting} variant="light" onPress={onClose}>
                {copy.cancel}
              </Button>
              <Button
                color="primary"
                isDisabled={loading}
                isLoading={submitting}
                startContent={
                  !submitting && <Icon icon="lucide:save" width={17} />
                }
                onPress={submit}
              >
                {submitting ? copy.saving : copy.save}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
