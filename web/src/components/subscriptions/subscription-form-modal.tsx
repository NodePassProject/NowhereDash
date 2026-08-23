import {
  Button,
  Checkbox,
  DatePicker,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ScrollShadow,
  Tooltip,
} from "@heroui/react";
import { Icon } from "@iconify/react/dist/offline";
import { addToast } from "@heroui/toast";
import { parseDateTime } from "@internationalized/date";
import {
  type ComponentProps,
  type FormEvent,
  type ReactNode,
  useEffect,
  useMemo,
  useState,
} from "react";
import { useTranslation } from "react-i18next";

import SubscriptionIconPicker from "@/components/subscriptions/subscription-icon-picker";
import {
  createSubscription,
  PortalOption,
  PortalSubscription,
  SubscriptionPayload,
  updateSubscription,
} from "@/lib/subscriptions-api";

interface SubscriptionFormModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  portals: PortalOption[];
  subscription?: PortalSubscription | null;
  onSaved: () => void | Promise<void>;
}

interface SubscriptionFormState {
  icon: string | null;
  name: string;
  expiresAt: string;
  trafficLimitGiB: string;
  tunnelIds: number[];
}

const GIB = 1024 ** 3;

const INITIAL_FORM: SubscriptionFormState = {
  icon: null,
  name: "",
  expiresAt: "",
  trafficLimitGiB: "",
  tunnelIds: [],
};

interface FormFieldProps {
  children: ReactNode;
  label: string;
  className?: string;
  hint?: string;
  required?: boolean;
}

interface FieldLabelProps {
  label: string;
  required?: boolean;
}

interface TunnelTransferListProps {
  countLabel: string;
  emptyLabel: string;
  noResultsLabel: string;
  portals: PortalOption[];
  query: string;
  searchPlaceholder: string;
  selectAllLabel: string;
  selection: Set<number>;
  statusLabels: Record<PortalOption["status"], string>;
  title: string;
  totalCount: number;
  onQueryChange: (value: string) => void;
  onSelectVisible: (selected: boolean) => void;
  onSelectionChange: (portalId: number, selected: boolean) => void;
}

function FieldLabel({ label, required = false }: FieldLabelProps) {
  return (
    <div className="flex min-h-5 items-center gap-1 px-1 text-sm text-foreground-600">
      <span>{label}</span>
      {required && (
        <span aria-hidden="true" className="text-danger">
          *
        </span>
      )}
    </div>
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
      <FieldLabel label={label} required={required} />
      {children}
      {hint && (
        <p className="line-clamp-2 px-1 text-xs leading-4 text-default-400">
          {hint}
        </p>
      )}
    </div>
  );
}

const PORTAL_STATUS_COLORS: Record<PortalOption["status"], string> = {
  running: "bg-success",
  stopped: "bg-warning",
  error: "bg-danger",
  offline: "bg-default-300",
};

