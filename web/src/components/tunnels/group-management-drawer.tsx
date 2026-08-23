import {
  Button,
  Chip,
  Drawer,
  DrawerBody,
  DrawerContent,
  DrawerFooter,
  DrawerHeader,
  Input,
  Spinner,
  Table,
  TableBody,
  TableCell,
  TableColumn,
  TableHeader,
  TableRow,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";

import { ConfirmationModal } from "@/components/ui/confirmation-modal";
import { buildApiUrl } from "@/lib/utils";

interface PortalGroup {
  id: number;
  name: string;
  createdAt: string;
  updatedAt: string;
  tunnelIds?: number[];
}

interface GroupResponse {
  success?: boolean;
  message?: string;
  error?: string;
  group?: PortalGroup;
  groups?: PortalGroup[];
}

interface GroupManagementDrawerProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void | Promise<void>;
}

const COPY = {
  zh: {
    title: "隧道分组管理",
    eyebrow: "Nowhere 隧道",
    add: "新增分组",
    name: "分组名称",
    namePlaceholder: "输入分组名称",
    portalCount: "隧道数量",
    actions: "操作",
    save: "保存",
    cancel: "取消",
    close: "关闭",
    rename: "重命名",
    remove: "删除分组",
    empty: "尚无隧道分组",
    emptyDetail: "新增分组后即可整理隧道。",
    portals: (count: number) => `${count} 个`,
    deleteTitle: "删除隧道分组",
    deleteMessage: (name: string) =>
      `确定删除“${name}”吗？分组关系会被清除，关联的 Nowhere 隧道不会被删除。`,
    deleteConfirm: "删除",
    nameRequired: "请输入分组名称",
    loadFailed: "加载隧道分组失败",
    createSuccess: "隧道分组已创建",
    createFailed: "创建隧道分组失败",
    updateSuccess: "隧道分组已重命名",
    updateFailed: "重命名隧道分组失败",
    deleteSuccess: "隧道分组已删除",
    deleteFailed: "删除隧道分组失败",
  },
  en: {
    title: "Tunnel Groups",
    eyebrow: "Nowhere Tunnel",
    add: "New group",
    name: "Group name",
    namePlaceholder: "Enter a group name",
    portalCount: "Tunnels",
    actions: "Actions",
    save: "Save",
    cancel: "Cancel",
    close: "Close",
    rename: "Rename",
    remove: "Delete group",
    empty: "No Tunnel groups",
    emptyDetail: "Create a group to organize Tunnels.",
    portals: (count: number) => `${count}`,
    deleteTitle: "Delete Tunnel group",
    deleteMessage: (name: string) =>
      `Delete “${name}”? Its group assignments will be cleared, but the associated Nowhere Tunnels will remain.`,
    deleteConfirm: "Delete",
    nameRequired: "Enter a group name",
    loadFailed: "Failed to load Tunnel groups",
    createSuccess: "Tunnel group created",
    createFailed: "Failed to create Tunnel group",
    updateSuccess: "Tunnel group renamed",
    updateFailed: "Failed to rename Tunnel group",
    deleteSuccess: "Tunnel group deleted",
    deleteFailed: "Failed to delete Tunnel group",
  },
};

const readResponse = async (response: Response): Promise<GroupResponse> => {
  const body = (await response.json().catch(() => ({}))) as GroupResponse;

  if (!response.ok || body.success === false) {
    throw new Error(body.error || body.message || response.statusText);
  }

  return body;
};

