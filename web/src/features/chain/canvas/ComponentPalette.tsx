import { useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Collapse, Empty, Input, Spin } from "antd";

import { componentApi, ComponentMeta } from "@/api/component";
import { catStyle, categoryLabel } from "./category";
import { componentZhDesc, componentZhName } from "./componentZh";

export const DND_MIME = "application/rulego-component";

// 分组顺序
const ORDER = [
  "endpoint",
  "filter",
  "transform",
  "action",
  "external",
  "common",
  "flow",
];

export default function ComponentPalette() {
  const [kw, setKw] = useState("");
  const [activeKeys, setActiveKeys] = useState<string[]>([]);
  const { data, isLoading } = useQuery({
    queryKey: ["components"],
    queryFn: () => componentApi.list(),
    staleTime: 5 * 60 * 1000,
  });

  const groups = useMemo(() => {
    const list = (data?.list ?? []).filter((c) => !c.configSchema?.disabled);
    const k = kw.trim().toLowerCase();
    const filtered = k
      ? list.filter(
          (c) =>
            c.name.toLowerCase().includes(k) ||
            c.type.toLowerCase().includes(k) ||
            componentZhName(c.type).toLowerCase().includes(k) ||
            (componentZhDesc(c.type) ?? "").toLowerCase().includes(k),
        )
      : list;
    const byCat = new Map<string, ComponentMeta[]>();
    filtered.forEach((c) => {
      const arr = byCat.get(c.category) ?? [];
      arr.push(c);
      byCat.set(c.category, arr);
    });
    const entries = Array.from(byCat.entries());
    entries.sort(
      (a, b) =>
        (ORDER.indexOf(a[0]) === -1 ? 99 : ORDER.indexOf(a[0])) -
        (ORDER.indexOf(b[0]) === -1 ? 99 : ORDER.indexOf(b[0])),
    );
    return entries;
  }, [data, kw]);

  useEffect(() => {
    const keys = groups.map(([category]) => category);
    setActiveKeys((previous) => {
      if (kw.trim()) return keys;
      const kept = previous.filter((key) => keys.includes(key));
      return kept.length > 0 ? kept : keys;
    });
  }, [groups, kw]);

  return (
    <div className="bf-palette">
      <div style={{ padding: "10px 10px 6px" }}>
        <Input.Search
          allowClear
          size="small"
          placeholder="搜索组件（中文/英文/类型）"
          onChange={(e) => setKw(e.target.value)}
        />
      </div>
      {isLoading ? (
        <div style={{ textAlign: "center", padding: 30 }}>
          <Spin size="small" />
        </div>
      ) : groups.length === 0 ? (
        <Empty
          image={Empty.PRESENTED_IMAGE_SIMPLE}
          description="无匹配组件"
          style={{ marginTop: 30 }}
        />
      ) : (
        <Collapse
          ghost
          size="small"
          activeKey={activeKeys}
          onChange={(keys) =>
            setActiveKeys(Array.isArray(keys) ? keys : [keys])
          }
          items={groups.map(([cat, items]) => ({
            key: cat,
            label: (
              <span
                style={{
                  fontSize: 12,
                  fontWeight: 600,
                  color: catStyle(cat).color,
                }}
              >
                {categoryLabel(cat)}（{items.length}）
              </span>
            ),
            children: items.map((c) => (
              <div
                key={c.type}
                className="bf-palette-item"
                style={{
                  ["--cat-color" as string]: catStyle(c.category).color,
                }}
                draggable
                title={`${componentZhName(c.type)}（${c.type}）\n${componentZhDesc(c.type) ?? c.description ?? ""}`}
                onDragStart={(e) => {
                  e.dataTransfer.setData(DND_MIME, JSON.stringify(c));
                  e.dataTransfer.effectAllowed = "move";
                }}
              >
                <span className="pi-dot" />
                <span>{componentZhName(c.type)}</span>
                <span className="pi-type">{c.type}</span>
              </div>
            )),
          }))}
        />
      )}
    </div>
  );
}
