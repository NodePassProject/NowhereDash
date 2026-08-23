export type SiteConfig = typeof siteConfig;

export const siteConfig = {
  name: "NowhereDash",
  description: "A modern and secure tunnel dashboard.",
  navItems: [
    {
      label: "仪表盘",
      href: "/dashboard",
    },
    {
      label: "订阅管理",
      href: "/subscriptions",
    },
    {
      label: "隧道管理",
      href: "/tunnels",
    },
    {
      label: "节点管理",
      href: "/endpoints",
    },
  ],
  navMenuItems: [
    {
      label: "设置",
      href: "/settings",
    },
    {
      label: "退出登录",
      href: "/logout",
    },
  ],
  links: {
    github: "https://github.com/NodePassProject/NowhereDash",
    docs: "https://github.com/NodePassProject/Nowhere/tree/main/docs",
  },
};
