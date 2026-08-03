import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  App,
  Breadcrumb,
  Button,
  Drawer,
  Form,
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
  SettingOutlined,
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
import InputSchemaField from "./InputSchemaField";
import {
  dslToFlow,
  flowToDsl,
  isContainerType,
  relationTypesForNode,
  RuleNodeData,
  DslChain,
} from "./chainDsl";
import { layoutFlowElk } from "./elkLayout";
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

function validateSwitchConfigurations(nodes: Node[]): boolean {
  return nodes.every((node) => {
    const data = node.data as RuleNodeData;
    const cases = data.ruleType === "switch" ? data.configuration.cases : undefined;
    const validCases = !Array.isArray(cases) || cases.every((item) => {
      if (!item || typeof item !== "object") return false;
      const branch = item as { case?: unknown; then?: unknown };
      return typeof branch.case === "string"
        && branch.case.trim().length > 0
        && typeof branch.then === "string"
        && branch.then.trim().length > 0;
    });
    const subNodes = data.subFlow?.nodes;
    return validCases && (!subNodes || validateSwitchConfigurations(subNodes));
  });
}

export default function ChainEditorPage() {
  const { id } = useParams<{ id: string }>();
  const isNew = id === "new";
  const navigate = useNavigate();
  const { message } = App.useApp();

  const [chainId, setChainId] = useState<string>(isNew ? "" : id!);
  const [name, setName] = useState("未命名规则链");
  const [description, setDescription] = useState("");
  const [inputSchema, setInputSchema] = useState<Record<string, unknown> | undefined>(undefined);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [status, setStatus] = useState<string>("draft");
  const [version, setVersion] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [running, setRunning] = useState(false);
  const loadedChainId = useRef<string | null>(null);

  const [stack, setStack] = useState<Frame[]>([
    { nodeId: null, label: "根", nodes: [], edges: [] },
  ]);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);

  const nodeStates = useCanvasStore((s) => s.nodeStates);
  const setNodeState = useCanvasStore((s) => s.setNodeState);
  const resetNodeStates = useCanvasStore((s) => s.resetNodeStates);
  const lastRun = useCanvasStore((s) => s.lastRun);
  const setLastRun = useCanvasStore((s) => s.setLastRun);
  const setSelectedNodeId = useCanvasStore((s) => s.setSelectedNodeId);

  const { data: compData } = useQuery({
    queryKey: ["components"],
    queryFn: () => componentApi.list(),
    staleTime: 5 * 60 * 1000,
  });
  const components = useMemo(() => compData?.list ?? [], [compData]);

  const cur = stack[stack.length - 1];
  // 画布节点 id -> 显示名（含所有层级子画布），供调试控制台把 nodeId 映射成中文节点名
  const nodeNames = useMemo(() => {
    const map: Record<string, string> = {};
    const walk = (nodes: Node[]) => {
      nodes.forEach((n) => {
        const d = n.data as RuleNodeData;
        map[n.id] = d.name ?? d.ruleType;
        if (d.subFlow?.nodes) walk(d.subFlow.nodes);
      });
    };
    stack.forEach((f) => walk(f.nodes));
    return map;
  }, [stack]);
  const setCur = useCallback((patch: Partial<Frame>) => {
    setStack((st) =>
      st.map((f, i) => (i === st.length - 1 ? { ...f, ...patch } : f)),
    );
    setDirty(true);
  }, []);

  // ---- 加载 ----
  useEffect(() => {
    if (isNew) return;
    if (!compData) return;
    if (loadedChainId.current === id) return;
    (async () => {
      setLoading(true);
      try {
        const c = await chainApi.get(id!);
        setName(c.name);
        setDescription(c.description ?? "");
        setInputSchema(c.inputSchema && Object.keys(c.inputSchema).length > 0 ? c.inputSchema : undefined);
        setStatus(c.status);
        setVersion(c.version);
        const { nodes, edges } = dslToFlow((c.dsl as DslChain) ?? {}, components);
        setStack([{ nodeId: null, label: "根", nodes, edges }]);
        loadedChainId.current = id!;
        setDirty(false);
      } catch {
        message.error("加载规则链失败");
      } finally {
        setLoading(false);
      }
    })();
  }, [id, isNew, compData, components, message]); // eslint-disable-line

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
      const nextPatch = { ...patch };
      let nextEdges = cur.edges;
      if (current && patch.configuration) {
        const currentData = current.data as RuleNodeData;
        const nextRelations = relationTypesForNode(
          currentData.ruleType,
          patch.configuration,
          components,
        );
        const staleEdges = currentData.ruleType === "switch"
          ? cur.edges.filter((edge) => {
              if (edge.source !== nodeId) return false;
              const relationType = (edge.data as { relationType?: unknown } | undefined)?.relationType;
              return typeof relationType === "string" && !nextRelations.includes(relationType);
            })
          : [];
        if (staleEdges.length > 0) {
          message.warning(`条件关系已变更，移除${staleEdges.length}条失效连线`);
          const staleIds = new Set(staleEdges.map((edge) => edge.id));
          nextEdges = cur.edges.filter((edge) => !staleIds.has(edge.id));
        }
        const edgeRelations = cur.edges
          .filter((edge) => edge.source === nodeId)
          .map((edge) => edge.data?.relationType)
          .filter((value): value is string => typeof value === "string");
        nextPatch.relationTypes = relationTypesForNode(
          currentData.ruleType,
          patch.configuration,
          components,
          edgeRelations.filter((relation) => nextRelations.includes(relation)),
        );
      }
      if (current) {
        setSelectedNode({ ...current, data: { ...current.data, ...nextPatch } });
      }
      setCur({
        nodes: cur.nodes.map((n) =>
          n.id === nodeId ? { ...n, data: { ...n.data, ...nextPatch } } : n,
        ),
        edges: nextEdges,
      });
    },
    [components, cur.edges, cur.nodes, message, setCur],
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

  const onLayout = useCallback(async () => {
    try {
      const laid = await layoutFlowElk(cur.nodes, cur.edges);
      setCur({ nodes: laid });
      message.success("已自动整理布局");
    } catch {
      message.error("自动布局失败");
    }
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
      if (!validateSwitchConfigurations(collapsedRoot().nodes)) {
        message.warning("请补全条件分支的条件表达式和关系名称");
        return "";
      }
      const dsl = buildDsl();
      if (isNew && !chainId) {
        const created = await chainApi.create({ name, description, inputSchema, dsl });
        setChainId(created.id);
        message.success("已创建规则链");
        navigate(`/chains/${created.id}/edit`, { replace: true });
        setDirty(false);
        return created.id;
      }
      await chainApi.update(chainId, { name, description, inputSchema, dsl });
      message.success("已保存草稿");
      setDirty(false);
      return chainId;
    } catch {
      /* 拦截器提示 */
      return "";
    } finally {
      setSaving(false);
    }
  }, [buildDsl, isNew, chainId, name, description, inputSchema, navigate, message]);

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
      const savedId = await onSave();
      if (!savedId) return;
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

  // 调试控制台"定位"：在所有层级子画布中找到节点并选中（同画布点击）。
  const onLocateNode = useCallback(
    (nodeId: string) => {
      const find = (nodes: Node[]): Node | null => {
        for (const n of nodes) {
          if (n.id === nodeId) return n;
          const sub = (n.data as RuleNodeData).subFlow?.nodes;
          if (sub) {
            const hit = find(sub);
            if (hit) return hit;
          }
        }
        return null;
      };
      const root = collapsedRoot();
      const node = find(root.nodes);
      if (node) {
        setSelectedNodeId(nodeId);
        setSelectedNode(node);
      }
    },
    [collapsedRoot, setSelectedNodeId],
  );

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
            <Tooltip title="链设置：描述与入参格式（供 MCP / SKILL 调用方参考）">
              <Button icon={<SettingOutlined />} onClick={() => setSettingsOpen(true)} />
            </Tooltip>
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
                components={components}
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
              nodeNames={nodeNames}
              onRun={onDebug}
              onClear={onClearDebug}
              onLocateNode={onLocateNode}
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

      <Drawer
        title="链设置 · 描述与入参格式"
        width={720}
        open={settingsOpen}
        onClose={() => setSettingsOpen(false)}
      >
        <ChainSettingsForm
          description={description}
          inputSchema={inputSchema}
          onChange={(patch) => {
            if ("description" in patch) setDescription(patch.description ?? "");
            if ("inputSchema" in patch) setInputSchema(patch.inputSchema);
            setDirty(true);
          }}
        />
      </Drawer>
    </ReactFlowProvider>
  );
}

