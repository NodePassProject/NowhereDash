import { NavbarBrand, Link, cn } from "@heroui/react";
import { fontSans } from "@/config/fonts";
import { UpdateChip } from "./update-chip";

// Compact crop of the official Nowhere artwork.
export const NowhereLogo = () => {
  return (
    <span
      aria-label="Nowhere"
      className="block h-8 w-8 shrink-0 rounded-md bg-left bg-no-repeat shadow-sm"
      role="img"
      style={{
        backgroundImage: "url('/nowhere.png')",
        backgroundSize: "700% 100%",
      }}
    />
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
        <p className={cn("font-bold text-foreground pl-1", fontSans.className)}>
          NowhereDash
        </p>
      </Link>
      <UpdateChip />
    </NavbarBrand>
  );
};
