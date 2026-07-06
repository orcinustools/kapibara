// Railway-style service canvas: an auto-laid-out node graph of a project's
// deployable units (applications, databases, compose apps). Nodes are plain
// React components; elkjs computes positions; @xyflow/react handles the canvas.
import ELK from "elkjs/lib/elk.bundled.js";
import { Handle, Position, type Node } from "@xyflow/react";
import { Boxes, Database, FileCode2 } from "lucide-react";
import { cn } from "@/lib/utils";

export type UnitKind = "application" | "database" | "compose";

export interface Unit {
  id: string;
  kind: UnitKind;
  name: string;
  subtitle: string;
  status?: string; // pod phase, e.g. "Running", "Pending"
  raw: any;
}

export interface UnitNodeData extends Record<string, unknown> {
  unit: Unit;
  selected: boolean;
}

const KIND_META: Record<UnitKind, { icon: typeof Boxes; label: string; accent: string }> = {
  application: { icon: Boxes, label: "App", accent: "text-primary" },
  database: { icon: Database, label: "Database", accent: "text-success" },
  compose: { icon: FileCode2, label: "Compose", accent: "text-amber-500" },
};

function statusDot(status?: string): string {
  const s = (status || "").toLowerCase();
  if (s === "running" || s === "succeeded") return "bg-success";
  if (s === "pending" || s === "containercreating") return "bg-amber-500";
  if (!s) return "bg-muted-foreground/40";
  return "bg-destructive";
}

/** UnitNode renders one deployable unit as a Railway-style card node. */
export function UnitNode({ data }: { data: UnitNodeData }) {
  const { unit, selected } = data;
  const meta = KIND_META[unit.kind];
  const Icon = meta.icon;
  return (
    <div
      className={cn(
        "w-[220px] rounded-xl border bg-card px-3.5 py-3 shadow-sm transition-colors",
        selected ? "border-primary ring-2 ring-primary/40" : "border-border hover:border-primary/50",
      )}
    >
      <Handle type="target" position={Position.Left} className="!size-2 !border-border !bg-muted-foreground/50" />
      <div className="flex items-center gap-2">
        <span className={cn("grid size-8 shrink-0 place-items-center rounded-lg bg-accent", meta.accent)}>
          <Icon className="size-4" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold leading-tight text-foreground">{unit.name}</div>
          <div className="truncate text-[11px] text-muted-foreground">{unit.subtitle}</div>
        </div>
        <span
          className={cn("size-2.5 shrink-0 rounded-full", statusDot(unit.status))}
          title={unit.status || "no pods"}
        />
      </div>
      <div className="mt-2 flex items-center justify-between">
        <span className="rounded-md bg-accent px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          {meta.label}
        </span>
        {unit.status && <span className="text-[10px] text-muted-foreground">{unit.status}</span>}
      </div>
      <Handle type="source" position={Position.Right} className="!size-2 !border-border !bg-muted-foreground/50" />
    </div>
  );
}

export const nodeTypes = { unit: UnitNode };

const elk = new ELK();

/** layoutUnits computes left-to-right layered positions for the unit nodes. */
export async function layoutUnits(
  units: Unit[],
  edges: { id: string; source: string; target: string }[],
  selectedId: string | null,
): Promise<Node<UnitNodeData>[]> {
  const W = 220;
  const H = 84;
  const graph = {
    id: "root",
    layoutOptions: {
      "elk.algorithm": "layered",
      "elk.direction": "RIGHT",
      "elk.layered.spacing.nodeNodeBetweenLayers": "110",
      "elk.spacing.nodeNode": "48",
    },
    children: units.map((u) => ({ id: u.id, width: W, height: H })),
    edges: edges.map((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  };
  let pos: Record<string, { x: number; y: number }> = {};
  try {
    const res = await elk.layout(graph as any);
    for (const c of res.children || []) pos[c.id] = { x: c.x ?? 0, y: c.y ?? 0 };
  } catch {
    // Fallback: simple vertical stack if elk fails.
    units.forEach((u, i) => (pos[u.id] = { x: 0, y: i * (H + 48) }));
  }
  return units.map((u) => ({
    id: u.id,
    type: "unit",
    position: pos[u.id] || { x: 0, y: 0 },
    data: { unit: u, selected: u.id === selectedId },
  }));
}
