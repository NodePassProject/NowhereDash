import AnywhereImportModal from "@/components/ui/anywhere-import-modal";

interface PortalVectorQrModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  vectorUrl: string | null;
}

export default function PortalVectorQrModal({
  isOpen,
  onOpenChange,
  vectorUrl,
}: PortalVectorQrModalProps) {
  return (
    <AnywhereImportModal
      importUrl={vectorUrl ?? ""}
      isOpen={isOpen}
      kind="vector"
      onOpenChange={onOpenChange}
    />
  );
}
