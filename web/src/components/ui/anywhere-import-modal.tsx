import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalHeader,
  Spinner,
  Tab,
  Tabs,
} from "@heroui/react";
import { Icon } from "@iconify/react/dist/offline";
import QRCode from "qrcode";
import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import {
  absoluteSubscriptionUrl,
  anywhereImportUrl,
} from "@/lib/subscriptions-api";
import { copyToClipboard } from "@/lib/utils/clipboard";

export type AnywhereImportKind = "subscription" | "vector";

interface AnywhereImportModalProps {
  isOpen: boolean;
  importUrl: string;
  kind: AnywhereImportKind;
  headerAction?: ReactNode;
  onOpenChange: (open: boolean) => void;
}

export default function AnywhereImportModal({
  isOpen,
  importUrl,
  kind,
  headerAction,
  onOpenChange,
}: AnywhereImportModalProps) {
  const { t } = useTranslation("subscriptions");
  const [qrDataUrl, setQrDataUrl] = useState("");
  const [qrError, setQrError] = useState(false);
  const [copied, setCopied] = useState(false);

  const normalizedImportUrl = useMemo(
    () => absoluteSubscriptionUrl(importUrl),
    [importUrl],
  );
  const anywhereUrl = useMemo(
    () => anywhereImportUrl(normalizedImportUrl),
    [normalizedImportUrl],
  );
  const isVector = kind === "vector";
  const copyToast = t(
    isVector ? "import.vectorToast.copied" : "import.toast.copied",
  );

  useEffect(() => {
    if (!isOpen) return;

    setCopied(false);
  }, [importUrl, isOpen, kind]);

  useEffect(() => {
    let active = true;

    if (!isOpen || !normalizedImportUrl) {
      setQrDataUrl("");
      setQrError(false);

      return;
    }

    setQrDataUrl("");
    setQrError(false);
    QRCode.toDataURL(normalizedImportUrl, {
      errorCorrectionLevel: "M",
      margin: 2,
      width: 360,
      color: { dark: "#111827", light: "#ffffff" },
    })
      .then((value) => {
        if (active) setQrDataUrl(value);
      })
      .catch(() => {
        if (active) setQrError(true);
      });

    return () => {
      active = false;
    };
  }, [isOpen, normalizedImportUrl]);

  const copyImportUrl = async () => {
    if (!normalizedImportUrl) return;

    await copyToClipboard(normalizedImportUrl, copyToast);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  const importToAnywhere = () => {
    if (anywhereUrl) window.location.assign(anywhereUrl);
  };

  const title = t(isVector ? "import.vectorTitle" : "import.title");
  const unavailable = t(
    isVector ? "import.vectorUnavailable" : "import.unavailable",
  );
  const qrAlt = t(isVector ? "import.vectorQrAlt" : "import.qrAlt");
  const qrLead = t(isVector ? "import.vectorQrLead" : "import.qrLead");
  const qrHint = t(isVector ? "import.vectorQrHint" : "import.qrHint");
  const copyLabel = t(
    isVector ? "import.copyVectorUrl" : "import.copySubscriptionUrl",
  );

  return (
    <>
      <Modal
        isOpen={isOpen}
        placement="center"
        scrollBehavior="inside"
        size="xl"
        onOpenChange={onOpenChange}
      >
        <ModalContent>
          {() => (
            <>
              <ModalHeader className="border-b border-default-100 px-5 py-3.5 pr-12">
                <div className="flex min-w-0 items-center gap-2">
                  <p className="text-lg font-semibold">{title}</p>
                  {headerAction}
                </div>
              </ModalHeader>

              <ModalBody className="gap-3 px-5 py-4">
                {!normalizedImportUrl ? (
                  <div className="flex min-h-44 flex-col items-center justify-center gap-3 rounded-xl border border-warning-200 bg-warning-50 px-5 text-center text-sm text-warning-700 dark:border-warning-800/60 dark:bg-warning-900/20 dark:text-warning-300">
                    <Icon icon="lucide:circle-alert" width={24} />
                    <span>{unavailable}</span>
                  </div>
                ) : (
                  <Tabs
                    aria-label={t("import.methods.label")}
                    classNames={{
                      base: "flex w-full justify-center",
                      tabList:
                        "w-auto rounded-lg bg-default-100/80 p-1 dark:bg-default-100/40",
                      tab: "h-11 min-w-60 px-3",
                      tabContent: "group-data-[selected=true]:text-primary",
                      cursor: "rounded-md bg-content1 shadow-small",
                      panel: "w-full px-0 pt-3",
                    }}
                    color="primary"
                    defaultSelectedKey="anywhere"
                    variant="solid"
                  >
                    <Tab
                      key="anywhere"
                      title={
                        <span className="flex items-center gap-2.5 text-left">
                          <img
                            alt="Anywhere"
                            className="size-7 shrink-0 rounded-lg object-cover shadow-small"
                            src="/anywhere-app-icon.png"
                          />
                          <span className="flex min-w-0 flex-col items-start leading-tight">
                            <span className="text-sm font-semibold">
                              {t("import.methods.anywhere")}
                            </span>
                            <span className="mt-0.5 text-[11px] font-normal text-default-500">
                              {t("import.appDescription")}
                            </span>
                          </span>
                        </span>
                      }
                    >
                      <div className="space-y-3">
                        <div className="min-w-0 space-y-3">
                          <div className="mx-auto flex size-[min(72vw,248px)] items-center justify-center overflow-hidden rounded-lg border border-default-200 bg-white p-2.5 shadow-small">
                            {qrError ? (
                              <p className="px-4 text-center text-sm text-danger">
                                {t("import.qrError")}
                              </p>
                            ) : qrDataUrl ? (
                              <img
                                alt={qrAlt}
                                className="size-full object-contain"
                                src={qrDataUrl}
                              />
                            ) : (
                              <Spinner label={t("import.qrGenerating")} />
                            )}
                          </div>

                          <div className="space-y-2.5">
                            <p className="text-sm font-semibold">{qrLead}</p>
                            <div className="flex items-start gap-2 rounded-lg bg-default-100/80 px-3 py-2.5 text-xs leading-5 text-default-600 dark:bg-default-100/30">
                              <Icon
                                className="mt-0.5 shrink-0 text-primary"
                                icon="lucide:circle-alert"
                                width={15}
                              />
                              <span>{qrHint}</span>
                            </div>
                            <ol className="grid gap-1.5 text-sm text-default-700">
                              <li className="flex items-center gap-2.5">
                                <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary-50 text-[11px] font-semibold text-primary dark:bg-primary-900/25">
                                  1
                                </span>
                                <span>{t("import.qrStepOne")}</span>
                              </li>
                              <li className="flex items-center gap-2.5">
                                <span className="flex size-5 shrink-0 items-center justify-center rounded-full bg-primary-50 text-[11px] font-semibold text-primary dark:bg-primary-900/25">
                                  2
                                </span>
                                <span>{t("import.qrStepTwo")}</span>
                              </li>
                            </ol>
                          </div>

                          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                            <Button
                              color="primary"
                              isDisabled={!anywhereUrl}
                              size="sm"
                              startContent={
                                <Icon icon="lucide:download" width={16} />
                              }
                              onPress={importToAnywhere}
                            >
                              {t("import.action")}
                            </Button>
                            <Button
                              color="primary"
                              isDisabled={!normalizedImportUrl}
                              size="sm"
                              startContent={
                                <Icon
                                  icon={copied ? "lucide:check" : "lucide:copy"}
                                  width={16}
                                />
                              }
                              onPress={() => void copyImportUrl()}
                            >
                              {copied ? t("import.copied") : copyLabel}
                            </Button>
                          </div>
                        </div>
                      </div>
                    </Tab>
                  </Tabs>
                )}
              </ModalBody>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
