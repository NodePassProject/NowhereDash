import { NavbarBrand, Link, cn } from "@heroui/react";

import { UpdateChip } from "./update-chip";

import { fontSans } from "@/config/fonts";

interface BrandProps {
  className?: string;
}

interface LogoProps extends BrandProps {
  alt?: string;
}

export const NowhereLogo = ({ alt = "NowhereDash", className }: LogoProps) => {
  const logoClassName = cn("block h-8 w-8 shrink-0 object-contain", className);

  return (
    <>
      <img
        alt={alt}
        className={cn(logoClassName, "dark:hidden")}
        height={32}
        src="/logo.png"
        width={32}
      />
      <img
        alt={alt}
        className={cn(logoClassName, "hidden dark:block")}
        height={32}
        src="/logo-dark.png"
        width={32}
      />
    </>
  );
};

export const NowhereBrandLabel = ({ className }: BrandProps) => {
  return (
    <span
      aria-label="NowhereDash"
      className={cn(
        "whitespace-nowrap pl-1 font-bold text-foreground",
        fontSans.className,
        className,
      )}
    >
      <span aria-hidden="true">Nowhere</span>
      <span
        aria-hidden="true"
        className="bg-gradient-to-r from-[#0868f2] to-[#19aff4] bg-clip-text text-transparent"
      >
        Dash
      </span>
    </span>
  );
};

/**
 * 导航栏Logo组件
 */
export const NavbarLogo = () => {
  return (
    <NavbarBrand as="li" className="gap-1 max-w-fit items-center">
      <Link className="flex justify-start items-center" href="/">
        <NowhereLogo />
        <NowhereBrandLabel />
      </Link>
      <UpdateChip />
    </NavbarBrand>
  );
};
