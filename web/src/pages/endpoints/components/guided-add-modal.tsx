import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Tab,
  Tabs,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import { buildApiUrl } from "@/lib/utils";

const GITHUB_PROXY_PRESETS = [
  "https://ghproxy.com/",
  "https://mirror.ghproxy.com/",
  "https://ghps.cc/",
  "https://gh.api.99988866.xyz/",
  "https://github.moeyy.xyz/",
  "https://gh-proxy.com/",
  "https://download.scholar.rr.nu/",
  "https://hub.gitmirror.com/",
] as const;

type TLSMode = "0" | "1" | "2";

interface GuidedInstallForm {
  certPath: string;
  dashUrl: string;
  keyPath: string;
  listenHost: string;
  name: string;
  port: string;
  prefix: string;
  tlsMode: TLSMode;
}

interface GuidedAddModalProps {
  installScriptPath: string;
  installScriptUrl: string;
  isOpen: boolean;
  onCopyCommand: (command: string) => void;
  onOpenChange: (open: boolean) => void;
}

interface RegistrationTokenResponse {
  error?: string;
  expiresAt?: string;
  token?: string;
}

function createDefaultName() {
  const now = new Date();
  const stamp = [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, "0"),
    String(now.getDate()).padStart(2, "0"),
    "-",
    String(now.getHours()).padStart(2, "0"),
    String(now.getMinutes()).padStart(2, "0"),
    String(now.getSeconds()).padStart(2, "0"),
  ].join("");

  return `Nowhere-${stamp}`;
}

function createDefaultForm(): GuidedInstallForm {
  return {
    certPath: "",
    dashUrl: typeof window === "undefined" ? "" : window.location.origin,
    keyPath: "",
    listenHost: "",
    name: createDefaultName(),
    port: "10101",
    prefix: "api",
    tlsMode: "1",
  };
}

function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'"'"'`)}'`;
}

function applyGitHubProxy(url: string, proxy: string) {
  if (!proxy) return url;
  return `${proxy.replace(/\/+$/, "")}/${url}`;
}

function buildRegisterUrl(baseUrl: string) {
  const url = new URL(baseUrl.trim());

  url.hash = "";
  url.search = "";
  url.pathname = `${url.pathname.replace(/\/+$/, "")}/api/endpoints/register`;
  return url.toString();
}