function TunnelTransferList({
  countLabel,
  emptyLabel,
  noResultsLabel,
  portals,
  query,
  searchPlaceholder,
  selectAllLabel,
  selection,
  statusLabels,
  title,
  totalCount,
  onQueryChange,
  onSelectVisible,
  onSelectionChange,
}: TunnelTransferListProps) {
  const allVisibleSelected =
    portals.length > 0 && portals.every((portal) => selection.has(portal.id));
  const someVisibleSelected = portals.some((portal) =>
    selection.has(portal.id),
  );

  return (
    <div className="flex h-[19rem] min-w-0 flex-col overflow-hidden rounded-medium border border-default-200 bg-content1">
      <div className="flex min-h-11 items-center gap-2 border-b border-default-100 px-3">
        <Checkbox
          aria-label={selectAllLabel}
          isIndeterminate={someVisibleSelected && !allVisibleSelected}
          isSelected={allVisibleSelected}
          size="sm"
          onValueChange={onSelectVisible}
        />
        <div className="flex min-w-0 flex-1 items-center justify-between gap-2">
          <span className="truncate text-sm font-medium text-foreground-700">
            {title}
          </span>
          <span className="shrink-0 text-xs tabular-nums text-default-400">
            {countLabel}
          </span>
        </div>
      </div>
      <div className="border-b border-default-100 p-2">
        <Input
          isClearable
          aria-label={searchPlaceholder}
          classNames={{ inputWrapper: "h-9 min-h-9 shadow-none" }}
          placeholder={searchPlaceholder}
          size="sm"
          startContent={
            <Icon
              className="text-default-400"
              icon="lucide:search"
              width={15}
            />
          }
          value={query}
          variant="flat"
          onClear={() => onQueryChange("")}
          onValueChange={onQueryChange}
        />
      </div>
      <ScrollShadow
        aria-label={title}
        className="min-h-0 flex-1 overflow-y-auto p-1.5"
        role="group"
      >
        {portals.length > 0 ? (
          <div className="space-y-1">
            {portals.map((portal) => (
              <Checkbox
                key={portal.id}
                aria-label={portal.name}
                classNames={{
                  base: "m-0 flex w-full max-w-none items-center rounded-small px-2 py-2 transition-colors hover:bg-default-100 data-[selected=true]:bg-primary-50 dark:data-[selected=true]:bg-primary-100/10",
                  label: "min-w-0 flex-1",
                  wrapper: "me-2 shrink-0",
                }}
                isSelected={selection.has(portal.id)}
                size="sm"
                onValueChange={(selected) =>
                  onSelectionChange(portal.id, selected)
                }
              >
                <div className="flex min-w-0 items-center justify-between gap-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-foreground-700">
                      {portal.name}
                    </p>
                    <p className="truncate font-mono text-xs text-default-400">
                      {portal.listenHost || "*"}:{portal.listenPort}
                    </p>
                  </div>
                  <span
                    className="flex shrink-0 items-center gap-1.5 text-xs text-default-500"
                    title={statusLabels[portal.status]}
                  >
                    <span
                      className={`size-2 rounded-full ${PORTAL_STATUS_COLORS[portal.status]}`}
                    />
                    <span className="hidden 2xl:inline">
                      {statusLabels[portal.status]}
                    </span>
                  </span>
                </div>
              </Checkbox>
            ))}
          </div>
        ) : (
          <div className="flex h-full min-h-32 flex-col items-center justify-center px-4 text-center">
            <Icon
              className="mb-2 text-default-300"
              icon={query ? "lucide:search-x" : "lucide:inbox"}
              width={24}
            />
            <p className="text-xs leading-5 text-default-400">
              {totalCount === 0 ? emptyLabel : noResultsLabel}
            </p>
          </div>
        )}
      </ScrollShadow>
    </div>
  );
}

const toLocalDateTime = (value: string | null) => {
  if (!value) return "";
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);

  return local.toISOString().slice(0, 16);
};

const parseLocalDateTime = (value: string) => {
  if (!value) return null;

  try {
    return parseDateTime(value);
  } catch {
    return null;
  }
};

const matchesPortalQuery = (portal: PortalOption, query: string) => {
  const normalizedQuery = query.trim().toLocaleLowerCase();

  if (!normalizedQuery) return true;

  return [portal.name, portal.listenHost, portal.listenPort, portal.status]
    .join(" ")
    .toLocaleLowerCase()
    .includes(normalizedQuery);
};

const toFormState = (
  subscription?: PortalSubscription | null,
): SubscriptionFormState => {
  if (!subscription) return INITIAL_FORM;

  return {
    icon: subscription.icon.startsWith("data:image/png;base64,")
      ? subscription.icon
      : null,
    name: subscription.name,
    expiresAt: toLocalDateTime(subscription.expiresAt),
    trafficLimitGiB:
      subscription.trafficLimit == null
        ? ""
        : String(Number((subscription.trafficLimit / GIB).toFixed(3))),
    tunnelIds: subscription.tunnelIds,
  };
};

