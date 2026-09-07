import { NowhereLogo } from "@/components/layout/navbar-logo";
import { cn } from "@/lib/utils";

interface SubscriptionIconProps {
  alt: string;
  className?: string;
  icon?: string | null;
}

const CUSTOM_ICON_PREFIX = "data:image/png;base64,";

export function SubscriptionIcon({
  alt,
  className,
  icon,
}: SubscriptionIconProps) {
  if (icon?.startsWith(CUSTOM_ICON_PREFIX)) {
    return (
      <img alt={alt} className={cn("object-cover", className)} src={icon} />
    );
  }

  return <NowhereLogo alt={alt} className={className} />;
}
