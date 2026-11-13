import {
  BarChart3,
  Boxes,
  ChevronLeft,
  ClipboardList,
  CreditCard,
  LayoutDashboard,
  Map,
  Menu,
  Moon,
  Monitor,
  Package,
  Receipt,
  Server,
  ShoppingCart,
  Sparkles,
  Store,
  Sun,
  Tags,
  User,
  type LucideIcon
} from 'lucide-react';

// Icons come from lucide (https://lucide.dev). Pages refer to them by short name.
const icons: Record<string, LucideIcon> = {
  home: LayoutDashboard,
  orders: ClipboardList,
  payments: CreditCard,
  tag: Tags,
  chart: BarChart3,
  server: Server,
  box: Boxes,
  package: Package,
  map: Map,
  menu: Menu,
  chevron: ChevronLeft,
  cart: ShoppingCart,
  receipt: Receipt,
  store: Store,
  user: User,
  spark: Sparkles,
  sun: Sun,
  moon: Moon,
  system: Monitor
};

export function Icon({ name, className = 'h-[18px] w-[18px]' }: { name: string; className?: string }) {
  const Glyph = icons[name] ?? Boxes;
  return <Glyph className={className} strokeWidth={1.8} aria-hidden="true" />;
}
