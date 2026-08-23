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
  listenPort: string;
  sharedKey: string;
  network: string;
  tlsMode: string;
  certPath: string;
  keyPath: string;
  alpn: string;
  rate: string;
  etar: string;
  dial: string;
  socks: string;
  next: string;
  up: string;
  down: string;
  poolSize: string;
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
  listenPort: "2077",
  sharedKey: "",
  network: "mix",
  tlsMode: "1",
  certPath: "",
  keyPath: "",
  alpn: "",
  rate: "",
  etar: "",
  dial: "",
  socks: "",
  next: "none",
  up: "udp",
  down: "udp",
  poolSize: "0",
  sni: "",
  pin: "none",
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
  label: string;
  className?: string;
  hint?: string;
  required?: boolean;
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
            network: "传输模式",
            tls: "TLS 模式",
            log: "日志级别",
            cert: "证书路径",
            key: "私钥路径",
            optional: "可选配置",
            alpn: "ALPN",
            dial: "出口地址",
            rate: "入口限速 (Mbps)",
            etar: "出口限速 (Mbps)",
            socks: "SOCKS 出口",
            socksHint: "与下级隧道互斥",
            next: "下级隧道",
            nextHint: "格式：shared-key@host:port；与 SOCKS 出口互斥",
            up: "上行载体",
            down: "下行载体",
            poolSize: "TLS 连接池",
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
            network: "Transport mode",
            tls: "TLS mode",
            log: "Log level",
            cert: "Certificate path",
            key: "Private key path",
            optional: "Optional Configuration",
            alpn: "ALPN",
            dial: "Outbound address",
            rate: "Ingress rate (Mbps)",
            etar: "Egress rate (Mbps)",
            socks: "SOCKS egress",
            socksHint: "Mutually exclusive with Next Tunnel",
            next: "Next Tunnel",
            nextHint:
              "Format: shared-key@host:port; mutually exclusive with SOCKS",
            up: "Up carrier",
            down: "Down carrier",
            poolSize: "TLS pool",
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

  useEffect(() => {
    if (!isOpen) return;

    const load = async () => {
      setLoading(true);
      setAdvancedOpen(false);
      setShowSharedKey(false);
      setForm(INITIAL_FORM);
      setRealTunnelId("");

      try {
        const endpointResponse = await fetch(
          buildApiUrl("/api/endpoints/simple?excludeFailed=true"),
        );

        if (!endpointResponse.ok) throw new Error("Failed to load nodes");
        const endpointData =
          (await endpointResponse.json()) as EndpointSimple[];

        setEndpoints(endpointData);

        if (mode === "edit" && instanceId) {
          const response = await fetch(
            buildApiUrl(`/api/tunnels/${instanceId}/details`),
          );
          const body = await response.json();

          if (!response.ok)
            throw new Error(body.error || "Failed to load Tunnel");
          const tunnel = body.tunnel ?? body;

          setRealTunnelId(String(tunnel.id ?? instanceId));
          setForm({
            apiEndpoint: String(tunnel.endpointId ?? body.endpoint?.id ?? ""),
            tunnelName: tunnel.name ?? "",
            listenHost: tunnel.listenHost ?? "",
            listenPort: String(tunnel.listenPort ?? ""),
            sharedKey: tunnel.sharedKey ?? "",
            network: tunnel.network ?? "mix",
            tlsMode: String(tunnel.tlsMode ?? "1"),
            certPath: tunnel.certPath ?? "",
            keyPath: tunnel.keyPath ?? "",
            alpn: tunnel.alpn ?? "now/1",
            rate: String(tunnel.rate ?? 0),
            etar: String(tunnel.etar ?? 0),
            dial: tunnel.dial ?? "auto",
            socks: tunnel.socks ?? "none",
            next: tunnel.next ?? "none",
            up: tunnel.up ?? "udp",
            down: tunnel.down ?? "udp",
            poolSize: String(tunnel.poolSize ?? 0),
            sni: tunnel.sni ?? "none",
            pin: tunnel.pin ?? "none",
            logLevel: tunnel.logLevel ?? "info",
            restart: tunnel.restart ?? true,
            enableLogStore:
              tunnel.enableLogStore ?? tunnel.enable_log_store ?? true,
            tagsText: tagsToText(tunnel.tags),
            peer: tunnel.peer ?? null,
          });
        } else {
          setForm({
            ...INITIAL_FORM,
            apiEndpoint: endpointData.length ? String(endpointData[0].id) : "",
            sharedKey: randomSharedKey(),
          });
        }
      } catch (error) {
        addToast({
          title: copy.failure,
          description: error instanceof Error ? error.message : copy.failure,
          color: "danger",
        });
      } finally {
        setLoading(false);
      }
    };

    void load();
  }, [copy.failure, instanceId, isOpen, mode]);

  const validate = () => {
    if (
      !form.apiEndpoint ||
      !form.tunnelName.trim() ||
      !form.listenPort ||
      !form.sharedKey
    ) {
      throw new Error(copy.required);
    }

    const port = Number(form.listenPort);

    if (!Number.isInteger(port) || port < 1 || port > 65535)
      throw new Error(copy.invalid);
    if (new TextEncoder().encode(form.sharedKey).length > 255)
      throw new Error(copy.invalid);
    if (form.tlsMode === "2" && (!form.certPath.trim() || !form.keyPath.trim()))
      throw new Error(copy.invalid);

    const rate = Number(form.rate || 0);
    const etar = Number(form.etar || 0);
    const poolSize = Number(form.poolSize || 0);

    if (
      ![rate, etar, poolSize].every(Number.isInteger) ||
      rate < 0 ||
      etar < 0 ||
      poolSize < 0 ||
      poolSize > 256
    )
      throw new Error(copy.invalid);
    if (form.socks !== "none" && form.next !== "none")
      throw new Error(copy.invalid);
    if (form.pin !== "none" && !/^[a-f0-9]{64}$/.test(form.pin))
      throw new Error(copy.invalid);
  };

  const submit = async () => {
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
            listenPort: form.listenPort,
            sharedKey: form.sharedKey,
            network: form.network,
            tlsMode: form.tlsMode,
            certPath: form.tlsMode === "2" ? form.certPath.trim() : "",
            keyPath: form.tlsMode === "2" ? form.keyPath.trim() : "",
            alpn: form.alpn.trim() || undefined,
            rate: form.rate !== "" ? Number(form.rate) : undefined,
            etar: form.etar !== "" ? Number(form.etar) : undefined,
            dial: form.dial.trim() || undefined,
            socks: form.socks.trim() || undefined,
            next: form.next.trim() || "none",
            up: form.up,
            down: form.down,
            poolSize: Number(form.poolSize || 0),
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
                      hint={copy.listenHostHint}
                      label={copy.listenHost}
                    >
                      <Input
                        aria-label={copy.listenHost}
                        placeholder="0.0.0.0"
                        value={form.listenHost}
                        onValueChange={(value) => update("listenHost", value)}
                      />
                    </FormField>

                    <FormField required label={copy.listenPort}>
                      <Input
                        isRequired
                        aria-label={copy.listenPort}
                        endContent={
                          <Tooltip content={copy.randomPort}>
                            <Button
                              isIconOnly
                              aria-label={copy.randomPort}
                              size="sm"
                              type="button"
                              variant="light"
                              onPress={() =>
                                update("listenPort", randomListenPort())
                              }
                            >
                              <Icon icon="lucide:dices" width={17} />
                            </Button>
                          </Tooltip>
                        }
                        max={65535}
                        min={1}
                        type="number"
                        value={form.listenPort}
                        onValueChange={(value) => update("listenPort", value)}
                      />
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

                    <FormField label={copy.network}>
                      <Tabs
                        fullWidth
                        aria-label={copy.network}
                        className="text-xs"
                        classNames={{
                          tabList: "h-10",
                          tab: "h-8",
                        }}
                        color="secondary"
                        selectedKey={form.network}
                        size="sm"
                        onSelectionChange={(key) =>
                          update("network", String(key))
                        }
                      >
                        <Tab key="mix" title="mix" />
                        <Tab key="tcp" title="tcp" />
                        <Tab key="udp" title="udp" />
                      </Tabs>
                    </FormField>

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
                        className="overflow-hidden"
                        exit={{ height: 0, opacity: 0 }}
                        id="portal-optional-configuration"
                        initial={{ height: 0, opacity: 0 }}
                        transition={{
                          duration: 0.3,
                          ease: "easeInOut",
                          height: { duration: 0.3, ease: "easeInOut" },
                        }}
                      >
                        <section className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                          <Input
                            aria-label={copy.alpn}
                            label={copy.alpn}
                            placeholder="now/1"
                            value={form.alpn}
                            onValueChange={(value) => update("alpn", value)}
                          />
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
                          <Input
                            aria-label={copy.sni}
                            label={copy.sni}
                            placeholder="none"
                            value={form.sni}
                            onValueChange={(value) => update("sni", value)}
                          />
                          <Input
                            aria-label={copy.socks}
                            className="sm:col-span-2"
                            label={copy.socks}
                            placeholder="none"
                            value={form.socks}
                            onValueChange={(value) => update("socks", value)}
                          />
                        </section>
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
