import type { Dispatch, SetStateAction } from "react";

import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@heroui/react";
import { useTranslation } from "react-i18next";

export interface EndpointConfigForm {
  name: string;
  url: string;
  apiKey: string;
  hostname: string;
}

interface EditEndpointConfigModalProps {
  configForm: EndpointConfigForm;
  isOpen: boolean;
  onOpenChange: () => void;
  onSubmit: () => void;
  setConfigForm: Dispatch<SetStateAction<EndpointConfigForm>>;
}

export default function EditEndpointConfigModal({
  configForm,
  isOpen,
  onOpenChange,
  onSubmit,
  setConfigForm,
}: EditEndpointConfigModalProps) {
  const { t } = useTranslation("endpoints");

  return (
    <Modal
      isOpen={isOpen}
      placement="center"
      size="lg"
      onOpenChange={onOpenChange}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader>{t("details.modals.editConfig.title")}</ModalHeader>
            <ModalBody>
              <div className="space-y-4">
                <p className="text-sm text-warning-600">
                  {t("details.modals.editConfig.warning")}
                </p>

                <Input
                  isRequired
                  endContent={
                    <span className="text-xs text-default-500">
                      {configForm.name.length}/25
                    </span>
                  }
                  label={t("details.modals.editConfig.nameLabel")}
                  maxLength={25}
                  placeholder={t("details.modals.editConfig.namePlaceholder")}
                  value={configForm.name}
                  onValueChange={(value) =>
                    setConfigForm((prev) => ({ ...prev, name: value }))
                  }
                />

                <Input
                  isRequired
                  label={t("details.modals.editConfig.urlLabel")}
                  placeholder={t("details.modals.editConfig.urlPlaceholder")}
                  type="url"
                  value={configForm.url}
                  onValueChange={(value) =>
                    setConfigForm((prev) => ({ ...prev, url: value }))
                  }
                />

                <Input
                  description={t("details.modals.editConfig.apiKeyDescription")}
                  label={t("details.modals.editConfig.apiKeyLabel")}
                  placeholder={t("details.modals.editConfig.apiKeyPlaceholder")}
                  type="password"
                  value={configForm.apiKey}
                  onValueChange={(value) =>
                    setConfigForm((prev) => ({ ...prev, apiKey: value }))
                  }
                />

                <Input
                  description={t(
                    "details.modals.editConfig.hostnameDescription",
                  )}
                  label={t("details.modals.editConfig.hostnameLabel")}
                  placeholder={t(
                    "details.modals.editConfig.hostnamePlaceholder",
                  )}
                  value={configForm.hostname}
                  onValueChange={(value) =>
                    setConfigForm((prev) => ({ ...prev, hostname: value }))
                  }
                />
              </div>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={onClose}>
                {t("details.modals.editConfig.cancel")}
              </Button>
              <Button color="warning" onPress={onSubmit}>
                {t("details.modals.editConfig.confirm")}
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