// 链设置表单：编辑描述与入参 JSON Schema（供 MCP 暴露 / SKILL 生成向调用方说明如何传参）。
// 本地受控（不走 antd Form），随主"保存草稿"一起持久化。
function ChainSettingsForm({
  description,
  inputSchema,
  onChange,
}: {
  description: string;
  inputSchema?: Record<string, unknown>;
  onChange: (patch: {
    description?: string;
    inputSchema?: Record<string, unknown>;
  }) => void;
}) {
  return (
    <div>
      <Form layout="vertical" size="small">
        <Form.Item
          label="规则链描述"
          style={{ marginBottom: 16 }}
          tooltip="说明这条链做什么，会展示在列表、并写入生成的 SKILL / MCP 工具描述"
        >
          <Input.TextArea
            rows={3}
            value={description}
            placeholder="这条规则链做什么？"
            onChange={(e) => onChange({ description: e.target.value })}
          />
        </Form.Item>
        <Form.Item
          label="入参格式（JSON Schema，可选）"
          style={{ marginBottom: 0 }}
          tooltip="声明调用方应传的消息体结构；MCP 暴露与 SKILL 生成会用它说明如何传参。表格/JSON 双视图实时同步，描述列即字段注释。"
        >
          <InputSchemaField
            value={inputSchema}
            onChange={(v) => onChange({ inputSchema: v })}
          />
        </Form.Item>
      </Form>
    </div>
  );
}
