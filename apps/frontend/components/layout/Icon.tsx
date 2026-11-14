import {
  BarChart3,
  Boxes,
  Bot,
  CornerDownRight,
  Folder,
  KeyRound,
  Trash2,
  Users,
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
  spark: Bot,
  sun: Sun,
  moon: Moon,
  system: Monitor,
  folder: Folder,
  child: CornerDownRight,
  users: Users,
  key: KeyRound,
  trash: Trash2
};

export function Icon({ name, className = 'h-[18px] w-[18px]' }: { name: string; className?: string }) {
  const Glyph = icons[name] ?? Boxes;
  return <Glyph className={className} strokeWidth={1.8} aria-hidden="true" />;
}