export default function SubscriptionFormModal({
  isOpen,
  onOpenChange,
  portals,
  subscription,
  onSaved,
}: SubscriptionFormModalProps) {
  const { t } = useTranslation("subscriptions");
  const [form, setForm] = useState<SubscriptionFormState>(INITIAL_FORM);
  const [iconChanged, setIconChanged] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [attempted, setAttempted] = useState(false);
  const [availableQuery, setAvailableQuery] = useState("");
  const [selectedQuery, setSelectedQuery] = useState("");
  const [availableSelection, setAvailableSelection] = useState<Set<number>>(
    () => new Set(),
  );
  const [selectedSelection, setSelectedSelection] = useState<Set<number>>(
    () => new Set(),
  );
  const [expirationPortal, setExpirationPortal] =
    useState<HTMLDivElement | null>(null);
  const editing = Boolean(subscription);

  useEffect(() => {
    if (!isOpen) return;
    setForm(toFormState(subscription));
    setIconChanged(false);
    setAttempted(false);
    setAvailableQuery("");
    setSelectedQuery("");
    setAvailableSelection(new Set());
    setSelectedSelection(new Set());
  }, [isOpen, subscription]);

  const selectedIds = useMemo(() => new Set(form.tunnelIds), [form.tunnelIds]);
  const availablePortals = useMemo(
    () => portals.filter((portal) => !selectedIds.has(portal.id)),
    [portals, selectedIds],
  );
  const selectedPortals = useMemo(
    () => portals.filter((portal) => selectedIds.has(portal.id)),
    [portals, selectedIds],
  );
  const filteredAvailablePortals = useMemo(() => {
    return availablePortals.filter((portal) =>
      matchesPortalQuery(portal, availableQuery),
    );
  }, [availablePortals, availableQuery]);
  const filteredSelectedPortals = useMemo(() => {
    return selectedPortals.filter((portal) =>
      matchesPortalQuery(portal, selectedQuery),
    );
  }, [selectedPortals, selectedQuery]);
  const setPortalSelection = (
    side: "available" | "selected",
    portalId: number,
    selected: boolean,
  ) => {
    const setter =
      side === "available" ? setAvailableSelection : setSelectedSelection;

    setter((current) => {
      const next = new Set(current);

      if (selected) next.add(portalId);
      else next.delete(portalId);

      return next;
    });
  };

  const setFilteredSelection = (
    side: "available" | "selected",
    checked: boolean,
  ) => {
    const visiblePortals =
      side === "available" ? filteredAvailablePortals : filteredSelectedPortals;
    const setter =
      side === "available" ? setAvailableSelection : setSelectedSelection;

    setter((current) => {
      const next = new Set(current);

      visiblePortals.forEach((portal) => {
        if (checked) next.add(portal.id);
        else next.delete(portal.id);
      });

      return next;
    });
  };

  const addPortals = (portalIds: number[]) => {
    if (portalIds.length === 0) return;

    setForm((current) => {
      const nextIds = new Set(current.tunnelIds);

      portalIds.forEach((portalId) => nextIds.add(portalId));

      return { ...current, tunnelIds: Array.from(nextIds) };
    });
    setAvailableSelection((current) => {
      const next = new Set(current);

      portalIds.forEach((portalId) => next.delete(portalId));

      return next;
    });
  };

  const removePortals = (portalIds: number[]) => {
    if (portalIds.length === 0) return;

    const removing = new Set(portalIds);

    setForm((current) => ({
      ...current,
      tunnelIds: current.tunnelIds.filter(
        (portalId) => !removing.has(portalId),
      ),
    }));
    setSelectedSelection((current) => {
      const next = new Set(current);

      portalIds.forEach((portalId) => next.delete(portalId));

      return next;
    });
  };

  const expirationValue = useMemo(
    () => parseLocalDateTime(form.expiresAt),
    [form.expiresAt],
  );

  const closeExpirationPicker = (target: Element) => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", {
        bubbles: true,
        cancelable: true,
        key: "Escape",
      }),
    );
  };

  const nameInvalid = attempted && !form.name.trim();
  const portalsInvalid = attempted && form.tunnelIds.length === 0;
  const trafficValue = form.trafficLimitGiB.trim()
    ? Number(form.trafficLimitGiB)
    : null;
  const trafficInvalid =
    attempted &&
    trafficValue !== null &&
    (!Number.isFinite(trafficValue) || trafficValue <= 0);
  const portalStatusLabels: Record<PortalOption["status"], string> = {
    running: t("form.tunnelStatus.running"),
    stopped: t("form.tunnelStatus.stopped"),
    error: t("form.tunnelStatus.error"),
    offline: t("form.tunnelStatus.offline"),
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setAttempted(true);

    if (nameInvalid || portalsInvalid || trafficInvalid) return;
    if (
      !form.name.trim() ||
      form.tunnelIds.length === 0 ||
      (trafficValue !== null &&
        (!Number.isFinite(trafficValue) || trafficValue <= 0))
    )
      return;

    const payload: SubscriptionPayload = {
      name: form.name.trim(),
      profileTitle: form.name.trim(),
      expiresAt: form.expiresAt ? new Date(form.expiresAt).toISOString() : null,
      trafficLimit:
        trafficValue === null ? null : Math.round(trafficValue * GIB),
      preferences: subscription
        ? { ...subscription.preferences, includeIpv6: false }
        : {
            expandCarrierCombos: true,
            upCarrier: "tcp",
            downCarrier: "tcp",
            includeIpv6: false,
          },
      tunnelIds: form.tunnelIds,
      ...(iconChanged ? { icon: form.icon ?? "" } : {}),
    };

    setSubmitting(true);
    try {
      if (subscription) {
        await updateSubscription(subscription.id, payload);
      } else {
        await createSubscription(payload);
      }
      addToast({
        title: t(editing ? "toast.updated" : "toast.created"),
        color: "success",
      });
      onOpenChange(false);
      await onSaved();
    } catch (error) {
      addToast({
        title: t("toast.saveFailed"),
        description:
          error instanceof Error ? error.message : t("toast.saveFailed"),
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
      size="5xl"
      onOpenChange={(open) => {
        if (!submitting) onOpenChange(open);
      }}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <form
              className="flex max-h-full min-h-0 flex-1 flex-col overflow-hidden"
              onSubmit={submit}
            >
              <ModalHeader className="flex items-center gap-2 pb-0">
                <Icon
                  className="shrink-0 text-primary"
                  icon="lucide:rss"
                  width={19}
                />
                <span className="text-base font-semibold">
                  {t(editing ? "form.editTitle" : "form.createTitle")}
                </span>
              </ModalHeader>
              <ModalBody className="min-h-0 flex-1 overflow-y-auto py-4">
                <div className="min-w-0 space-y-5">
                  <section className="min-w-0 space-y-3">
                    <div className="grid grid-cols-1 gap-x-3 gap-y-2 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_7.25rem]">
                      <FormField
                        required
                        className="md:col-span-2"
                        label={t("fields.name")}
                      >
                        <Input
                          isRequired
                          aria-label={t("fields.name")}
                          errorMessage={
                            nameInvalid ? t("validation.name") : undefined
                          }
                          isInvalid={nameInvalid}
                          placeholder={t("form.namePlaceholder")}
                          value={form.name}
                          onValueChange={(name) =>
                            setForm((old) => ({ ...old, name }))
                          }
                        />
                      </FormField>
                      <SubscriptionIconPicker
                        value={form.icon}
                        onChange={(icon) => {
                          setForm((current) => ({ ...current, icon }));
                          setIconChanged(true);
                        }}
                      />
                      <FormField
                        hint={t("form.noExpiry")}
                        label={t("fields.expiresAt")}
                      >
                        <DatePicker
                          shouldForceLeadingZeros
                          showMonthAndYearPickers
                          CalendarBottomContent={
                            <div className="flex items-center justify-between border-t border-default-100 px-3 py-2">
                              <Button
                                size="sm"
                                startContent={
                                  <Icon icon="lucide:infinity" width={15} />
                                }
                                type="button"
                                variant="light"
                                onPress={(event) => {
                                  setForm((old) => ({
                                    ...old,
                                    expiresAt: "",
                                  }));
                                  closeExpirationPicker(event.target);
                                }}
                              >
                                {t("actions.neverExpire")}
                              </Button>
                              <Button
                                color="primary"
                                size="sm"
                                type="button"
                                variant="flat"
                                onPress={(event) =>
                                  closeExpirationPicker(event.target)
                                }
                              >
                                {t("actions.done")}
                              </Button>
                            </div>
                          }
                          aria-label={t("fields.expiresAt")}
                          classNames={{
                            inputWrapper: "shadow-none",
                            popoverContent:
                              "border border-default-200 bg-content1 shadow-large",
                            selectorButton:
                              "text-default-500 data-[hover=true]:text-primary",
                            selectorIcon: "text-base",
                            timeInput:
                              "border-t border-default-100 px-4 pb-3 pt-3",
                          }}
                          granularity="minute"
                          hourCycle={24}
                          popoverProps={{
                            portalContainer: expirationPortal ?? undefined,
                          }}
                          selectorIcon={
                            <Icon icon="lucide:calendar-days" width={17} />
                          }
                          value={
                            expirationValue as unknown as ComponentProps<
                              typeof DatePicker
                            >["value"]
                          }
                          onChange={(expiresAt) =>
                            setForm((old) => ({
                              ...old,
                              expiresAt:
                                expiresAt?.toString().slice(0, 16) ?? "",
                            }))
                          }
                        />
                      </FormField>
                      <FormField
                        hint={t("form.unlimited")}
                        label={t("fields.trafficLimit")}
                      >
                        <Input
                          aria-label={t("fields.trafficLimit")}
                          endContent={
                            <span className="text-xs text-default-400">
                              GiB
                            </span>
                          }
                          errorMessage={
                            trafficInvalid
                              ? t("validation.trafficLimit")
                              : undefined
                          }
                          isInvalid={trafficInvalid}
                          min="0.001"
                          placeholder="0"
                          step="0.001"
                          type="number"
                          value={form.trafficLimitGiB}
                          onValueChange={(trafficLimitGiB) =>
                            setForm((old) => ({ ...old, trafficLimitGiB }))
                          }
                        />
                      </FormField>
                    </div>
                  </section>

                  <section className="min-w-0 border-t border-default-200 pt-3">
                    <div className="mb-2 flex min-h-5 items-center justify-between gap-3">
                      <FieldLabel required label={t("fields.portals")} />
                      <span className="shrink-0 text-xs tabular-nums text-default-500">
                        {t("form.selectedTunnelCount", {
                          count: selectedPortals.length,
                        })}
                      </span>
                    </div>

                    <div className="grid min-w-0 items-center gap-2 md:grid-cols-[minmax(0,1fr)_2.5rem_minmax(0,1fr)]">
                      <TunnelTransferList
                        countLabel={t("form.tunnelCount", {
                          count: availablePortals.length,
                        })}
                        emptyLabel={t("form.noAvailableTunnels")}
                        noResultsLabel={t("form.noTunnelResults")}
                        portals={filteredAvailablePortals}
                        query={availableQuery}
                        searchPlaceholder={t("form.searchTunnels")}
                        selectAllLabel={t("form.selectAllAvailable")}
                        selection={availableSelection}
                        statusLabels={portalStatusLabels}
                        title={t("form.availableTunnels")}
                        totalCount={availablePortals.length}
                        onQueryChange={setAvailableQuery}
                        onSelectVisible={(selected) =>
                          setFilteredSelection("available", selected)
                        }
                        onSelectionChange={(portalId, selected) =>
                          setPortalSelection("available", portalId, selected)
                        }
                      />

                      <div className="flex items-center justify-center gap-1.5 md:flex-col">
                        <Tooltip content={t("form.addSelectedTunnels")}>
                          <Button
                            isIconOnly
                            aria-label={t("form.addSelectedTunnels")}
                            color="primary"
                            isDisabled={availableSelection.size === 0}
                            size="sm"
                            type="button"
                            variant="flat"
                            onPress={() =>
                              addPortals(Array.from(availableSelection))
                            }
                          >
                            <Icon
                              className="rotate-90 md:rotate-0"
                              icon="lucide:chevron-right"
                              width={17}
                            />
                          </Button>
                        </Tooltip>
                        <Tooltip content={t("form.addAllVisibleTunnels")}>
                          <Button
                            isIconOnly
                            aria-label={t("form.addAllVisibleTunnels")}
                            color="primary"
                            isDisabled={filteredAvailablePortals.length === 0}
                            size="sm"
                            type="button"
                            variant="light"
                            onPress={() =>
                              addPortals(
                                filteredAvailablePortals.map(
                                  (portal) => portal.id,
                                ),
                              )
                            }
                          >
                            <Icon
                              className="rotate-90 md:rotate-0"
                              icon="lucide:chevrons-right"
                              width={17}
                            />
                          </Button>
                        </Tooltip>
                        <Tooltip content={t("form.removeSelectedTunnels")}>
                          <Button
                            isIconOnly
                            aria-label={t("form.removeSelectedTunnels")}
                            isDisabled={selectedSelection.size === 0}
                            size="sm"
                            type="button"
                            variant="flat"
                            onPress={() =>
                              removePortals(Array.from(selectedSelection))
                            }
                          >
                            <Icon
                              className="rotate-90 md:rotate-0"
                              icon="lucide:chevron-left"
                              width={17}
                            />
                          </Button>
                        </Tooltip>
                        <Tooltip content={t("form.removeAllVisibleTunnels")}>
                          <Button
                            isIconOnly
                            aria-label={t("form.removeAllVisibleTunnels")}
                            isDisabled={filteredSelectedPortals.length === 0}
                            size="sm"
                            type="button"
                            variant="light"
                            onPress={() =>
                              removePortals(
                                filteredSelectedPortals.map(
                                  (portal) => portal.id,
                                ),
                              )
                            }
                          >
                            <Icon
                              className="rotate-90 md:rotate-0"
                              icon="lucide:chevrons-left"
                              width={17}
                            />
                          </Button>
                        </Tooltip>
                      </div>

                      <TunnelTransferList
                        countLabel={t("form.tunnelCount", {
                          count: selectedPortals.length,
                        })}
                        emptyLabel={t("form.noSelectedTunnels")}
                        noResultsLabel={t("form.noTunnelResults")}
                        portals={filteredSelectedPortals}
                        query={selectedQuery}
                        searchPlaceholder={t("form.searchTunnels")}
                        selectAllLabel={t("form.selectAllSelected")}
                        selection={selectedSelection}
                        statusLabels={portalStatusLabels}
                        title={t("form.selectedTunnels")}
                        totalCount={selectedPortals.length}
                        onQueryChange={setSelectedQuery}
                        onSelectVisible={(selected) =>
                          setFilteredSelection("selected", selected)
                        }
                        onSelectionChange={(portalId, selected) =>
                          setPortalSelection("selected", portalId, selected)
                        }
                      />
                    </div>

                    {portalsInvalid && (
                      <p className="mt-2 px-1 text-xs text-danger" role="alert">
                        {t("validation.portals")}
                      </p>
                    )}
                  </section>
                </div>
              </ModalBody>
              <ModalFooter className="pt-0">
                <Button
                  isDisabled={submitting}
                  variant="light"
                  onPress={onClose}
                >
                  {t("actions.cancel")}
                </Button>
                <Button
                  color="primary"
                  isLoading={submitting}
                  startContent={
                    !submitting ? (
                      <Icon icon="lucide:save" width={17} />
                    ) : undefined
                  }
                  type="submit"
                >
                  {t(editing ? "actions.save" : "actions.create")}
                </Button>
              </ModalFooter>
            </form>
            <div
              ref={setExpirationPortal}
              className="pointer-events-none fixed inset-0 z-[60] [&>*]:pointer-events-auto"
            />
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
