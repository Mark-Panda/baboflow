// 正交绕障路由：给定源/目标锚点与障碍矩形集合，产出一条"绕过节点"的折线顶点序列。
// 纯函数，便于单测；渲染端用 getSmoothStepPath 把拐点转成圆角折线（过渡自然）。

export interface Pt {
  x: number;
  y: number;
}

export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}

// 障碍外扩缓冲：给节点四周留出让边通行的通道。
const GAP = 18;

function inflate(r: Rect, gap: number): Rect {
  return { x: r.x - gap, y: r.y - gap, width: r.width + gap * 2, height: r.height + gap * 2 };
}

// 水平线段 y=c 在 [x1,x2] 区间是否与矩形内部相交
function hSegHitsRect(x1: number, x2: number, y: number, r: Rect): boolean {
  const lo = Math.min(x1, x2);
  const hi = Math.max(x1, x2);
  return y > r.y && y < r.y + r.height && hi > r.x && lo < r.x + r.width;
}

// 竖直线段 x=c 在 [y1,y2] 区间是否与矩形内部相交
function vSegHitsRect(x: number, y1: number, y2: number, r: Rect): boolean {
  const lo = Math.min(y1, y2);
  const hi = Math.max(y1, y2);
  return x > r.x && x < r.x + r.width && hi > r.y && lo < r.y + r.height;
}

// 一条折线是否完全不撞任何障碍
function pathClear(pts: Pt[], barriers: Rect[]): boolean {
  for (let i = 0; i < pts.length - 1; i += 1) {
    const a = pts[i];
    const b = pts[i + 1];
    const hit = barriers.some((r) =>
      a.y === b.y ? hSegHitsRect(a.x, b.x, a.y, r) : vSegHitsRect(a.x, a.y, b.y, r),
    );
    if (hit) return false;
  }
  return true;
}

// 收集可绕行的候选横向通道 y：每个障碍的上下沿（±1），外加整体最上/最下。
function candidateYs(s: Pt, t: Pt, barriers: Rect[]): number[] {
  const ys = new Set<number>();
  barriers.forEach((r) => {
    ys.add(Math.round(r.y) - 1);
    ys.add(Math.round(r.y + r.height) + 1);
  });
  // 以源/目标连线的垂直通道为准，找出真正挡路的障碍用于兜底通道
  const midX = (s.x + t.x) / 2;
  const blockers = barriers.filter((r) => vSegHitsRect(midX, s.y, t.y, r));
  if (blockers.length > 0) {
    ys.add(Math.round(Math.min(...blockers.map((r) => r.y))) - 1);
    ys.add(Math.round(Math.max(...blockers.map((r) => r.y + r.height))) + 1);
  }
  return [...ys];
}

/**
 * 计算从 s 到 t 的正交折线顶点（含起终点），绕开 obstacles（调用方需传入未外扩的节点包围盒）。
 * 返回顶点数组；相邻点构成水平或竖直段。
 * 策略：
 *  1. 优先 H-V-H（先水平后竖直再水平）——左→右连边的常规形态；
 *  2. 有障时尝试所有候选横向通道 y 的 S 形（源→(sx,y)→(tx,y)→目标）；
 *  3. 再退化到 V-H-V；最后兜底整体最上方通道。
 */
export function routeAround(s: Pt, t: Pt, obstacles: Rect[], gap: number = GAP): Pt[] {
  const barriers = obstacles.map((r) => inflate(r, gap));
  const midX = (s.x + t.x) / 2;
  const midY = (s.y + t.y) / 2;

  // 1. 直连/标准 H-V-H
  const hvh: Pt[] =
    Math.abs(s.y - t.y) < 1
      ? [s, t]
      : [s, { x: midX, y: s.y }, { x: midX, y: t.y }, t];
  if (pathClear(hvh, barriers)) return hvh;

  // 2. S 形：横向通道 y 取候选值，挑离源/目标中点最近的一条
  const candidates = candidateYs(s, t, barriers).sort(
    (a, b) => Math.abs(a - midY) - Math.abs(b - midY),
  );
  for (const y of candidates) {
    const detour: Pt[] = [s, { x: s.x, y }, { x: t.x, y }, t];
    if (pathClear(detour, barriers)) return detour;
  }

  // 3. V-H-V：先竖直后水平（适合目标在源正下方/上方且横向被挡）
  for (const y of candidates) {
    const vhv: Pt[] = [s, { x: s.x, y }, { x: t.x, y }, t];
    if (pathClear(vhv, barriers)) return vhv;
  }

  // 4. 兜底：抬到所有障碍之上的安全高度
  const safeY =
    barriers.length > 0 ? Math.round(Math.min(...barriers.map((r) => r.y))) - 1 : midY;
  return [s, { x: s.x, y: safeY }, { x: t.x, y: safeY }, t];
}

/**
 * 关系标签锚点：沿「源点 → 第一个拐点」方向取距源 offset 处。
 * 多条连线汇入同一目标时，路径几何中心（getSmoothStepPath 的默认 label 位）会重叠；
 * 而每条连线的首段是各自独有的，把标签锚在首段可避免相互遮挡。
 * 首段不足 2*offset 时取其中点，避免压到拐点；无拐点时退回源→目标方向。
 */
export function labelAnchorNearSource(pts: Pt[], offset: number = 28): Pt {
  const s = pts[0];
  const next = pts[1] ?? pts[pts.length - 1];
  if (!next) return s;
  const dx = next.x - s.x;
  const dy = next.y - s.y;
  const len = Math.hypot(dx, dy);
  if (len < 1) return s;
  const dist = Math.min(offset, len / 2);
  return { x: s.x + (dx / len) * dist, y: s.y + (dy / len) * dist };
}
