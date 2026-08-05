import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  App,
  Breadcrumb,
  Button,
  FloatButton,
  Form,
  Input,
  Modal,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import {
  ArrowLeftOutlined,
  SaveOutlined,
  CloudUploadOutlined,
  CaretRightOutlined,
  ApartmentOutlined,
  CheckCircleFilled,
  RobotOutlined,
  CodeOutlined,
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
import ChainAgentPanel from "./ChainAgentPanel";
import { setApplyChainDslHandler, useCanvasChatStore } from "@/stores/canvasChatStore";
import {
  dslToFlow,
  flowToDsl,
  isContainerType,
  relationTypesForNode,
  RuleNodeData,
  DslChain,
} from "./chainDsl";
import { layoutFlowElk, layoutFlowTree } from "./elkLayout";
import { shouldAutoLayout, summarizeCanvasDiff } from "./agentCanvas";
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
  const { message, modal } = App.useApp();

  const [chainId, setChainId] = useState<string>(isNew ? "" : id!);
  const [name, setName] = useState("未命名规则链");
  const [description, setDescription] = useState("");
  const [inputSchema, setInputSchema] = useState<Record<string, unknown> | undefined>(undefined);
  // 右侧面板互斥：空白、节点配置、调试控制台、Agent 对话。
  const [panelMode, setPanelMode] = useState<"none" | "config" | "debug" | "agent">("none");
  const [status, setStatus] = useState<string>("draft");
  const [version, setVersion] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const [running, setRunning] = useState(false);
  const [dslOpen, setDslOpen] = useState(false);
  const [dslText, setDslText] = useState("");
  const loadedChainId = useRef<string | null>(null);

  const [stack, setStack] = useState<Frame[]>([
    { nodeId: null, label: "根", nodes: [], edges: [] },
  ]);
  const [selectedNode, setSelectedNode] = useState<Node | null>(null);
  const [agentUndo, setAgentUndo] = useState<Frame[] | null>(null);

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
  // 全部节点（含各层子画布，按画布顺序去重），供节点引用类字段（ref/groupAction 等）做下拉
  const allNodes = useMemo(() => {
    const list: { id: string; name: string; ruleType: string }[] = [];
    const seen = new Set<string>();
    const walk = (nodes: Node[]) => {
      nodes.forEach((n) => {
        if (seen.has(n.id)) return;
        seen.add(n.id);
        const d = n.data as RuleNodeData;
        list.push({ id: n.id, name: d.name ?? d.ruleType, ruleType: d.ruleType });
        if (d.subFlow?.nodes) walk(d.subFlow.nodes);
      });
    };
    stack.forEach((f) => walk(f.nodes));
    return list;
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
        setAgentUndo(null);
        setDirty(false);
      } catch {
        message.error("加载规则链失败");
      } finally {
        setLoading(false);
      }
    })();
  }, [id, isNew, compData, components, message]); // eslint-disable-line

  // 路由切换到另一条规则链时，必须切换对应的 Agent 会话，避免上下文串链。
  useEffect(() => {
    useCanvasChatStore.getState().reset();
    setPanelMode("none");
  }, [id]);

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
      setPanelMode("none");
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

  // ---- 规则链生成器（画布内嵌 Agent）----
  // 取当前画布完整 DSL（含各层子画布），发送给 Agent 做增量编辑。
  const getCanvasDsl = useCallback((): string => {
    const root = collapsedRoot();
    const dsl = flowToDsl(
      { id: chainId || undefined, name },
      root.nodes,
      root.edges,
    );
    try {
      return JSON.stringify(dsl);
    } catch {
      return "";
    }
  }, [collapsedRoot, chainId, name]);

  // 应用 Agent 回传的完整 DSL 到当前画布：整树替换根 Frame（DSL 已含增量编辑结果）。
  const handleApplyDsl = useCallback(
    async (dslStr: string) => {
      try {
        const dsl = JSON.parse(dslStr) as DslChain;
        const { nodes, edges } = dslToFlow(dsl, components);
        if (!validateSwitchConfigurations(nodes)) {
          message.error("应用规则链失败：条件分支配置不完整");
          return;
        }
        const currentRoot = collapsedRoot();
        const nextRoot = { nodeId: null, label: "根", nodes, edges };
        const diff = summarizeCanvasDiff(currentRoot, nextRoot);
        const previewNodes = shouldAutoLayout(dsl, currentRoot, nextRoot)
          ? await layoutFlowTree(nodes, edges)
          : nodes;
        const apply = () => {
          setAgentUndo(stack);
          setStack([{ nodeId: null, label: "根", nodes: previewNodes, edges }]);
          setSelectedNode(null);
          setDirty(true);
          message.success("已把生成的规则链应用到画布，可继续编辑后保存");
        };
        modal.confirm({
          title: "确认应用 Agent 修改？",
          okText: "应用",
          cancelText: "取消",
          content: (
            <div>
              <div>新增节点：{diff.addedNodes} 个，删除节点：{diff.removedNodes} 个，修改节点：{diff.changedNodes} 个。</div>
              <div>新增连线：{diff.addedEdges} 条，删除连线：{diff.removedEdges} 条。</div>
              <div style={{ marginTop: 8, color: "#8c8c8c" }}>
                应用后仍需点击“保存草稿”才会写入规则链。
              </div>
            </div>
          ),
          onOk: apply,
        });
      } catch (error) {
        const reason = error instanceof SyntaxError ? "DSL 格式错误" : "画布布局或结构处理失败";
        message.error(`应用规则链失败：${reason}`);
      }
    },
    [collapsedRoot, components, message, modal, stack],
  );

  const undoAgentChange = useCallback(() => {
    if (!agentUndo) return;
    setStack(agentUndo);
    setAgentUndo(null);
    setSelectedNode(null);
    setDirty(true);
    message.success("已撤销 Agent 修改");
  }, [agentUndo, message]);

  // 挂载时注册"应用到画布"回调；卸载时清空并重置生成器会话。
  useEffect(() => {
    const disposeApplyHandler = setApplyChainDslHandler((dsl, sessionId) => {
      if (sessionId !== useCanvasChatStore.getState().sessionId) return;
      void handleApplyDsl(dsl);
    });
    return () => {
      disposeApplyHandler();
      useCanvasChatStore.getState().reset();
    };
  }, [handleApplyDsl]);

  // ---- 节点操作 ----
  const onSelectNode = useCallback((n: Node | null) => {
    setSelectedNode(n);
    setPanelMode(n ? "config" : "none");
  }, []);
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

  // ---- 保存（返回最终生效的 chainId，新建时为创建后的 id）----
  // 实际写库逻辑；发布/调试复用它直接保存，不弹键设置。
  // 可传入要持久化的 description/inputSchema（弹窗确认时用编辑后的值，避免等 setState）。
  const doSave = useCallback(
    async (vals?: {
      name: string;
      description: string;
      inputSchema?: Record<string, unknown>;
    }): Promise<string> => {
      const chainName = (vals?.name ?? name).trim() || name;
      const desc = vals?.description ?? description;
      const schema = vals ? vals.inputSchema : inputSchema;
      setSaving(true);
      try {
        if (!validateSwitchConfigurations(collapsedRoot().nodes)) {
          message.warning("请补全条件分支的条件表达式和关系名称");
          return "";
        }
        const root = collapsedRoot();
        const dsl = flowToDsl({ id: chainId || undefined, name: chainName }, root.nodes, root.edges);
        if (isNew && !chainId) {
          const created = await chainApi.create({ name: chainName, description: desc, inputSchema: schema, dsl });
          setChainId(created.id);
          message.success("已创建规则链");
          navigate(`/chains/${created.id}/edit`, { replace: true });
          setDirty(false);
          setAgentUndo(null);
          return created.id;
        }
        await chainApi.update(chainId, { name: chainName, description: desc, inputSchema: schema, dsl });
        message.success("已保存草稿");
        setDirty(false);
        setAgentUndo(null);
        return chainId;
      } catch {
        /* 拦截器提示 */
        return "";
      } finally {
        setSaving(false);
      }
    },
    [collapsedRoot, isNew, chainId, name, description, inputSchema, navigate, message],
  );

  // ---- 点「保存草稿」：弹出键设置（名称 + 描述 + 入参），回显当前值，确认后保存 ----
  const onSave = useCallback(() => {
    // 用本地副本承载弹窗内编辑，确认后写回 state 并以编辑后的值直接保存。
    let draftName = name;
    let draftDesc = description;
    let draftSchema = inputSchema;
    modal.confirm({
      title: "键设置 · 名称 / 描述 / 入参格式",
      width: 720,
      icon: null,
      okText: "保存",
      cancelText: "取消",
      content: (
        <ChainSettingsForm
          name={name}
          description={description}
          inputSchema={inputSchema}
          onChange={(patch) => {
            if ("name" in patch) draftName = patch.name ?? "";
            if ("description" in patch) draftDesc = patch.description ?? "";
            if ("inputSchema" in patch) draftSchema = patch.inputSchema;
          }}
        />
      ),
      onOk: () => {
        const finalName = draftName.trim() || name;
        setName(finalName);
        setDescription(draftDesc);
        setInputSchema(draftSchema);
        return doSave({
          name: finalName,
          description: draftDesc,
          inputSchema: draftSchema,
        }).then(() => undefined);
      },
    });
  }, [name, description, inputSchema, modal, doSave]);

  const onViewDsl = useCallback(() => {
    const root = collapsedRoot();
    const dsl = flowToDsl({ id: chainId || undefined, name }, root.nodes, root.edges);
    setDslText(JSON.stringify(dsl, null, 2));
    setDslOpen(true);
  }, [chainId, collapsedRoot, name]);

  // ---- 发布 ----
  const onPublish = useCallback(async () => {
    setPublishing(true);
    try {
      const id = await doSave();
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
  }, [doSave, message]);

  // ---- 调试 ----
  const onDebug = useCallback(
    async (input: string, metadataInput: string) => {
      if (!chainId) {
        message.warning("请先保存后再调试");
        return;
      }
      let metadata: Record<string, string> | undefined;
      if (metadataInput.trim()) {
        try {
          const parsed = JSON.parse(metadataInput) as unknown;
          if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
            message.warning("metadata 必须是 JSON 对象");
            return;
          }
          metadata = Object.fromEntries(
            Object.entries(parsed).map(([key, value]) => [key, String(value)]),
          );
        } catch {
          message.warning("metadata JSON 格式不正确");
          return;
        }
      }
      // 先保存当前草稿（直接保存，不弹键设置），确保调试的是画布最新内容
      const savedId = await doSave();
      if (!savedId) return;
      setRunning(true);
      resetNodeStates();
      setLastRun(null);
      try {
        const res = await chainApi.debug(chainId, {
          data: input,
          dataType: "JSON",
          metadata,
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
    [chainId, doSave, resetNodeStates, setLastRun, setNodeState, message],
  );

  // 点顶栏「调试」只打开控制台，实际请求由控制台中的「运行」触发。
  const onToggleDebug = useCallback(() => {
    setPanelMode((mode) => (mode === "debug" ? "none" : "debug"));
  }, []);

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
          {/* 规则链名称：只读展示（不可在此编辑），改名通过「保存草稿」弹窗进行 */}
          <span
            style={{
              maxWidth: 280,
              fontWeight: 600,
              fontSize: 14,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
            }}
            title={name}
          >
            {name}
          </span>
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
            {agentUndo && (
              <Button onClick={undoAgentChange}>
                撤销 Agent 修改
              </Button>
            )}
            <Button icon={<SaveOutlined />} loading={saving} onClick={onSave}>
              保存草稿
            </Button>
            <Button icon={<CodeOutlined />} onClick={onViewDsl}>
              查看 DSL
            </Button>
            <Button
              icon={<CloudUploadOutlined />}
              loading={publishing}
              onClick={onPublish}
            >
              发布
            </Button>
            <Button
              type={panelMode === "debug" ? "default" : "primary"}
              icon={<CaretRightOutlined />}
              loading={running}
              onClick={onToggleDebug}
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
          <div style={{ flex: 1, minWidth: 0, minHeight: 0 }}>
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
          {/* 右侧面板互斥：空白时隐藏，节点配置/调试/Agent 只显示其中一个 */}
          {panelMode !== "none" && (
          <div className="bf-config" style={{ display: "flex", flexDirection: "column", padding: 0 }}>
            {panelMode === "debug" ? (
              <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
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
            ) : panelMode === "config" ? (
              <div style={{ flex: 1, minHeight: 0, overflow: "auto" }}>
                <NodeConfigPanel
                  node={selectedNode}
                  components={components}
                  onChange={onNodeDataChange}
                  onDelete={onDeleteNode}
                  allNodes={allNodes}
                />
              </div>
            ) : (
              <ChainAgentPanel
                open
                onClose={() => setPanelMode("none")}
                chainId={chainId}
                getCanvasDsl={getCanvasDsl}
              />
            )}
          </div>
          )}
        </div>

        {/* 规则链生成器入口；对话内容显示在右侧互斥面板中 */}
        <FloatButton
          icon={<RobotOutlined />}
          type="primary"
          tooltip="规则链生成器"
          style={{ right: 24, bottom: 24, background: "#722ed1" }}
          onClick={() => setPanelMode((mode) => (mode === "agent" ? "none" : "agent"))}
        />
        <Modal
          title="规则链 DSL"
          open={dslOpen}
          onCancel={() => setDslOpen(false)}
          footer={(
            <Typography.Text
              copyable={{ text: dslText, tooltips: ["复制 DSL", "已复制"] }}
            >
              复制完整 DSL
            </Typography.Text>
          )}
          width={860}
          destroyOnClose
        >
          <Input.TextArea
            value={dslText}
            readOnly
            autoSize={{ minRows: 18, maxRows: 32 }}
            spellCheck={false}
            style={{ fontFamily: "monospace", fontSize: 12 }}
          />
        </Modal>
      </div>
    </ReactFlowProvider>
  );
}

// 键设置表单：编辑规则链名称 + 描述 + 入参 JSON Schema（供 MCP 暴露 / SKILL 生成向调用方说明如何传参）。
// 在 modal.confirm 中挂载（content 一次性渲染、父组件不重渲染），因此名称/描述用本地 state 自管受控，
// onChange 只负责把最新值同步给父级的确认回调。
function ChainSettingsForm({
  name,
  description,
  inputSchema,
  onChange,
}: {
  name: string;
  description: string;
  inputSchema?: Record<string, unknown>;
  onChange: (patch: {
    name?: string;
    description?: string;
    inputSchema?: Record<string, unknown>;
  }) => void;
}) {
  const [localName, setLocalName] = useState(name);
  const [localDesc, setLocalDesc] = useState(description);
  return (
    <div>
      <Form layout="vertical" size="small">
        <Form.Item label="规则链名称" style={{ marginBottom: 16 }} required>
          <Input
            value={localName}
            placeholder="规则链名称"
            onChange={(e) => {
              setLocalName(e.target.value);
              onChange({ name: e.target.value });
            }}
          />
        </Form.Item>
        <Form.Item
          label="规则链描述"
          style={{ marginBottom: 16 }}
          tooltip="说明这条链做什么，会展示在列表、并写入生成的 SKILL / MCP 工具描述"
        >
          <Input.TextArea
            rows={3}
            value={localDesc}
            placeholder="这条规则链做什么？"
            onChange={(e) => {
              setLocalDesc(e.target.value);
              onChange({ description: e.target.value });
            }}
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
