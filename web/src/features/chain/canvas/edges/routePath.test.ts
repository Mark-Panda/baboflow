import { describe, expect, it } from 'vitest';

import { labelAnchorNearSource, routeAround, type Pt, type Rect } from './routePath';

function rect(x: number, y: number, width: number, height: number): Rect {
  return { x, y, width, height };
}

// 判断折线任一段是否与矩形内部相交（测试辅助，与实现同一几何约定）。
function hitsRect(pts: Pt[], r: Rect, gap = 0): boolean {
  const R = { x: r.x - gap, y: r.y - gap, width: r.width + gap * 2, height: r.height + gap * 2 };
  for (let i = 0; i < pts.length - 1; i += 1) {
    const a = pts[i];
    const b = pts[i + 1];
    if (a.y === b.y) {
      const lo = Math.min(a.x, b.x);
      const hi = Math.max(a.x, b.x);
      if (a.y > R.y && a.y < R.y + R.height && hi > R.x && lo < R.x + R.width) return true;
    } else {
      const lo = Math.min(a.y, b.y);
      const hi = Math.max(a.y, b.y);
      if (a.x > R.x && a.x < R.x + R.width && hi > R.y && lo < R.y + R.height) return true;
    }
  }
  return false;
}

describe('routeAround', () => {
  it('同一水平线、无障碍 → 直线（两点）', () => {
    const pts = routeAround({ x: 0, y: 100 }, { x: 300, y: 100 }, []);
    expect(pts).toEqual([
      { x: 0, y: 100 },
      { x: 300, y: 100 },
    ]);
  });

  it('不同高度、无障碍 → 标准 H-V-H（4 点）且不撞', () => {
    const s = { x: 0, y: 0 };
    const t = { x: 300, y: 200 };
    const pts = routeAround(s, t, []);
    expect(pts[0]).toEqual(s);
    expect(pts[pts.length - 1]).toEqual(t);
    expect(pts.length).toBe(4);
    // 中间拐点在 midX
    expect(pts[1].x).toBeCloseTo(150);
    expect(pts[2].x).toBeCloseTo(150);
  });

  it('中间有节点 → 路径绕开其包围盒（不穿过节点）', () => {
    const s = { x: 0, y: 100 };
    const t = { x: 400, y: 100 };
    const blocker = rect(180, 80, 60, 40); // 横在直连路径上的中间节点
    const pts = routeAround(s, t, [blocker]);
    // 必须起终点正确
    expect(pts[0]).toEqual(s);
    expect(pts[pts.length - 1]).toEqual(t);
    // 不穿过障碍（含 GAP 缓冲）
    expect(hitsRect(pts, blocker, 18)).toBe(false);
    // 确实拐了弯（不止一条直线）
    expect(pts.length).toBeGreaterThan(2);
  });

  it('目标在障碍正后方（不同高度）→ 也能绕开', () => {
    const s = { x: 0, y: 50 };
    const t = { x: 400, y: 150 };
    const blocker = rect(180, 60, 80, 80);
    const pts = routeAround(s, t, [blocker]);
    expect(hitsRect(pts, blocker, 18)).toBe(false);
  });

  it('多个障碍 → 不与任何一个相交', () => {
    const s = { x: 0, y: 100 };
    const t = { x: 500, y: 100 };
    const obstacles = [rect(120, 80, 60, 40), rect(300, 70, 60, 60)];
    const pts = routeAround(s, t, obstacles);
    obstacles.forEach((r) => expect(hitsRect(pts, r, 18)).toBe(false));
  });

  it('完全同向多障碍时退到安全通道', () => {
    const s = { x: 0, y: 0 };
    const t = { x: 200, y: 0 };
    // 一道横贯的"墙"，上下都有空隙但 midY 被挡
    const wall = rect(50, -50, 100, 40);
    const pts = routeAround(s, t, [wall]);
    expect(hitsRect(pts, wall, 18)).toBe(false);
  });
});

describe('labelAnchorNearSource', () => {
  it('水平首段：锚在源点右侧 offset 处', () => {
    const anchor = labelAnchorNearSource([{ x: 0, y: 100 }, { x: 300, y: 100 }], 28);
    expect(anchor).toEqual({ x: 28, y: 100 });
  });

  it('H-V-H 首段（先水平）：锚在源→第一拐点方向，不压拐点', () => {
    // 源 (0,0) → 第一拐点 (150,0)（midX），首段长 150
    const anchor = labelAnchorNearSource(
      [{ x: 0, y: 0 }, { x: 150, y: 0 }, { x: 150, y: 200 }, { x: 300, y: 200 }],
      28,
    );
    expect(anchor).toEqual({ x: 28, y: 0 });
  });

  it('首段过短（< 2*offset）时取首段中点', () => {
    // 源 (0,0) → 第一拐点 (20,0)，首段长 20 < 56 → 取中点 (10,0)
    const anchor = labelAnchorNearSource(
      [{ x: 0, y: 0 }, { x: 20, y: 0 }, { x: 20, y: 100 }, { x: 300, y: 100 }],
      28,
    );
    expect(anchor).toEqual({ x: 10, y: 0 });
  });

  it('源在目标右侧（连线向左）：锚点也向左偏移', () => {
    const anchor = labelAnchorNearSource([{ x: 300, y: 50 }, { x: 0, y: 50 }], 28);
    expect(anchor).toEqual({ x: 272, y: 50 });
  });

  it('汇入同一目标的多条连线：各自锚点不同（不重叠）', () => {
    const t = { x: 400, y: 200 };
    const a = routeAround({ x: 0, y: 0 }, t, []);
    const b = routeAround({ x: 0, y: 400 }, t, []);
    const anchorA = labelAnchorNearSource(a);
    const anchorB = labelAnchorNearSource(b);
    // 两条线源端高度不同，锚点应分别贴近各自源端
    expect(anchorA).not.toEqual(anchorB);
    expect(anchorA.y).toBeCloseTo(a[0].y);
    expect(anchorB.y).toBeCloseTo(b[0].y);
  });

  it('仅单点/空拐点时退回源点', () => {
    expect(labelAnchorNearSource([{ x: 5, y: 5 }])).toEqual({ x: 5, y: 5 });
  });
});
