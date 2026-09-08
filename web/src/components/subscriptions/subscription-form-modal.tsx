import {
  Button,
  Checkbox,
  Chip,
  DatePicker,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  ScrollShadow,
  Textarea,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { parseDateTime } from "@internationalized/date";
import {
  closestCenter,
  DndContext,
  type DragEndEvent,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  arrayMove,
  sortableKeyboardCoordinates,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
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
import { ConfirmationModal } from "@/components/ui/confirmation-modal";
import {
  createSubscription,
  PortalOption,
  PortalSubscription,
  SubscriptionNodeOrderItem,
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
  tunnelNames: Record<number, string>;
  externalUriText: string;
  nodeOrder: SubscriptionNodeOrderItem[];
}

interface ExternalUriIssue {
  line: number;
  type: "duplicate" | "format" | "unsupported" | "tooLong";
  value?: string;
}

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

interface PortalPickerListProps {
  emptyLabel: string;
  noResultsLabel: string;
  portals: PortalOption[];
  query: string;
  searchPlaceholder: string;
  selectAllLabel: string;
  selection: Set<number>;
  statusLabels: Record<PortalOption["status"], string>;
  totalCount: number;
  onQueryChange: (value: string) => void;
  onSelectVisible: (selected: boolean) => void;
  onSelectionChange: (portalId: number, selected: boolean) => void;
}

type NodeImportMode = "portal" | "url" | null;

type NodeNameTarget =
  | { source: "portal"; portalId: number; name: string }
  | { source: "external"; index: number; name: string };

type SubscriptionNodeRow =
  | {
      key: string;
      source: "portal";
      portal: PortalOption;
      name: string;
      protocol: string;
      url: string;
    }
  | {
      key: string;
      source: "external";
      index: number;
      name: string;
      protocol: string;
      url: string;
    };

const nodeOrderKey = (item: SubscriptionNodeOrderItem) =>
  item.source === "portal" ? `portal-${item.tunnelId}` : `external-${item.uri}`;

const GIB = 1024 ** 3;

const INITIAL_FORM: SubscriptionFormState = {
  icon: null,
  name: "",
  expiresAt: "",
  trafficLimitGiB: "",
  tunnelIds: [],
  tunnelNames: {},
  externalUriText: "",
  nodeOrder: [],
};

const SUPPORTED_URI_SCHEMES = new Set([
  "nowhere",
  "vless",
  "hysteria2",
  "hy2",
  "trojan",
  "anytls",
  "ss",
  "socks5",
  "socks",
  "sudoku",
]);

const PORTAL_STATUS_COLORS: Record<PortalOption["status"], string> = {
  running: "bg-success",
  stopped: "bg-warning",
  error: "bg-danger",
  offline: "bg-default-300",
};

const inspectExternalUris = (input: string) => {
  const entries = input.split(/\r?\n/).map((value, index) => ({
    line: index + 1,
    value: value.trim(),
  }));
  const values: string[] = [];
  const issues: ExternalUriIssue[] = [];
  const schemes = new Map<string, number>();
  const seen = new Set<string>();

  entries.forEach(({ line, value }) => {
    if (!value) return;

    values.push(value);
    if (new TextEncoder().encode(value).length > 8192) {
      issues.push({ line, type: "tooLong" });

      return;
    }
    if (seen.has(value)) {
      issues.push({ line, type: "duplicate" });

      return;
    }
    seen.add(value);

    const match = /^([a-z0-9]+):\/\/(.+)$/.exec(value);

    if (!match) {
      issues.push({ line, type: "format" });

      return;
    }
    const scheme = match[1];

    if (!SUPPORTED_URI_SCHEMES.has(scheme)) {
      issues.push({ line, type: "unsupported", value: scheme });

      return;
    }
    schemes.set(scheme, (schemes.get(scheme) ?? 0) + 1);
  });

  if (values.length > 500) {
    issues.push({ line: 501, type: "tooLong" });
  }

  return { values, issues, schemes };
};

const externalUriScheme = (uri: string) =>
  uri.slice(0, Math.max(0, uri.indexOf(":"))).toLowerCase();

const decodeNodeName = (uri: string, index: number) => {
  const fragmentIndex = uri.indexOf("#");

  if (fragmentIndex >= 0 && fragmentIndex < uri.length - 1) {
    const fragment = uri.slice(fragmentIndex + 1);

    try {
      const decoded = decodeURIComponent(fragment).trim();

      if (decoded) return decoded;
    } catch {
      if (fragment.trim()) return fragment.trim();
    }
  }

  const scheme = externalUriScheme(uri).toUpperCase();

  return `${scheme || "URL"} ${index + 1}`;
};

const replaceNodeName = (uri: string, name: string) => {
  const fragmentIndex = uri.indexOf("#");
  const base = fragmentIndex >= 0 ? uri.slice(0, fragmentIndex) : uri;

  return `${base}#${encodeURIComponent(name.trim())}`;
};

const formatHost = (host: string) => {
  const value = host.trim().replace(/^\[|\]$/g, "");

  return value.includes(":") ? `[${value}]` : value;
};

const portalUrlPreview = (
  portal: PortalOption,
  name: string,
  subscription?: PortalSubscription | null,
) => {
  const host = portal.portalHost || portal.listenHost || "*";
  const credential = portal.sharedKey?.trim() || "...";
  const network = portal.network?.toLowerCase();
  const preferences = subscription?.preferences;
  const up =
    network === "tcp" || network === "udp"
      ? network
      : preferences?.upCarrier || "tcp";
  const down =
    network === "tcp" || network === "udp"
      ? network
      : preferences?.downCarrier || "tcp";
  const query = new URLSearchParams({ up, down });

  if (up === "tcp" || down === "tcp") query.set("mux", "1");
  if (portal.alpn) query.set("alpn", portal.alpn);

  return `nowhere://${encodeURIComponent(credential)}@${formatHost(host)}:${portal.listenPort}?${query.toString()}#${encodeURIComponent(name)}`;
};

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

function PortalPickerList({
  emptyLabel,
  noResultsLabel,
  portals,
  query,
  searchPlaceholder,
  selectAllLabel,
  selection,
  statusLabels,
  totalCount,
  onQueryChange,
  onSelectVisible,
  onSelectionChange,
}: PortalPickerListProps) {
  const allVisibleSelected =
    portals.length > 0 && portals.every((portal) => selection.has(portal.id));
  const someVisibleSelected = portals.some((portal) =>
    selection.has(portal.id),
  );

  return (
    <div className="flex h-[22rem] min-w-0 flex-col overflow-hidden rounded-medium border border-default-200 bg-content1">
      <div className="flex min-h-12 items-center gap-2 border-b border-default-100 px-3">
        <Checkbox
          aria-label={selectAllLabel}
          isIndeterminate={someVisibleSelected && !allVisibleSelected}
          isSelected={allVisibleSelected}
          size="sm"
          onValueChange={onSelectVisible}
        />
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
                  base: "m-0 flex w-full max-w-none items-center rounded-small px-2 py-2.5 transition-colors hover:bg-default-100 data-[selected=true]:bg-primary-50 dark:data-[selected=true]:bg-primary-100/10",
                  label: "min-w-0 flex-1",
                  wrapper: "me-2 shrink-0",
                }}
                isSelected={selection.has(portal.id)}
                size="sm"
                onValueChange={(selected) =>
                  onSelectionChange(portal.id, selected)
                }
              >
                <div className="flex min-w-0 items-center justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-foreground-700">
                      {portal.name}
                    </p>
                    <p className="truncate font-mono text-xs text-default-400">
                      {portal.portalHost || portal.listenHost || "*"}:
                      {portal.listenPort}
                    </p>
                  </div>
                  <span className="flex shrink-0 items-center gap-1.5 text-xs text-default-500">
                    <span
                      className={`size-2 rounded-full ${PORTAL_STATUS_COLORS[portal.status]}`}
                    />
                    {statusLabels[portal.status]}
                  </span>
                </div>
              </Checkbox>
            ))}
          </div>
        ) : (
          <div className="flex h-full min-h-36 flex-col items-center justify-center px-4 text-center">
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

function SortableNodeRow({
  row,
  deleteLabel,
  editLabel,
  reorderLabel,
  onEdit,
  onRemove,
}: {
  row: SubscriptionNodeRow;
  deleteLabel: string;
  editLabel: string;
  reorderLabel: string;
  onEdit: (row: SubscriptionNodeRow) => void;
  onRemove: (row: SubscriptionNodeRow) => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: row.key });
  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.55 : 1,
    position: "relative" as const,
    zIndex: isDragging ? 10 : 0,
  };

  return (
    <tr
      ref={setNodeRef}
      className="transition-colors hover:bg-default-50"
      style={style}
    >
      <td className="px-3 py-3">
        <div className="flex min-w-0 items-center gap-1">
          <Tooltip content={reorderLabel}>
            <button
              {...attributes}
              {...listeners}
              aria-label={`${reorderLabel}: ${row.name}`}
              className="-ms-1 inline-flex size-8 shrink-0 cursor-grab touch-none items-center justify-center rounded-small text-default-400 transition-colors hover:bg-default-100 hover:text-default-600 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary active:cursor-grabbing"
              type="button"
            >
              <Icon icon="lucide:grip-vertical" width={15} />
            </button>
          </Tooltip>
          <p className="min-w-0 max-w-52 truncate text-sm font-medium text-foreground-700">
            {row.name}
          </p>
        </div>
      </td>
      <td className="w-[120px] px-3 py-3">
        <Chip
          className="font-mono text-[11px] uppercase"
          color={row.source === "portal" ? "primary" : "default"}
          size="sm"
          variant="flat"
        >
          {row.protocol}
        </Chip>
      </td>
      <td className="min-w-[300px] px-3 py-3">
        <Tooltip
          content={
            <span className="block max-w-lg break-all font-mono text-xs">
              {row.url}
            </span>
          }
          delay={500}
        >
          <span className="block max-w-[34rem] truncate font-mono text-xs text-default-500">
            {row.url}
          </span>
        </Tooltip>
      </td>
      <td className="w-[104px] px-3 py-3 text-end">
        <div className="flex items-center justify-end gap-0.5">
          <Tooltip content={editLabel}>
            <Button
              isIconOnly
              aria-label={`${editLabel}: ${row.name}`}
              color="primary"
              size="sm"
              type="button"
              variant="light"
              onPress={() => onEdit(row)}
            >
              <Icon icon="lucide:pencil" width={15} />
            </Button>
          </Tooltip>
          <Tooltip content={deleteLabel}>
            <Button
              isIconOnly
              aria-label={`${deleteLabel}: ${row.name}`}
              color="danger"
              size="sm"
              type="button"
              variant="light"
              onPress={() => onRemove(row)}
            >
              <Icon icon="lucide:trash-2" width={15} />
            </Button>
          </Tooltip>
        </div>
      </td>
    </tr>
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

  return [portal.name, portal.portalHost, portal.listenHost, portal.listenPort]
    .join(" ")
    .toLocaleLowerCase()
    .includes(normalizedQuery);
};

