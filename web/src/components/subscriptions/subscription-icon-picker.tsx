import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Tooltip,
} from "@heroui/react";
import { addToast } from "@heroui/toast";
import { Icon } from "@iconify/react/dist/offline";
import {
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useTranslation } from "react-i18next";

const DEFAULT_ICON_URL = "/nowhere-icon.png";
const ICON_SIZE = 96;
const PREVIEW_SIZE = 280;
const MAX_INPUT_SIZE = 5 * 1024 * 1024;
const MAX_OUTPUT_SIZE = 32 * 1024;

interface CropSource {
  height: number;
  name: string;
  url: string;
  width: number;
}

interface SubscriptionIconPickerProps {
  value: string | null;
  onChange: (value: string | null) => void;
}

interface Point {
  x: number;
  y: number;
}

interface DragState extends Point {
  pointerId: number;
  startX: number;
  startY: number;
}

const clamp = (value: number, minimum: number, maximum: number) =>
  Math.min(maximum, Math.max(minimum, value));

const loadImage = (url: string) =>
  new Promise<HTMLImageElement>((resolve, reject) => {
    const image = new Image();

    image.onload = () => resolve(image);
    image.onerror = () => reject(new Error("Unable to load image"));
    image.src = url;
  });

const canvasToBlob = (canvas: HTMLCanvasElement) =>
  new Promise<Blob>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (blob) resolve(blob);
      else reject(new Error("Unable to encode image"));
    }, "image/png");
  });

const blobToDataURL = (blob: Blob) =>
  new Promise<string>((resolve, reject) => {
    const reader = new FileReader();

    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("Unable to read image"));
    reader.readAsDataURL(blob);
  });

