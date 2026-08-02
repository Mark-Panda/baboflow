import { useCallback, useEffect, useMemo, useState } from "react";
import {
  App,
  Breadcrumb,
  Button,
  Input,
  Space,
  Spin,
  Tag,
  Tooltip,
} from "antd";
import {
  ArrowLeftOutlined,
  SaveOutlined,
  CloudUploadOutlined,
  CaretRightOutlined,
  ApartmentOutlined,
  CheckCircleFilled,
} from "@ant-design/icons";
import { useNavigate, useParams } from "react-router-dom";
import { ReactFlowProvider, type Edge, type Node } from "@xyflow/react";
import { useQuery } from "@tanstack/react-query";

import { chainApi } from "@/api/chain";
import { componentApi } from "@/api/component";
import ComponentPalette from "./ComponentPalette";
import FlowCanvas from "./FlowCanvas";
import NodeConfigPanel from "./NodeConfigPanel";
import DebugPanel from "./DebugPanel";
import {
  dslToFlow,
  flowToDsl,
  layoutFlow,
  isContainerType,
  RuleNodeData,
  DslChain,
} from "./chainDsl";
import { useCanvasStore } from "@/stores/canvasStore";
import "./canvas.css";
import "@xyflow/react/dist/style.css";

// 子画布栈帧：root 为根画布，其余为容器节点的子画布
interface Frame {
  nodeId: string | null; // root 为 null
  label: string;
  nodes: Node[];
  edges: Edge[];
}

