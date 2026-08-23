import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Select,
  SelectItem,
  Switch,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";

import AnywhereImportModal from "@/components/ui/anywhere-import-modal";
import {
  type PortalSubscription,
  type SubscriptionCarrier,
  toSubscriptionPayload,
  updateSubscription,
} from "@/lib/subscriptions-api";

interface SubscriptionImportModalProps {
  isOpen: boolean;
  subscription: PortalSubscription | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void | Promise<void>;
}

interface OutputPreferencesState {
  downCarrier: SubscriptionCarrier;
  expandCarrierCombos: boolean;
  upCarrier: SubscriptionCarrier;
}

const DEFAULT_OUTPUT_PREFERENCES: OutputPreferencesState = {
  downCarrier: "tcp",
  expandCarrierCombos: true,
  upCarrier: "tcp",
};

const getOutputPreferences = (
  subscription: PortalSubscription | null,
): OutputPreferencesState => {
  if (!subscription) return DEFAULT_OUTPUT_PREFERENCES;

  return {
    downCarrier: subscription.preferences.downCarrier,
    expandCarrierCombos: subscription.preferences.expandCarrierCombos,
    upCarrier: subscription.preferences.upCarrier,
  };
};

export default function SubscriptionImportModal({
  isOpen,
  subscription,
  onOpenChange,
  onSaved,
}: SubscriptionImportModalProps) {
  const { t } = useTranslation("subscriptions");
  const [preferences, setPreferences] = useState<OutputPreferencesState>(
    DEFAULT_OUTPUT_PREFERENCES,
  );
  const [savedPreferences, setSavedPreferences] =
    useState<OutputPreferencesState>(DEFAULT_OUTPUT_PREFERENCES);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!isOpen) return;

    const nextPreferences = getOutputPreferences(subscription);

    setPreferences(nextPreferences);
    setSavedPreferences(nextPreferences);
    setSaving(false);
  }, [isOpen, subscription]);

  const preferencesDirty =
    preferences.downCarrier !== savedPreferences.downCarrier ||
    preferences.expandCarrierCombos !== savedPreferences.expandCarrierCombos ||
    preferences.upCarrier !== savedPreferences.upCarrier;

  const savePreferences = async () => {
    if (!subscription || !preferencesDirty) return;

    setSaving(true);
    try {
      await updateSubscription(
        subscription.id,
        toSubscriptionPayload(subscription, {
          preferences: { ...preferences, includeIpv6: false },
        }),
      );
      setSavedPreferences(preferences);
      addToast({ title: t("import.preferencesSaved"), color: "success" });
      await onSaved();
    } catch (error) {
      addToast({
        title: t("toast.saveFailed"),
        description:
          error instanceof Error ? error.message : t("toast.saveFailed"),
        color: "danger",
      });
    } finally {
      setSaving(false);
    }
  };

  const outputPreferences = (
    <section className="w-full space-y-4 p-3">
      <div>
        <h3 className="text-sm font-semibold text-foreground-700">
          {t("import.outputPreferences")}
        </h3>
        <p className="mt-1 text-xs leading-5 text-default-500">
          {t("import.outputPreferencesHint")}
        </p>
      </div>

      <div className="flex min-h-[4.25rem] items-center justify-between gap-4 rounded-medium bg-default-100/60 px-3 py-2">
        <span className="text-sm text-foreground-600">
          {t("fields.expandCarrierCombos")}
        </span>
        <Switch
          aria-label={t("fields.expandCarrierCombos")}
          className="shrink-0"
          isSelected={preferences.expandCarrierCombos}
          onValueChange={(expandCarrierCombos) =>
            setPreferences((current) => ({
              ...current,
              expandCarrierCombos,
            }))
          }
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div className="min-w-0 space-y-1">
          <p className="px-1 text-sm text-foreground-600">
            {t("fields.upCarrier")}
          </p>
          <Select
            aria-label={t("fields.upCarrier")}
            isDisabled={preferences.expandCarrierCombos}
            selectedKeys={new Set([preferences.upCarrier])}
            onSelectionChange={(keys) =>
              setPreferences((current) => ({
                ...current,
                upCarrier: String(
                  Array.from(keys)[0] ?? "tcp",
                ) as SubscriptionCarrier,
              }))
            }
          >
            <SelectItem key="tcp">TCP</SelectItem>
            <SelectItem key="udp">UDP</SelectItem>
          </Select>
        </div>
        <div className="min-w-0 space-y-1">
          <p className="px-1 text-sm text-foreground-600">
            {t("fields.downCarrier")}
          </p>
          <Select
            aria-label={t("fields.downCarrier")}
            isDisabled={preferences.expandCarrierCombos}
            selectedKeys={new Set([preferences.downCarrier])}
            onSelectionChange={(keys) =>
              setPreferences((current) => ({
                ...current,
                downCarrier: String(
                  Array.from(keys)[0] ?? "tcp",
                ) as SubscriptionCarrier,
              }))
            }
          >
            <SelectItem key="tcp">TCP</SelectItem>
            <SelectItem key="udp">UDP</SelectItem>
          </Select>
        </div>
      </div>

      <Button
        className="w-full"
        color="primary"
        isDisabled={!preferencesDirty}
        isLoading={saving}
        startContent={
          !saving ? <Icon icon="lucide:save" width={16} /> : undefined
        }
        onPress={() => void savePreferences()}
      >
        {t("import.savePreferences")}
      </Button>
    </section>
  );

  const outputPreferencesAction = (
    <Popover offset={8} placement="bottom-start">
      <PopoverTrigger>
        <Button
          isIconOnly
          aria-label={t("import.outputPreferences")}
          className="size-8 min-w-8"
          size="sm"
          variant="light"
        >
          <Tooltip content={t("import.outputPreferences")} placement="top">
            <span className="inline-flex items-center justify-center">
              <Icon icon="lucide:settings-2" width={17} />
            </span>
          </Tooltip>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[300px] max-w-[calc(100vw-2rem)] p-0">
        {outputPreferences}
      </PopoverContent>
    </Popover>
  );

  return (
    <AnywhereImportModal
      headerAction={outputPreferencesAction}
      importUrl={subscription?.subscriptionUrl ?? ""}
      isOpen={isOpen}
      kind="subscription"
      onOpenChange={onOpenChange}
    />
  );
}
