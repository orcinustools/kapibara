import { NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";

/**
 * Route-driven tab bar. Each tab is a query-param link (?tab=foo) so tab state
 * lives in the URL. `onSelect` lets callers keep using setSearchParams.
 */
export function TabBar({
  tabs,
  active,
  onSelect,
}: {
  tabs: { key: string; label: string }[];
  active: string;
  onSelect: (key: string) => void;
}) {
  return (
    <div className="mb-5 flex flex-wrap gap-1 border-b border-border">
      {tabs.map((t) => (
        <NavLink
          key={t.key}
          to={`?tab=${t.key}`}
          onClick={(e) => {
            e.preventDefault();
            onSelect(t.key);
          }}
          className={cn(
            "-mb-px rounded-t-md border-b-2 px-3 py-2 text-sm font-medium transition-colors",
            active === t.key
              ? "border-primary text-foreground"
              : "border-transparent text-muted-foreground hover:text-foreground"
          )}
        >
          {t.label}
        </NavLink>
      ))}
    </div>
  );
}
