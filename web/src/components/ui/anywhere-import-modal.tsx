import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
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
        size="2xl"
        onOpenChange={onOpenChange}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-start gap-3 border-b border-default-100 pb-4">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-xl bg-primary-50 text-primary dark:bg-primary-900/25">
                  <Icon icon="lucide:radio-tower" width={20} />
                </span>
                <div className="flex min-w-0 items-center gap-2">
                  <p className="text-lg font-semibold">{title}</p>
                  {headerAction}
                </div>
              </ModalHeader>

              <ModalBody className="gap-4 px-5 py-5 sm:px-6">
                {!normalizedImportUrl ? (
                  <div className="flex min-h-44 flex-col items-center justify-center gap-3 rounded-xl border border-warning-200 bg-warning-50 px-5 text-center text-sm text-warning-700 dark:border-warning-800/60 dark:bg-warning-900/20 dark:text-warning-300">
                    <Icon icon="lucide:circle-alert" width={24} />
                    <span>{unavailable}</span>
                  </div>
                ) : (
                  <Tabs
                    aria-label={t("import.methods.label")}
                    classNames={{
                      base: "w-full",
                      tabList:
                        "w-full justify-center gap-5 rounded-none border-b border-default-200 p-0",
                      tab: "h-14 min-w-52 px-3",
                      tabContent: "group-data-[selected=true]:text-primary",
                      panel: "px-0 pt-4",
                    }}
                    color="primary"
                    defaultSelectedKey="anywhere"
                    variant="underlined"
                  >
                    <Tab
                      key="anywhere"
                      title={
                        <span className="flex items-center gap-2.5 text-left">
                          <img
                            alt="Anywhere"
                            className="size-8 shrink-0 rounded-lg object-cover shadow-small"
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
                      <div className="space-y-4">
                        <div className="min-w-0 space-y-4">
                          <div className="mx-auto flex size-[min(76vw,320px)] items-center justify-center overflow-hidden rounded-xl border border-default-200 bg-white p-3 shadow-small">
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

                          <div className="space-y-3">
                            <p className="text-sm font-semibold">{qrLead}</p>
                            <div className="flex items-start gap-2 rounded-lg bg-default-100/80 px-3.5 py-3 text-xs leading-5 text-default-600 dark:bg-default-100/30">
                              <Icon
                                className="mt-0.5 shrink-0 text-primary"
                                icon="lucide:circle-alert"
                                width={15}
                              />
                              <span>{qrHint}</span>
                            </div>
                            <ol className="grid gap-2 text-sm text-default-700">
                              <li className="flex items-center gap-3">
                                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary dark:bg-primary-900/25">
                                  1
                                </span>
                                <span>{t("import.qrStepOne")}</span>
                              </li>
                              <li className="flex items-center gap-3">
                                <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary-50 text-xs font-semibold text-primary dark:bg-primary-900/25">
                                  2
                                </span>
                                <span>{t("import.qrStepTwo")}</span>
                              </li>
                            </ol>
                          </div>

                          <div className="grid grid-cols-2 gap-2">
                            <Button
                              color="primary"
                              isDisabled={!anywhereUrl}
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

              <ModalFooter className="border-t border-default-100 pt-3">
                <Button variant="light" onPress={onClose}>
                  {t("actions.close")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