export default function ChainEditorPage() {
  const { id } = useParams<{ id: string }>();
  const isNew = id === "new";
  const navigate = useNavigate();
  const { message } = App.useApp();

  const [chainId, setChainId] = useState<string>(isNew ? "" : id!);
  const [name, setName] = useState("未命名规则链");
  const [description] = useState("");
  const [status, setStatus] = useState<string>("draft");
  const [version, setVersion] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [running, setRunning] = useState(false);

  const [stack, setStack] = useState<Frame[]>([
    { nodeId: null, label: "根", nodes: [], edges: [] },
  ]);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);

  const nodeStates = useCanvasStore((s) => s.nodeStates);
  const setNodeState = useCanvasStore((s) => s.setNodeState);
  const resetNodeStates = useCanvasStore((s) => s.resetNodeStates);
  const lastRun = useCanvasStore((s) => s.lastRun);
  const setLastRun = useCanvasStore((s) => s.setLastRun);

  const { data: compData } = useQuery({
    queryKey: ["components"],
    queryFn: () => componentApi.list(),
    staleTime: 5 * 60 * 1000,
  });
  const components = useMemo(() => compData?.list ?? [], [compData]);

  const cur = stack[stack.length - 1];
  const setCur = useCallback((patch: Partial<Frame>) => {
    setStack((st) =>
      st.map((f, i) => (i === st.length - 1 ? { ...f, ...patch } : f)),
    );
    setDirty(true);
  }, []);

  // ---- 加载 ----
  useEffect(() => {
    if (isNew) return;
    (async () => {
      setLoading(true);
      try {
        const c = await chainApi.get(id!);
        setName(c.name);
        setStatus(c.status);
        setVersion(c.version);
        const { nodes, edges } = dslToFlow((c.dsl as DslChain) ?? {});
        setStack([{ nodeId: null, label: "根", nodes, edges }]);
        setDirty(false);
      } catch {
        message.error("加载规则链失败");
      } finally {
        setLoading(false);
      }
    })();
  }, [id, isNew]); // eslint-disable-line

  // ---- 进入/退出子画布 ----
  const enterSub = useCallback(
    (nodeId: string) => {
      const node = cur.nodes.find((n) => n.id === nodeId);
      if (!node || !isContainerType((node.data as RuleNodeData).ruleType))
        return;
      const d = node.data as RuleNodeData;
      const sub = d.subFlow ?? { nodes: [], edges: [] };
      setStack((st) => [
        ...st,
        { nodeId, label: d.name, nodes: sub.nodes, edges: sub.edges },
      ]);
      setSelectedNode(null);
    },
    [cur.nodes],
  );

  const jumpTo = useCallback((index: number) => {
    setStack((st) => {
      if (index >= st.length - 1) return st;
      // 把更深层子画布逐级写回容器节点 subFlow，再截断到目标层
      const next = [...st];
      for (let i = next.length - 1; i > index; i--) {
        const popped = next[i];
        const parent = next[i - 1];
        parent.nodes = parent.nodes.map((n) =>
          n.id === popped.nodeId
            ? {
                ...n,
                data: {
                  ...n.data,
                  subFlow: { nodes: popped.nodes, edges: popped.edges },
                },
              }
            : n,
        );
        next.splice(i, 1);
      }
      return next;
    });
    setSelectedNode(null);
  }, []);

  // 退出前把当前子画布写回其父容器节点（进入保存/调试时统一处理）
  const collapsedRoot = useCallback((): Frame => {
    const next = stack.map((f) => ({
      ...f,
      nodes: [...f.nodes],
      edges: [...f.edges],
    }));
    for (let i = next.length - 1; i > 0; i--) {
      const popped = next[i];
      const parent = next[i - 1];
      parent.nodes = parent.nodes.map((n) =>
        n.id === popped.nodeId
          ? {
              ...n,
              data: {
                ...n.data,
                subFlow: { nodes: popped.nodes, edges: popped.edges },
              },
            }
          : n,
      );
    }
    return next[0];
  }, [stack]);

  // ---- 节点操作 ----
  const onSelectNode = useCallback((n: Node | null) => setSelectedNode(n), []);
  const onNodeDataChange = useCallback(
    (nodeId: string, patch: Partial<RuleNodeData>) => {
      const current = cur.nodes.find((n) => n.id === nodeId);
      if (current) {
        setSelectedNode({ ...current, data: { ...current.data, ...patch } });
      }
      setCur({
        nodes: cur.nodes.map((n) =>
          n.id === nodeId ? { ...n, data: { ...n.data, ...patch } } : n,
        ),
      });
    },
    [cur.nodes, setCur],
  );
  const onDeleteNode = useCallback(
    (nodeId: string) => {
      setCur({
        nodes: cur.nodes.filter((n) => n.id !== nodeId),
        edges: cur.edges.filter(
          (e) => e.source !== nodeId && e.target !== nodeId,
        ),
      });
      setSelectedNode(null);
    },
    [cur.nodes, cur.edges, setCur],
  );

  const onLayout = useCallback(() => {
    setCur({ nodes: layoutFlow(cur.nodes, cur.edges) });
    message.success("已自动整理布局");
  }, [cur.nodes, cur.edges, setCur, message]);

  // ---- 构建 DSL ----
  const buildDsl = useCallback((): DslChain => {
    const root = collapsedRoot();
    return flowToDsl(
      { id: chainId || undefined, name },
      root.nodes,
      root.edges,
    );
  }, [collapsedRoot, chainId, name]);

  // ---- 保存（返回最终生效的 chainId，新建时为创建后的 id）----
  const onSave = useCallback(async (): Promise<string> => {
    setSaving(true);
    try {
      const dsl = buildDsl();
      if (isNew && !chainId) {
        const created = await chainApi.create({ name, description, dsl });
        setChainId(created.id);
        message.success("已创建规则链");
        navigate(`/chains/${created.id}/edit`, { replace: true });
        setDirty(false);
        return created.id;
      }
      await chainApi.update(chainId, { name, description, dsl });
      message.success("已保存草稿");
      setDirty(false);
      return chainId;
    } catch {
      /* 拦截器提示 */
      return "";
    } finally {
      setSaving(false);
    }
  }, [buildDsl, isNew, chainId, name, description, navigate, message]);

  // ---- 发布 ----
  const onPublish = useCallback(async () => {
    setPublishing(true);
    try {
      const id = await onSave();
      if (!id) {
        message.warning("保存失败，无法发布");
        return;
      }
      const r = await chainApi.publish(id);
      setStatus("published");
      setVersion(r.version);
      message.success(`已发布 v${r.version}`);
    } catch {
      /* 拦截器提示 */
    } finally {
      setPublishing(false);
    }
  }, [onSave, message]);

  // ---- 调试 ----
  const onDebug = useCallback(
    async (input: string) => {
      if (!chainId) {
        message.warning("请先保存后再调试");
        return;
      }
      // 先保存当前草稿，确保调试的是画布最新内容
      await onSave();
      setRunning(true);
      resetNodeStates();
      setLastRun(null);
      try {
        const res = await chainApi.debug(chainId, {
          data: input,
          dataType: "JSON",
        });
        setLastRun({
          output: res.output,
          error: res.error,
          traces: res.nodeTrace ?? [],
        });
        // 逐节点着色
        (res.nodeTrace ?? []).forEach((t) => {
          setNodeState(t.nodeId, t.err ? "failure" : "success");
        });
        if (res.error) message.error(`运行失败：${res.error}`);
        else message.success("运行完成");
      } catch {
        /* 拦截器提示 */
      } finally {
        setRunning(false);
      }
    },
    [chainId, onSave, resetNodeStates, setLastRun, setNodeState, message],
  );

  const onClearDebug = useCallback(() => {
    resetNodeStates();
    setLastRun(null);
  }, [resetNodeStates, setLastRun]);

  if (loading) {
    return (
      <div
        style={{
          height: "100vh",
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
        }}
      >
        <Spin size="large" tip="加载规则链…" />
      </div>
    );
  }

  const hasStates = Object.keys(nodeStates).length > 0;

  return (
    <ReactFlowProvider>
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          height: "100vh",
          background: "#f5f6fa",
        }}
      >
        {/* 顶栏 */}
        <div className="bf-editor-bar">
          <Tooltip title="返回列表">
            <Button
              type="text"
              icon={<ArrowLeftOutlined />}
              onClick={() => navigate("/chains")}
            />
          </Tooltip>
          <Input
            value={name}
            onChange={(e) => {
              setName(e.target.value);
              setDirty(true);
            }}
            bordered={false}
            style={{ width: 240, fontWeight: 600, fontSize: 14 }}
            placeholder="规则链名称"
          />
          <Tag
            color={status === "published" ? "green" : "default"}
            style={{ marginLeft: 4 }}
          >
            {status === "published" ? `已发布 v${version}` : "草稿"}
          </Tag>
          {dirty && (
            <span style={{ color: "#e6a23c", fontSize: 12 }}>●未保存</span>
          )}
          {hasStates && !dirty && (
            <span style={{ color: "#3fbf6b", fontSize: 12 }}>
              <CheckCircleFilled /> 调试完成
            </span>
          )}

          <Space style={{ marginLeft: "auto" }} size="small">
            <Tooltip title="自动整理布局">
              <Button icon={<ApartmentOutlined />} onClick={onLayout} />
            </Tooltip>
            <Button icon={<SaveOutlined />} loading={saving} onClick={onSave}>
              保存草稿
            </Button>
            <Button
              icon={<CloudUploadOutlined />}
              loading={publishing}
              onClick={onPublish}
            >
              发布
            </Button>
            <Button
              type="primary"
              icon={<CaretRightOutlined />}
              loading={running}
              onClick={() => onDebug("{}")}
            >
              调试
            </Button>
          </Space>
        </div>

        {/* 面包屑（子画布层级） */}
        {stack.length > 1 && (
          <div
            style={{
              padding: "6px 14px",
              background: "#fbfcfe",
              borderBottom: "1px solid #eef0f5",
            }}
          >
            <Breadcrumb
              items={stack.map((f, i) => ({
                title: (
                  <a
                    onClick={() => jumpTo(i)}
                    style={{
                      color: i === stack.length - 1 ? "#1f2430" : "#4f6ef7",
                    }}
                  >
                    {i === 0 ? name || "根" : f.label}
                  </a>
                ),
              }))}
            />
          </div>
        )}

        {/* 三栏 */}
        <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
          <ComponentPalette />
          <div
            style={{
              flex: 1,
              minWidth: 0,
              display: "flex",
              flexDirection: "column",
            }}
          >
            <div style={{ flex: 1, minHeight: 0 }}>
              <FlowCanvas
                nodes={cur.nodes}
                edges={cur.edges}
                onNodesChange={(nodes) => setCur({ nodes })}
                onEdgesChange={(edges) => setCur({ edges })}
                onSelectNode={onSelectNode}
                onEnterSub={enterSub}
              />
            </div>
            <DebugPanel
              running={running}
              output={lastRun?.output ?? ""}
              error={lastRun?.error ?? ""}
              traces={lastRun?.traces ?? []}
              onRun={onDebug}
              onClear={onClearDebug}
            />
          </div>
          <NodeConfigPanel
            node={selectedNode}
            components={components}
            onChange={onNodeDataChange}
            onDelete={onDeleteNode}
          />
        </div>
      </div>
    </ReactFlowProvider>
  );
}