const toFormState = (
  subscription?: PortalSubscription | null,
): SubscriptionFormState => {
  if (!subscription) return { ...INITIAL_FORM, tunnelNames: {}, nodeOrder: [] };

  const defaultNodeOrder: SubscriptionNodeOrderItem[] = [
    ...subscription.tunnelIds.map((tunnelId) => ({
      source: "portal" as const,
      tunnelId,
    })),
    ...subscription.externalUris.map((uri) => ({
      source: "url" as const,
      uri,
    })),
  ];

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
    tunnelNames: { ...subscription.tunnelNames },
    externalUriText: subscription.externalUris.join("\n"),
    nodeOrder:
      subscription.nodeOrder?.length === defaultNodeOrder.length
        ? subscription.nodeOrder.map((item) => ({ ...item }))
        : defaultNodeOrder,
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
  const [saveConfirmationOpen, setSaveConfirmationOpen] = useState(false);
  const [externalServerError, setExternalServerError] = useState("");
  const [nodeImportMode, setNodeImportMode] = useState<NodeImportMode>(null);
  const [portalQuery, setPortalQuery] = useState("");
  const [portalSelection, setPortalSelection] = useState<Set<number>>(
    () => new Set(),
  );
  const [urlDraft, setUrlDraft] = useState("");
  const [urlImportAttempted, setUrlImportAttempted] = useState(false);
  const [nameTarget, setNameTarget] = useState<NodeNameTarget | null>(null);
  const [nameDraft, setNameDraft] = useState("");
  const [nameAttempted, setNameAttempted] = useState(false);
  const [nameError, setNameError] = useState("");
  const [expirationPortal, setExpirationPortal] =
    useState<HTMLDivElement | null>(null);
  const editing = Boolean(subscription);
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 4 },
    }),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    }),
  );

  useEffect(() => {
    if (!isOpen) return;
    setForm(toFormState(subscription));
    setIconChanged(false);
    setAttempted(false);
    setSaveConfirmationOpen(false);
    setExternalServerError("");
    setNodeImportMode(null);
    setPortalQuery("");
    setPortalSelection(new Set());
    setUrlDraft("");
    setUrlImportAttempted(false);
    setNameTarget(null);
    setNameDraft("");
    setNameAttempted(false);
    setNameError("");
  }, [isOpen, subscription]);

  const selectedIds = useMemo(() => new Set(form.tunnelIds), [form.tunnelIds]);
  const selectedPortals = useMemo(
    () => portals.filter((portal) => selectedIds.has(portal.id)),
    [portals, selectedIds],
  );
  const availablePortals = useMemo(
    () => portals.filter((portal) => !selectedIds.has(portal.id)),
    [portals, selectedIds],
  );
  const filteredAvailablePortals = useMemo(
    () =>
      availablePortals.filter((portal) =>
        matchesPortalQuery(portal, portalQuery),
      ),
    [availablePortals, portalQuery],
  );
  const externalUris = useMemo(
    () => inspectExternalUris(form.externalUriText),
    [form.externalUriText],
  );
  const draftExternalUris = useMemo(
    () => inspectExternalUris(urlDraft),
    [urlDraft],
  );
  const portalStatusLabels: Record<PortalOption["status"], string> = {
    running: t("form.tunnelStatus.running"),
    stopped: t("form.tunnelStatus.stopped"),
    error: t("form.tunnelStatus.error"),
    offline: t("form.tunnelStatus.offline"),
  };
  const tableRows = useMemo<SubscriptionNodeRow[]>(() => {
    const portalsById = new Map(
      selectedPortals.map((portal) => [portal.id, portal]),
    );
    const externalIndexes = new Map(
      externalUris.values.map((uri, index) => [uri, index]),
    );
    const rows: SubscriptionNodeRow[] = [];

    form.nodeOrder.forEach((item) => {
      if (item.source === "portal") {
        const portal = portalsById.get(item.tunnelId);

        if (!portal) return;
        const name = form.tunnelNames[portal.id]?.trim() || portal.name;

        rows.push({
          key: nodeOrderKey(item),
          source: "portal",
          portal,
          name,
          protocol: "nowhere",
          url: portalUrlPreview(portal, name, subscription),
        });

        return;
      }

      const index = externalIndexes.get(item.uri);

      if (index === undefined) return;
      rows.push({
        key: nodeOrderKey(item),
        source: "external",
        index,
        name: decodeNodeName(item.uri, index),
        protocol: externalUriScheme(item.uri),
        url: item.uri,
      });
    });

    return rows;
  }, [
    externalUris.values,
    form.nodeOrder,
    form.tunnelNames,
    selectedPortals,
    subscription,
  ]);

  const expirationValue = useMemo(
    () => parseLocalDateTime(form.expiresAt),
    [form.expiresAt],
  );

  const externalIssueMessage = (issue?: ExternalUriIssue) =>
    issue
      ? t(`validation.externalUri.${issue.type}`, {
          line: issue.line,
          scheme: issue.value,
        })
      : "";

  const duplicateImportLine = draftExternalUris.values.findIndex((uri) =>
    externalUris.values.includes(uri),
  );
  const urlImportError = urlImportAttempted
    ? !draftExternalUris.values.length
      ? t("validation.importUrlRequired")
      : externalIssueMessage(draftExternalUris.issues[0]) ||
        (duplicateImportLine >= 0
          ? t("validation.externalUri.alreadyImported", {
              line: duplicateImportLine + 1,
            })
          : externalUris.values.length + draftExternalUris.values.length > 500
            ? t("validation.externalUri.tooLong", { line: 501 })
            : "")
    : "";
  const nameInvalid = attempted && !form.name.trim();
  const externalUrisInvalid = externalUris.issues.length > 0;
  const sourcesInvalid = attempted && tableRows.length === 0;
  const trafficValue = form.trafficLimitGiB.trim()
    ? Number(form.trafficLimitGiB)
    : null;
  const trafficInvalid =
    attempted &&
    trafficValue !== null &&
    (!Number.isFinite(trafficValue) || trafficValue <= 0);
  const externalUriError =
    externalIssueMessage(externalUris.issues[0]) || externalServerError;
  const nodeNameInvalid =
    nameAttempted &&
    (!nameDraft.trim() ||
      new TextEncoder().encode(nameDraft.trim()).length > 255);

  const closeExpirationPicker = (target: Element) => {
    target.dispatchEvent(
      new KeyboardEvent("keydown", {
        bubbles: true,
        cancelable: true,
        key: "Escape",
      }),
    );
  };

  const openImporter = (mode: Exclude<NodeImportMode, null>) => {
    setNodeImportMode(mode);
    setPortalQuery("");
    setPortalSelection(new Set());
    setUrlDraft("");
    setUrlImportAttempted(false);
  };

  const togglePortalSelection = (portalId: number, selected: boolean) => {
    setPortalSelection((current) => {
      const next = new Set(current);

      if (selected) next.add(portalId);
      else next.delete(portalId);

      return next;
    });
  };

  const selectVisiblePortals = (selected: boolean) => {
    setPortalSelection((current) => {
      const next = new Set(current);

      filteredAvailablePortals.forEach((portal) => {
        if (selected) next.add(portal.id);
        else next.delete(portal.id);
      });

      return next;
    });
  };

  const importPortals = () => {
    const importing = portals
      .filter((portal) => portalSelection.has(portal.id))
      .map((portal) => portal.id);

    if (!importing.length) return;
    setForm((current) => ({
      ...current,
      tunnelIds: [...current.tunnelIds, ...importing],
      nodeOrder: [
        ...current.nodeOrder,
        ...importing.map((tunnelId) => ({
          source: "portal" as const,
          tunnelId,
        })),
      ],
    }));
    setNodeImportMode(null);
  };

  const importUrls = () => {
    setUrlImportAttempted(true);
    if (
      !draftExternalUris.values.length ||
      draftExternalUris.issues.length > 0 ||
      duplicateImportLine >= 0 ||
      externalUris.values.length + draftExternalUris.values.length > 500
    ) {
      return;
    }

    setForm((current) => ({
      ...current,
      externalUriText: [
        ...externalUris.values,
        ...draftExternalUris.values,
      ].join("\n"),
      nodeOrder: [
        ...current.nodeOrder,
        ...draftExternalUris.values.map((uri) => ({
          source: "url" as const,
          uri,
        })),
      ],
    }));
    setExternalServerError("");
    setNodeImportMode(null);
  };

  const removePortal = (portalId: number) => {
    setForm((current) => {
      const tunnelNames = { ...current.tunnelNames };

      delete tunnelNames[portalId];

      return {
        ...current,
        tunnelIds: current.tunnelIds.filter((id) => id !== portalId),
        tunnelNames,
        nodeOrder: current.nodeOrder.filter(
          (item) => item.source !== "portal" || item.tunnelId !== portalId,
        ),
      };
    });
  };

  const removeExternal = (index: number) => {
    setForm((current) => {
      const values = inspectExternalUris(current.externalUriText).values;
      const removedUri = values[index];

      return {
        ...current,
        externalUriText: values
          .filter((_, currentIndex) => currentIndex !== index)
          .join("\n"),
        nodeOrder: current.nodeOrder.filter(
          (item) => item.source !== "url" || item.uri !== removedUri,
        ),
      };
    });
    setExternalServerError("");
  };

  const handleDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return;
    setForm((current) => {
      const oldIndex = current.nodeOrder.findIndex(
        (item) => nodeOrderKey(item) === active.id,
      );
      const newIndex = current.nodeOrder.findIndex(
        (item) => nodeOrderKey(item) === over.id,
      );

      if (oldIndex < 0 || newIndex < 0) return current;

      return {
        ...current,
        nodeOrder: arrayMove(current.nodeOrder, oldIndex, newIndex),
      };
    });
  };

  const openNameEditor = (row: SubscriptionNodeRow) => {
    setNameTarget(
      row.source === "portal"
        ? { source: "portal", portalId: row.portal.id, name: row.name }
        : { source: "external", index: row.index, name: row.name },
    );
    setNameDraft(row.name);
    setNameAttempted(false);
    setNameError("");
  };

  const saveNodeName = () => {
    setNameAttempted(true);
    setNameError("");
    const name = nameDraft.trim();

    if (!name || new TextEncoder().encode(name).length > 255 || !nameTarget)
      return;

    if (nameTarget.source === "portal") {
      const portal = portals.find((item) => item.id === nameTarget.portalId);

      setForm((current) => {
        const tunnelNames = { ...current.tunnelNames };

        if (portal && name === portal.name)
          delete tunnelNames[nameTarget.portalId];
        else tunnelNames[nameTarget.portalId] = name;

        return { ...current, tunnelNames };
      });
    } else {
      const nextUris = [...externalUris.values];
      const previousUri = nextUris[nameTarget.index];
      const nextUri = replaceNodeName(previousUri, name);

      if (
        nextUris.some(
          (uri, index) => index !== nameTarget.index && uri === nextUri,
        )
      ) {
        setNameError(t("validation.nodeNameDuplicate"));

        return;
      }
      nextUris[nameTarget.index] = nextUri;
      setForm((current) => ({
        ...current,
        externalUriText: nextUris.join("\n"),
        nodeOrder: current.nodeOrder.map((item) =>
          item.source === "url" && item.uri === previousUri
            ? { ...item, uri: nextUri }
            : item,
        ),
      }));
      setExternalServerError("");
    }
    setNameTarget(null);
  };

  const saveSubscription = async () => {
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
      tunnelNames: form.tunnelNames,
      externalUris: externalUris.values,
      nodeOrder: form.nodeOrder,
      ...(iconChanged ? { icon: form.icon ?? "" } : {}),
    };

    setSubmitting(true);
    try {
      if (subscription) await updateSubscription(subscription.id, payload);
      else await createSubscription(payload);
      addToast({
        title: t(editing ? "toast.updated" : "toast.created"),
        color: "success",
      });
      setSaveConfirmationOpen(false);
      onOpenChange(false);
      await onSaved();
    } catch (error) {
      const message =
        error instanceof Error ? error.message : t("toast.saveFailed");

      if (message.includes("externalUris")) setExternalServerError(message);
      addToast({
        title: t("toast.saveFailed"),
        description: message,
        color: "danger",
      });
      setSaveConfirmationOpen(false);
    } finally {
      setSubmitting(false);
    }
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setAttempted(true);

    if (
      !form.name.trim() ||
      tableRows.length === 0 ||
      externalUrisInvalid ||
      (trafficValue !== null &&
        (!Number.isFinite(trafficValue) || trafficValue <= 0))
    ) {
      return;
    }

    if (externalUris.values.length > 0) {
      setSaveConfirmationOpen(true);

      return;
    }

    await saveSubscription();
  };

  return (
    <>
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
                              setForm((old) => ({
                                ...old,
                                trafficLimitGiB,
                              }))
                            }
                          />
                        </FormField>
                      </div>
                    </section>

                    <section className="min-w-0">
                      <div className="mb-2 flex min-h-8 items-center justify-between gap-3 px-1">
                        <div className="flex min-w-0 items-center gap-2">
                          <FieldLabel label={t("fields.nodes")} />
                          <span className="text-xs tabular-nums text-default-400">
                            {t("form.nodeCount", { count: tableRows.length })}
                          </span>
                        </div>
                        <Dropdown placement="bottom-end">
                          <DropdownTrigger>
                            <Button
                              isIconOnly
                              aria-label={t("form.addNode")}
                              color="primary"
                              size="sm"
                              type="button"
                              variant="flat"
                            >
                              <Icon icon="lucide:plus" width={17} />
                            </Button>
                          </DropdownTrigger>
                          <DropdownMenu aria-label={t("form.addNodeOptions")}>
                            <DropdownItem
                              key="portal"
                              startContent={
                                <Icon icon="lucide:waypoints" width={17} />
                              }
                              onPress={() => openImporter("portal")}
                            >
                              {t("form.importPortal")}
                            </DropdownItem>
                            <DropdownItem
                              key="url"
                              startContent={
                                <Icon icon="lucide:link" width={17} />
                              }
                              onPress={() => openImporter("url")}
                            >
                              {t("form.importUrl")}
                            </DropdownItem>
                          </DropdownMenu>
                        </Dropdown>
                      </div>

                      <div className="max-h-80 overflow-auto rounded-medium border border-default-100 bg-content1 p-2">
                        <DndContext
                          collisionDetection={closestCenter}
                          sensors={sensors}
                          onDragEnd={handleDragEnd}
                        >
                          <table
                            aria-label={t("fields.nodes")}
                            className="w-full min-w-[680px] border-separate border-spacing-0"
                          >
                            <thead>
                              <tr className="text-start text-xs font-medium text-default-500">
                                <th className="sticky top-0 z-10 w-[170px] rounded-s-medium bg-default-100 px-3 py-3 text-start font-medium">
                                  {t("fields.nodeName")}
                                </th>
                                <th className="sticky top-0 z-10 w-[120px] bg-default-100 px-3 py-3 text-start font-medium">
                                  {t("fields.protocol")}
                                </th>
                                <th className="sticky top-0 z-10 min-w-[300px] bg-default-100 px-3 py-3 text-start font-medium">
                                  {t("fields.nodeUrl")}
                                </th>
                                <th className="sticky top-0 z-10 w-[104px] rounded-e-medium bg-default-100 px-3 py-3 text-end font-medium">
                                  {t("fields.actions")}
                                </th>
                              </tr>
                            </thead>
                            <SortableContext
                              items={tableRows.map((row) => row.key)}
                              strategy={verticalListSortingStrategy}
                            >
                              <tbody className="divide-y divide-default-100">
                                {tableRows.length > 0 ? (
                                  tableRows.map((row) => (
                                    <SortableNodeRow
                                      key={row.key}
                                      deleteLabel={t("actions.delete")}
                                      editLabel={t("form.editNodeName")}
                                      reorderLabel={t("form.reorderNode")}
                                      row={row}
                                      onEdit={openNameEditor}
                                      onRemove={(target) =>
                                        target.source === "portal"
                                          ? removePortal(target.portal.id)
                                          : removeExternal(target.index)
                                      }
                                    />
                                  ))
                                ) : (
                                  <tr>
                                    <td className="px-3 py-8" colSpan={4}>
                                      <div className="flex min-h-28 flex-col items-center justify-center text-center">
                                        <span className="mb-3 flex size-10 items-center justify-center rounded-full bg-default-100 text-default-400">
                                          <Icon
                                            icon="lucide:rows-3"
                                            width={19}
                                          />
                                        </span>
                                        <p className="text-sm font-medium text-default-600">
                                          {t("form.noNodes")}
                                        </p>
                                        <p className="mt-1 text-xs text-default-400">
                                          {t("form.noNodesHint")}
                                        </p>
                                      </div>
                                    </td>
                                  </tr>
                                )}
                              </tbody>
                            </SortableContext>
                          </table>
                        </DndContext>
                      </div>
                      {(sourcesInvalid || externalUriError) && (
                        <p
                          className="mt-2 px-1 text-xs text-danger"
                          role="alert"
                        >
                          {externalUriError || t("validation.sources")}
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

      <Modal
        isOpen={nodeImportMode === "portal"}
        placement="center"
        scrollBehavior="inside"
        size="2xl"
        onOpenChange={(open) => {
          if (!open) setNodeImportMode(null);
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Icon
                  className="text-primary"
                  icon="lucide:waypoints"
                  width={18}
                />
                <span className="text-base font-semibold">
                  {t("form.portalImportTitle")}
                </span>
              </ModalHeader>
              <ModalBody>
                <PortalPickerList
                  emptyLabel={t("form.noAvailableTunnels")}
                  noResultsLabel={t("form.noTunnelResults")}
                  portals={filteredAvailablePortals}
                  query={portalQuery}
                  searchPlaceholder={t("form.searchTunnels")}
                  selectAllLabel={t("form.selectAllAvailable")}
                  selection={portalSelection}
                  statusLabels={portalStatusLabels}
                  totalCount={availablePortals.length}
                  onQueryChange={setPortalQuery}
                  onSelectVisible={selectVisiblePortals}
                  onSelectionChange={togglePortalSelection}
                />
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("actions.cancel")}
                </Button>
                <Button
                  color="primary"
                  isDisabled={portalSelection.size === 0}
                  startContent={<Icon icon="lucide:plus" width={16} />}
                  onPress={importPortals}
                >
                  {t("form.importSelectedPortals", {
                    count: portalSelection.size,
                  })}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        isOpen={nodeImportMode === "url"}
        placement="center"
        size="2xl"
        onOpenChange={(open) => {
          if (!open) setNodeImportMode(null);
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Icon className="text-primary" icon="lucide:link" width={18} />
                <span className="text-base font-semibold">
                  {t("form.urlImportTitle")}
                </span>
              </ModalHeader>
              <ModalBody>
                <Textarea
                  aria-label={t("form.urlImportTitle")}
                  classNames={{
                    input: "font-mono text-xs leading-5",
                    inputWrapper: "min-h-56 shadow-none",
                  }}
                  description={
                    !urlImportError ? t("form.externalUriHint") : undefined
                  }
                  errorMessage={urlImportError || undefined}
                  isInvalid={Boolean(urlImportError)}
                  maxRows={14}
                  minRows={9}
                  placeholder={t("form.externalUriPlaceholder")}
                  value={urlDraft}
                  onValueChange={(value) => {
                    setUrlDraft(value);
                    setUrlImportAttempted(false);
                  }}
                />
                {draftExternalUris.schemes.size > 0 && (
                  <div className="flex flex-wrap gap-1.5">
                    {Array.from(draftExternalUris.schemes.entries()).map(
                      ([scheme, count]) => (
                        <Chip
                          key={scheme}
                          className="font-mono text-[11px] uppercase"
                          color="primary"
                          size="sm"
                          variant="flat"
                        >
                          {scheme} · {count}
                        </Chip>
                      ),
                    )}
                  </div>
                )}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("actions.cancel")}
                </Button>
                <Button
                  color="primary"
                  startContent={<Icon icon="lucide:download" width={16} />}
                  onPress={importUrls}
                >
                  {t("form.importUrls", {
                    count: draftExternalUris.values.length,
                  })}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        isOpen={Boolean(nameTarget)}
        placement="center"
        size="md"
        onOpenChange={(open) => {
          if (!open) setNameTarget(null);
        }}
      >
        <ModalContent>
          {(onClose) => (
            <form
              onSubmit={(event) => {
                event.preventDefault();
                saveNodeName();
              }}
            >
              <ModalHeader className="flex items-center gap-2">
                <Icon
                  className="text-primary"
                  icon="lucide:pencil"
                  width={18}
                />
                <span className="text-base font-semibold">
                  {t("form.editNodeName")}
                </span>
              </ModalHeader>
              <ModalBody>
                <Input
                  isRequired
                  aria-label={t("fields.nodeName")}
                  description={t("form.nodeNameHint")}
                  errorMessage={
                    nameError ||
                    (nodeNameInvalid ? t("validation.nodeName") : undefined)
                  }
                  isInvalid={nodeNameInvalid || Boolean(nameError)}
                  label={t("fields.nodeName")}
                  labelPlacement="outside"
                  placeholder={t("form.nodeNamePlaceholder")}
                  value={nameDraft}
                  onValueChange={(value) => {
                    setNameDraft(value);
                    setNameAttempted(false);
                    setNameError("");
                  }}
                />
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  {t("actions.cancel")}
                </Button>
                <Button color="primary" type="submit">
                  {t("actions.save")}
                </Button>
              </ModalFooter>
            </form>
          )}
        </ModalContent>
      </Modal>

      <ConfirmationModal
        cancelText={t("actions.cancel")}
        confirmColor="warning"
        confirmText={t("confirm.externalTraffic.confirm")}
        icon="lucide:triangle-alert"
        iconColor="text-warning"
        isLoading={submitting}
        isOpen={saveConfirmationOpen}
        message={t("confirm.externalTraffic.message")}
        title={t("confirm.externalTraffic.title")}
        onClose={() => {
          if (!submitting) setSaveConfirmationOpen(false);
        }}
        onConfirm={() => void saveSubscription()}
      />
    </>
  );
}