export default function GroupManagementDrawer({
  isOpen,
  onOpenChange,
  onSaved,
}: GroupManagementDrawerProps) {
  const { i18n } = useTranslation();
  const copy = useMemo(
    () =>
      (i18n.resolvedLanguage || i18n.language).toLowerCase().startsWith("zh")
        ? COPY.zh
        : COPY.en,
    [i18n.language, i18n.resolvedLanguage],
  );
  const [groups, setGroups] = useState<PortalGroup[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeAction, setActiveAction] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState("");
  const [editingId, setEditingId] = useState<number | null>(null);
  const [editName, setEditName] = useState("");
  const [deleteTarget, setDeleteTarget] = useState<PortalGroup | null>(null);

  const busy = activeAction !== null;

  const loadGroups = useCallback(async () => {
    setLoading(true);
    try {
      const response = await fetch(buildApiUrl("/api/groups"));
      const body = await readResponse(response);

      setGroups(Array.isArray(body.groups) ? body.groups : []);
    } catch (error) {
      addToast({
        title: copy.loadFailed,
        description: error instanceof Error ? error.message : copy.loadFailed,
        color: "danger",
      });
    } finally {
      setLoading(false);
    }
  }, [copy.loadFailed]);

  useEffect(() => {
    if (isOpen) void loadGroups();
  }, [isOpen, loadGroups]);

  const refreshAfterSave = async () => {
    await loadGroups();
    await onSaved();
  };

  const createGroup = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const name = newName.trim();

    if (!name) {
      addToast({ title: copy.nameRequired, color: "warning" });

      return;
    }

    setActiveAction("create");
    try {
      const response = await fetch(buildApiUrl("/api/groups"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });

      await readResponse(response);
      setNewName("");
      setShowCreate(false);
      addToast({ title: copy.createSuccess, color: "success" });
      await refreshAfterSave();
    } catch (error) {
      addToast({
        title: copy.createFailed,
        description: error instanceof Error ? error.message : copy.createFailed,
        color: "danger",
      });
    } finally {
      setActiveAction(null);
    }
  };

  const startEditing = (group: PortalGroup) => {
    setEditingId(group.id);
    setEditName(group.name);
    setShowCreate(false);
    setNewName("");
  };

  const cancelEditing = () => {
    setEditingId(null);
    setEditName("");
  };

  const renameGroup = async (
    event: FormEvent<HTMLFormElement>,
    group: PortalGroup,
  ) => {
    event.preventDefault();
    const name = editName.trim();

    if (!name) {
      addToast({ title: copy.nameRequired, color: "warning" });

      return;
    }
    if (name === group.name) {
      cancelEditing();

      return;
    }

    setActiveAction(`rename-${group.id}`);
    try {
      const response = await fetch(buildApiUrl(`/api/groups/${group.id}`), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name }),
      });

      await readResponse(response);
      cancelEditing();
      addToast({ title: copy.updateSuccess, color: "success" });
      await refreshAfterSave();
    } catch (error) {
      addToast({
        title: copy.updateFailed,
        description: error instanceof Error ? error.message : copy.updateFailed,
        color: "danger",
      });
    } finally {
      setActiveAction(null);
    }
  };

  const deleteGroup = async () => {
    if (!deleteTarget) return;
    const target = deleteTarget;

    setActiveAction(`delete-${target.id}`);
    try {
      const response = await fetch(buildApiUrl(`/api/groups/${target.id}`), {
        method: "DELETE",
      });

      await readResponse(response);
      setDeleteTarget(null);
      if (editingId === target.id) cancelEditing();
      addToast({ title: copy.deleteSuccess, color: "success" });
      await refreshAfterSave();
    } catch (error) {
      addToast({
        title: copy.deleteFailed,
        description: error instanceof Error ? error.message : copy.deleteFailed,
        color: "danger",
      });
    } finally {
      setActiveAction(null);
    }
  };

  const resetTransientState = () => {
    setShowCreate(false);
    setNewName("");
    cancelEditing();
    setDeleteTarget(null);
  };

  const handleOpenChange = (open: boolean) => {
    if (!open) resetTransientState();
    onOpenChange(open);
  };

  return (
    <>
      <Drawer
        hideCloseButton
        isOpen={isOpen}
        placement="right"
        size="lg"
        onOpenChange={handleOpenChange}
      >
        <DrawerContent>
          {(onClose) => (
            <>
              <DrawerHeader className="border-b border-default-200 px-5 py-4">
                <div className="flex w-full min-w-0 items-center justify-between gap-3">
                  <div className="flex min-w-0 items-center gap-3">
                    <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary-50 text-primary dark:bg-primary-900/30">
                      <Icon icon="lucide:tags" width={19} />
                    </span>
                    <div className="min-w-0">
                      <p className="truncate text-base font-semibold text-foreground">
                        {copy.title}
                      </p>
                      <p className="mt-0.5 text-xs font-normal text-default-500">
                        {copy.eyebrow}
                      </p>
                    </div>
                  </div>
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      color="primary"
                      isDisabled={busy || showCreate}
                      size="sm"
                      startContent={<Icon icon="lucide:plus" width={16} />}
                      variant="flat"
                      onPress={() => {
                        cancelEditing();
                        setShowCreate(true);
                      }}
                    >
                      {copy.add}
                    </Button>
                    <Tooltip content={copy.close}>
                      <Button
                        isIconOnly
                        aria-label={copy.close}
                        isDisabled={busy}
                        size="sm"
                        variant="light"
                        onPress={onClose}
                      >
                        <Icon icon="lucide:x" width={18} />
                      </Button>
                    </Tooltip>
                  </div>
                </div>
              </DrawerHeader>

              <DrawerBody className="gap-4 px-5 py-5">
                {showCreate && (
                  <form
                    className="rounded-lg border border-default-200 bg-default-50 p-4 dark:bg-default-100/40"
                    onSubmit={createGroup}
                  >
                    <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
                      <Input
                        className="min-w-0 flex-1"
                        isDisabled={busy}
                        label={copy.name}
                        maxLength={80}
                        placeholder={copy.namePlaceholder}
                        value={newName}
                        onValueChange={setNewName}
                      />
                      <div className="flex shrink-0 items-center gap-2">
                        <Button
                          color="primary"
                          isDisabled={!newName.trim()}
                          isLoading={activeAction === "create"}
                          startContent={
                            activeAction !== "create" ? (
                              <Icon icon="lucide:check" width={16} />
                            ) : undefined
                          }
                          type="submit"
                        >
                          {copy.save}
                        </Button>
                        <Button
                          isIconOnly
                          aria-label={copy.cancel}
                          isDisabled={busy}
                          variant="light"
                          onPress={() => {
                            setShowCreate(false);
                            setNewName("");
                          }}
                        >
                          <Icon icon="lucide:x" width={18} />
                        </Button>
                      </div>
                    </div>
                  </form>
                )}

                {loading ? (
                  <div className="flex min-h-56 items-center justify-center">
                    <Spinner color="primary" />
                  </div>
                ) : (
                  <Table
                    removeWrapper
                    aria-label={copy.title}
                    classNames={{
                      th: "bg-default-100 text-xs font-semibold text-default-500",
                      td: "border-b border-default-100 py-3",
                    }}
                  >
                    <TableHeader>
                      <TableColumn>{copy.name}</TableColumn>
                      <TableColumn width={110}>{copy.portalCount}</TableColumn>
                      <TableColumn align="end" width={112}>
                        {copy.actions}
                      </TableColumn>
                    </TableHeader>
                    <TableBody
                      emptyContent={
                        <div className="flex min-h-52 flex-col items-center justify-center gap-3 text-center">
                          <span className="flex size-12 items-center justify-center rounded-lg bg-default-100 text-default-400">
                            <Icon icon="lucide:tags" width={22} />
                          </span>
                          <div>
                            <p className="text-sm font-medium text-default-600">
                              {copy.empty}
                            </p>
                            <p className="mt-1 text-xs text-default-400">
                              {copy.emptyDetail}
                            </p>
                          </div>
                        </div>
                      }
                      items={groups}
                    >
                      {(group) => (
                        <TableRow key={group.id}>
                          <TableCell>
                            {editingId === group.id ? (
                              <form
                                className="flex min-w-[180px] items-center gap-1"
                                onSubmit={(event) => renameGroup(event, group)}
                              >
                                <Input
                                  aria-label={copy.name}
                                  isDisabled={busy}
                                  maxLength={80}
                                  size="sm"
                                  value={editName}
                                  onValueChange={setEditName}
                                />
                                <Tooltip content={copy.save}>
                                  <Button
                                    isIconOnly
                                    aria-label={copy.save}
                                    color="primary"
                                    isDisabled={!editName.trim()}
                                    isLoading={
                                      activeAction === `rename-${group.id}`
                                    }
                                    size="sm"
                                    type="submit"
                                    variant="light"
                                  >
                                    <Icon icon="lucide:check" width={16} />
                                  </Button>
                                </Tooltip>
                                <Tooltip content={copy.cancel}>
                                  <Button
                                    isIconOnly
                                    aria-label={copy.cancel}
                                    isDisabled={busy}
                                    size="sm"
                                    variant="light"
                                    onPress={cancelEditing}
                                  >
                                    <Icon icon="lucide:x" width={16} />
                                  </Button>
                                </Tooltip>
                              </form>
                            ) : (
                              <div className="flex min-w-0 items-center gap-2.5">
                                <Icon
                                  className="shrink-0 text-default-400"
                                  icon="lucide:folder"
                                  width={17}
                                />
                                <span className="truncate text-sm font-medium text-foreground">
                                  {group.name}
                                </span>
                              </div>
                            )}
                          </TableCell>
                          <TableCell>
                            <Chip size="sm" variant="flat">
                              {copy.portals(group.tunnelIds?.length ?? 0)}
                            </Chip>
                          </TableCell>
                          <TableCell>
                            <div className="flex justify-end gap-1">
                              <Tooltip content={copy.rename}>
                                <Button
                                  isIconOnly
                                  aria-label={`${copy.rename}: ${group.name}`}
                                  isDisabled={busy || editingId === group.id}
                                  size="sm"
                                  variant="light"
                                  onPress={() => startEditing(group)}
                                >
                                  <Icon icon="lucide:pencil" width={16} />
                                </Button>
                              </Tooltip>
                              <Tooltip color="danger" content={copy.remove}>
                                <Button
                                  isIconOnly
                                  aria-label={`${copy.remove}: ${group.name}`}
                                  color="danger"
                                  isDisabled={busy}
                                  size="sm"
                                  variant="light"
                                  onPress={() => setDeleteTarget(group)}
                                >
                                  <Icon icon="lucide:trash-2" width={16} />
                                </Button>
                              </Tooltip>
                            </div>
                          </TableCell>
                        </TableRow>
                      )}
                    </TableBody>
                  </Table>
                )}
              </DrawerBody>

              <DrawerFooter className="border-t border-default-200 px-5 py-3">
                <Button isDisabled={busy} variant="flat" onPress={onClose}>
                  {copy.close}
                </Button>
              </DrawerFooter>
            </>
          )}
        </DrawerContent>
      </Drawer>

      <ConfirmationModal
        confirmColor="danger"
        confirmText={copy.deleteConfirm}
        icon="lucide:trash-2"
        iconColor="text-danger"
        isLoading={
          deleteTarget ? activeAction === `delete-${deleteTarget.id}` : false
        }
        isOpen={Boolean(deleteTarget)}
        message={deleteTarget ? copy.deleteMessage(deleteTarget.name) : ""}
        title={copy.deleteTitle}
        onClose={() => {
          if (!busy) setDeleteTarget(null);
        }}
        onConfirm={() => void deleteGroup()}
      />
    </>
  );
}