export default function GuidedAddModal({
  installScriptPath,
  installScriptUrl,
  isOpen,
  onCopyCommand,
  onOpenChange,
}: GuidedAddModalProps) {
  const { t } = useTranslation("endpoints");
  const [form, setForm] =
    useState<Readonly<GuidedInstallForm>>(createDefaultForm);
  const [githubProxyChoice, setGithubProxyChoice] = useState("none");
  const [customGithubProxy, setCustomGithubProxy] = useState("");
  const [generatedCommand, setGeneratedCommand] = useState("");
  const [expiresAt, setExpiresAt] = useState("");
  const [isGenerating, setIsGenerating] = useState(false);

  useEffect(() => {
    if (!isOpen) return;
    setForm(createDefaultForm());
    setGithubProxyChoice("none");
    setCustomGithubProxy("");
    setGeneratedCommand("");
    setExpiresAt("");
  }, [isOpen]);

  const invalidateCommand = () => {
    setGeneratedCommand("");
    setExpiresAt("");
  };

  const updateField = <K extends keyof GuidedInstallForm>(
    field: K,
    value: GuidedInstallForm[K],
  ) => {
    setForm((current) => ({ ...current, [field]: value }));
    invalidateCommand();
  };

  const selectedGithubProxy =
    githubProxyChoice === "none"
      ? ""
      : githubProxyChoice === "custom"
        ? customGithubProxy.trim()
        : githubProxyChoice;

  const validateForm = () => {
    if (!form.name.trim() || form.name.trim().length > 50) {
      return t("guidedAdd.validation.name");
    }
    if (
      form.listenHost.trim() &&
      !/^[A-Za-z0-9._:[\]-]+$/.test(form.listenHost.trim())
    ) {
      return t("guidedAdd.validation.listenHost");
    }
    const port = Number(form.port);
    if (!Number.isInteger(port) || port < 1 || port > 65535) {
      return t("guidedAdd.validation.port");
    }
    if (!/^[A-Za-z0-9_-]+(?:\/[A-Za-z0-9_-]+)*$/.test(form.prefix)) {
      return t("guidedAdd.validation.prefix");
    }
    if (form.tlsMode === "2" && (!form.certPath || !form.keyPath)) {
      return t("guidedAdd.validation.certificate");
    }

    try {
      const dashUrl = new URL(form.dashUrl.trim());

      if (
        !["http:", "https:"].includes(dashUrl.protocol) ||
        dashUrl.username ||
        dashUrl.password
      ) {
        return t("guidedAdd.validation.dashUrl");
      }
    } catch {
      return t("guidedAdd.validation.dashUrl");
    }

    if (selectedGithubProxy) {
      try {
        const proxyUrl = new URL(selectedGithubProxy);

        if (!["http:", "https:"].includes(proxyUrl.protocol)) {
          return t("guidedAdd.validation.githubProxy");
        }
      } catch {
        return t("guidedAdd.validation.githubProxy");
      }
    }

    return "";
  };

  const buildCommand = (token: string) => {
    const installerUrl = applyGitHubProxy(
      installScriptUrl,
      selectedGithubProxy,
    );
    const options: string[] = [];

    if (form.listenHost.trim()) {
      options.push(
        `--openctrl-listen ${shellQuote(form.listenHost.trim())}`,
      );
    }
    options.push(
      `--openctrl-port ${shellQuote(form.port)}`,
      `--openctrl-prefix ${shellQuote(form.prefix)}`,
      `--openctrl-tls ${shellQuote(form.tlsMode)}`,
    );

    if (form.tlsMode === "2") {
      options.push(`--openctrl-cert ${shellQuote(form.certPath.trim())}`);
      options.push(`--openctrl-key ${shellQuote(form.keyPath.trim())}`);
    }
    if (selectedGithubProxy) {
      options.push(`--github-proxy ${shellQuote(selectedGithubProxy)}`);
    }
    options.push(
      `--register-url ${shellQuote(buildRegisterUrl(form.dashUrl.trim()))}`,
    );
    options.push(`--register-token ${shellQuote(token)}`);

    const optionLines = options
      .map(
        (option, index) =>
          `  ${option}${index === options.length - 1 ? "" : " \\"}`,
      )
      .join("\n");

    return [
      `curl -fsSL ${shellQuote(installerUrl)} -o ${shellQuote(installScriptPath)} &&`,
      `sudo bash ${shellQuote(installScriptPath)} install nowhere --yes \\`,
      optionLines,
    ].join("\n");
  };

  const handleGenerate = async () => {
    const validationError = validateForm();

    if (validationError) {
      addToast({
        color: "warning",
        description: validationError,
        title: t("guidedAdd.validation.title"),
      });
      return;
    }

    setIsGenerating(true);
    try {
      const response = await fetch(
        buildApiUrl("/api/endpoints/registration-token"),
        {
          body: JSON.stringify({ name: form.name.trim() }),
          headers: { "Content-Type": "application/json" },
          method: "POST",
        },
      );
      const data = (await response
        .json()
        .catch(() => ({}))) as RegistrationTokenResponse;

      if (!response.ok || !data.token || !data.expiresAt) {
        throw new Error(data.error || t("guidedAdd.generateFailedDesc"));
      }

      const command = buildCommand(data.token);

      setGeneratedCommand(command);
      setExpiresAt(data.expiresAt);
      onCopyCommand(command);
    } catch (error) {
      addToast({
        color: "danger",
        description:
          error instanceof Error
            ? error.message
            : t("guidedAdd.generateFailedDesc"),
        title: t("guidedAdd.generateFailed"),
      });
    } finally {
      setIsGenerating(false);
    }
  };

  const handleOpenChange = (open: boolean) => {
    if (!open) {
      setGeneratedCommand("");
      setExpiresAt("");
    }
    onOpenChange(open);
  };

  return (
    <Modal
      isOpen={isOpen}
      placement="top-center"
      scrollBehavior="inside"
      size="3xl"
      onOpenChange={handleOpenChange}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex items-center gap-2">
              <Icon
                className="text-primary"
                icon="lucide:list-checks"
                width={20}
              />
              {t("guidedAdd.title")}
            </ModalHeader>
            <ModalBody className="gap-4 pb-2">
              <section>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Input
                    isRequired
                    label={t("guidedAdd.name")}
                    labelPlacement="outside"
                    maxLength={50}
                    value={form.name}
                    onValueChange={(value) => updateField("name", value)}
                  />
                  <Input
                    isRequired
                    description={t("guidedAdd.dashUrlHint")}
                    label={t("guidedAdd.dashUrl")}
                    labelPlacement="outside"
                    type="url"
                    value={form.dashUrl}
                    onValueChange={(value) => updateField("dashUrl", value)}
                  />
                </div>
              </section>

              <section>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
                  <Input
                    label={t("guidedAdd.listenHost")}
                    labelPlacement="outside"
                    placeholder="0.0.0.0"
                    value={form.listenHost}
                    onValueChange={(value) => updateField("listenHost", value)}
                  />
                  <Input
                    isRequired
                    label={t("guidedAdd.port")}
                    labelPlacement="outside"
                    max={65535}
                    min={1}
                    type="number"
                    value={form.port}
                    onValueChange={(value) => updateField("port", value)}
                  />
                  <Input
                    isRequired
                    label={t("guidedAdd.prefix")}
                    labelPlacement="outside"
                    startContent={
                      <span className="text-small text-default-400">/</span>
                    }
                    value={form.prefix}
                    onValueChange={(value) => updateField("prefix", value)}
                  />
                </div>

                <div className="mt-4">
                  <p className="mb-2 text-small font-medium text-foreground">
                    {t("guidedAdd.tlsMode")}
                  </p>
                  <Tabs
                    fullWidth
                    aria-label={t("guidedAdd.tlsMode")}
                    selectedKey={form.tlsMode}
                    size="sm"
                    variant="bordered"
                    onSelectionChange={(key) =>
                      updateField("tlsMode", String(key) as TLSMode)
                    }
                  >
                    <Tab key="0" title={t("guidedAdd.tlsHttp")} />
                    <Tab key="1" title={t("guidedAdd.tlsSelfSigned")} />
                    <Tab key="2" title={t("guidedAdd.tlsPem")} />
                  </Tabs>
                </div>

                {form.tlsMode === "2" && (
                  <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <Input
                      isRequired
                      label={t("guidedAdd.certPath")}
                      labelPlacement="outside"
                      placeholder="/etc/letsencrypt/live/example.com/fullchain.pem"
                      value={form.certPath}
                      onValueChange={(value) => updateField("certPath", value)}
                    />
                    <Input
                      isRequired
                      label={t("guidedAdd.keyPath")}
                      labelPlacement="outside"
                      placeholder="/etc/letsencrypt/live/example.com/privkey.pem"
                      value={form.keyPath}
                      onValueChange={(value) => updateField("keyPath", value)}
                    />
                  </div>
                )}
              </section>

              <section>
                <Select
                  classNames={{ value: "truncate font-mono text-xs" }}
                  items={[
                    { key: "none", label: t("guidedAdd.proxyNone") },
                    ...GITHUB_PROXY_PRESETS.map((proxy) => ({
                      key: proxy,
                      label: proxy,
                    })),
                    { key: "custom", label: t("guidedAdd.proxyCustom") },
                  ]}
                  label={t("guidedAdd.githubProxy")}
                  labelPlacement="outside"
                  selectedKeys={[githubProxyChoice]}
                  onSelectionChange={(keys) => {
                    const choice = String(Array.from(keys)[0] ?? "none");

                    setGithubProxyChoice(choice);
                    invalidateCommand();
                  }}
                >
                  {(option) => (
                    <SelectItem key={option.key}>{option.label}</SelectItem>
                  )}
                </Select>
                {githubProxyChoice === "custom" && (
                  <Input
                    className="mt-4"
                    label={t("guidedAdd.customGithubProxy")}
                    labelPlacement="outside"
                    placeholder="https://proxy.example.com/"
                    type="url"
                    value={customGithubProxy}
                    onValueChange={(value) => {
                      setCustomGithubProxy(value);
                      invalidateCommand();
                    }}
                  />
                )}
                {generatedCommand && (
                  <div className="mt-5 border-t border-divider pt-5">
                    <div className="mb-2 flex items-center justify-between gap-3">
                      <div>
                        <h3 className="text-sm font-semibold text-success">
                          {t("guidedAdd.commandReady")}
                        </h3>
                        <p className="mt-1 text-xs text-default-500">
                          {t("guidedAdd.tokenExpiresAt", {
                            time: new Date(expiresAt).toLocaleTimeString([], {
                              hour: "2-digit",
                              minute: "2-digit",
                            }),
                          })}
                        </p>
                      </div>
                      <Tooltip content={t("guidedAdd.copyCommand")}>
                        <Button
                          isIconOnly
                          aria-label={t("guidedAdd.copyCommand")}
                          size="sm"
                          variant="light"
                          onPress={() => onCopyCommand(generatedCommand)}
                        >
                          <Icon icon="lucide:copy" width={17} />
                        </Button>
                      </Tooltip>
                    </div>
                    <pre className="max-h-48 overflow-auto whitespace-pre-wrap break-all rounded-md border border-divider bg-default-100 p-3 font-mono text-xs leading-5 text-default-700">
                      {generatedCommand}
                    </pre>
                  </div>
                )}
              </section>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                {t("guidedAdd.cancel")}
              </Button>
              <Button
                color="primary"
                isLoading={isGenerating}
                startContent={
                  !isGenerating && <Icon icon="lucide:copy-check" width={17} />
                }
                onPress={handleGenerate}
              >
                {generatedCommand
                  ? t("guidedAdd.regenerate")
                  : t("guidedAdd.generate")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