export default function SubscriptionIconPicker({
  value,
  onChange,
}: SubscriptionIconPickerProps) {
  const { t } = useTranslation("subscriptions");
  const inputRef = useRef<HTMLInputElement>(null);
  const dragRef = useRef<DragState | null>(null);
  const [source, setSource] = useState<CropSource | null>(null);
  const [zoom, setZoom] = useState(1);
  const [offset, setOffset] = useState<Point>({ x: 0, y: 0 });
  const [processing, setProcessing] = useState(false);

  useEffect(() => {
    return () => {
      if (source) URL.revokeObjectURL(source.url);
    };
  }, [source]);

  const geometry = useMemo(() => {
    if (!source) return null;

    const baseScale = Math.max(
      PREVIEW_SIZE / source.width,
      PREVIEW_SIZE / source.height,
    );
    const scale = baseScale * zoom;
    const displayWidth = source.width * scale;
    const displayHeight = source.height * scale;

    return {
      displayHeight,
      displayWidth,
      maxX: Math.max(0, (displayWidth - PREVIEW_SIZE) / 2),
      maxY: Math.max(0, (displayHeight - PREVIEW_SIZE) / 2),
      scale,
    };
  }, [source, zoom]);

  useEffect(() => {
    if (!geometry) return;

    setOffset((current) => ({
      x: clamp(current.x, -geometry.maxX, geometry.maxX),
      y: clamp(current.y, -geometry.maxY, geometry.maxY),
    }));
  }, [geometry]);

  const closeCropper = () => {
    if (processing) return;
    setSource(null);
    setZoom(1);
    setOffset({ x: 0, y: 0 });
    dragRef.current = null;
  };

  const chooseFile = async (file: File | undefined) => {
    if (!file) return;
    if (file.type !== "image/png") {
      addToast({
        title: t("form.iconInvalidType"),
        color: "warning",
      });

      return;
    }
    if (file.size > MAX_INPUT_SIZE) {
      addToast({
        title: t("form.iconInputTooLarge"),
        color: "warning",
      });

      return;
    }

    const url = URL.createObjectURL(file);

    try {
      const image = await loadImage(url);

      setSource({
        height: image.naturalHeight,
        name: file.name,
        url,
        width: image.naturalWidth,
      });
      setZoom(1);
      setOffset({ x: 0, y: 0 });
    } catch {
      URL.revokeObjectURL(url);
      addToast({ title: t("form.iconProcessFailed"), color: "danger" });
    }
  };

  const moveImage = (event: ReactPointerEvent<HTMLDivElement>) => {
    const drag = dragRef.current;

    if (!drag || drag.pointerId !== event.pointerId || !geometry) return;
    setOffset({
      x: clamp(
        drag.x + event.clientX - drag.startX,
        -geometry.maxX,
        geometry.maxX,
      ),
      y: clamp(
        drag.y + event.clientY - drag.startY,
        -geometry.maxY,
        geometry.maxY,
      ),
    });
  };

  const startMoving = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    dragRef.current = {
      pointerId: event.pointerId,
      startX: event.clientX,
      startY: event.clientY,
      ...offset,
    };
  };

  const stopMoving = (event: ReactPointerEvent<HTMLDivElement>) => {
    if (dragRef.current?.pointerId === event.pointerId) {
      dragRef.current = null;
    }
  };

  const renderIcon = async (reduceDetail: boolean) => {
    if (!source || !geometry) throw new Error("Missing crop source");
    const image = await loadImage(source.url);
    const sourceSize = PREVIEW_SIZE / geometry.scale;
    const sourceX =
      (geometry.displayWidth / 2 - PREVIEW_SIZE / 2 - offset.x) /
      geometry.scale;
    const sourceY =
      (geometry.displayHeight / 2 - PREVIEW_SIZE / 2 - offset.y) /
      geometry.scale;
    const canvas = document.createElement("canvas");

    canvas.width = ICON_SIZE;
    canvas.height = ICON_SIZE;
    const context = canvas.getContext("2d");

    if (!context) throw new Error("Canvas is unavailable");
    context.imageSmoothingEnabled = true;
    context.imageSmoothingQuality = "high";

    if (reduceDetail) {
      const staging = document.createElement("canvas");

      staging.width = 64;
      staging.height = 64;
      const stagingContext = staging.getContext("2d");

      if (!stagingContext) throw new Error("Canvas is unavailable");
      stagingContext.imageSmoothingEnabled = true;
      stagingContext.imageSmoothingQuality = "high";
      stagingContext.drawImage(
        image,
        sourceX,
        sourceY,
        sourceSize,
        sourceSize,
        0,
        0,
        staging.width,
        staging.height,
      );
      context.drawImage(staging, 0, 0, ICON_SIZE, ICON_SIZE);
    } else {
      context.drawImage(
        image,
        sourceX,
        sourceY,
        sourceSize,
        sourceSize,
        0,
        0,
        ICON_SIZE,
        ICON_SIZE,
      );
    }

    return canvasToBlob(canvas);
  };

  const applyCrop = async () => {
    setProcessing(true);
    try {
      let blob = await renderIcon(false);

      if (blob.size > MAX_OUTPUT_SIZE) blob = await renderIcon(true);
      if (blob.size > MAX_OUTPUT_SIZE) {
        addToast({
          title: t("form.iconOutputTooLarge"),
          color: "warning",
        });

        return;
      }
      onChange(await blobToDataURL(blob));
      setSource(null);
      setZoom(1);
      setOffset({ x: 0, y: 0 });
    } catch {
      addToast({ title: t("form.iconProcessFailed"), color: "danger" });
    } finally {
      setProcessing(false);
    }
  };

  return (
    <>
      <div className="min-w-0 space-y-1 md:row-span-2">
        <div className="flex min-h-5 items-center px-1 text-sm text-foreground-600">
          {t("fields.icon")}
        </div>
        <div className="flex h-[7.25rem] items-center justify-center rounded-medium border border-default-200 bg-default-50/70 px-3 py-2.5">
          <div className="group relative size-24 shrink-0">
            <button
              aria-label={t("form.uploadIcon")}
              className="relative size-full overflow-hidden rounded-xl border border-default-200 bg-content1 shadow-small outline-none transition-colors hover:border-primary focus-visible:ring-2 focus-visible:ring-primary"
              type="button"
              onClick={() => inputRef.current?.click()}
            >
              <img
                alt={t("fields.icon")}
                className="size-full object-cover"
                src={value || DEFAULT_ICON_URL}
              />
              <span className="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/45 text-white opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100">
                <Icon icon="lucide:upload" width={22} />
              </span>
            </button>
            {value && (
              <Tooltip content={t("form.resetIcon")}>
                <Button
                  isIconOnly
                  aria-label={t("form.resetIcon")}
                  className="absolute right-1 top-1 z-10 size-7 min-w-7 opacity-0 shadow-small transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
                  color="danger"
                  size="sm"
                  type="button"
                  onPress={() => onChange(null)}
                >
                  <Icon icon="lucide:trash-2" width={14} />
                </Button>
              </Tooltip>
            )}
          </div>
          <input
            ref={inputRef}
            accept="image/png,.png"
            className="sr-only"
            type="file"
            onChange={(event) => {
              void chooseFile(event.target.files?.[0]);
              event.target.value = "";
            }}
          />
        </div>
      </div>

      <Modal
        hideCloseButton={processing}
        isDismissable={!processing}
        isKeyboardDismissDisabled={processing}
        isOpen={Boolean(source)}
        placement="center"
        size="lg"
        onOpenChange={(open) => {
          if (!open) closeCropper();
        }}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <span className="text-base font-semibold">
                  {t("form.cropIconTitle")}
                </span>
                <span className="text-xs font-normal text-default-400">
                  {source?.name}
                </span>
              </ModalHeader>
              <ModalBody className="items-center gap-4 pb-3">
                <div
                  aria-label={t("form.cropIconArea")}
                  className="relative size-[280px] max-w-full touch-none cursor-grab overflow-hidden rounded-xl bg-black active:cursor-grabbing"
                  role="application"
                  onPointerCancel={stopMoving}
                  onPointerDown={startMoving}
                  onPointerMove={moveImage}
                  onPointerUp={stopMoving}
                >
                  {source && geometry && (
                    <img
                      alt=""
                      className="pointer-events-none absolute left-1/2 top-1/2 max-w-none select-none"
                      draggable={false}
                      src={source.url}
                      style={{
                        height: geometry.displayHeight,
                        transform: `translate(calc(-50% + ${offset.x}px), calc(-50% + ${offset.y}px))`,
                        width: geometry.displayWidth,
                      }}
                    />
                  )}
                  <div className="pointer-events-none absolute inset-0 rounded-xl ring-1 ring-inset ring-white/60" />
                  <div className="pointer-events-none absolute inset-1/3 border border-white/40" />
                </div>

                <div className="w-full space-y-2">
                  <div className="flex items-center justify-between gap-3 text-xs text-default-500">
                    <span>{t("form.iconZoom")}</span>
                    <span className="tabular-nums">
                      {Math.round(zoom * 100)}%
                    </span>
                  </div>
                  <input
                    aria-label={t("form.iconZoom")}
                    className="h-2 w-full cursor-pointer accent-primary"
                    disabled={processing}
                    max="3"
                    min="1"
                    step="0.01"
                    type="range"
                    value={zoom}
                    onChange={(event) => setZoom(Number(event.target.value))}
                  />
                  <p className="text-xs leading-5 text-default-400">
                    {t("form.cropIconHint")}
                  </p>
                </div>
              </ModalBody>
              <ModalFooter>
                <Button
                  isDisabled={processing}
                  type="button"
                  variant="light"
                  onPress={onClose}
                >
                  {t("actions.cancel")}
                </Button>
                <Button
                  color="primary"
                  isLoading={processing}
                  startContent={
                    !processing ? (
                      <Icon icon="lucide:crop" width={16} />
                    ) : undefined
                  }
                  type="button"
                  onPress={() => void applyCrop()}
                >
                  {t("form.applyIconCrop")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </>
  );
}
