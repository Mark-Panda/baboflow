import { describe, expect, it } from 'vitest';

import { routeAround, type Pt, type Rect } from './routePath';

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
