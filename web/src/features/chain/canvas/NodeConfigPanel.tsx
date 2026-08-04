import { useEffect, useMemo, useState } from "react";
import {
  AutoComplete,
  Button,
  Divider,
  Empty,
  Form,
  Input,
  InputNumber,
  Radio,
  Select,
  Switch,
  Tooltip,
  Typography,
} from "antd";
import { DeleteOutlined, DownOutlined, RightOutlined } from "@ant-design/icons";
import type { Node } from "@xyflow/react";

import { ComponentFormField, ComponentMeta } from "@/api/component";
import { RuleNodeData } from "./chainDsl";
import { componentZhDesc, componentZhName, fieldZh, optionZh } from "./componentZh";
import {
  fieldGroupsFor,
  isFreeInputSelect,
  isHiddenField,
  isStructArrayField,
  relationFor,
  staticOptionsFor,
  useRadio,
  widgetFor,
} from "./fieldWidgets";
import RelationSelect from "./RelationSelect";
import type { NodeOption } from "./NodeSelect";
import JsonField from "./JsonField";
import KeyValueField from "./KeyValueField";
import StructArrayField from "./StructArrayField";
import SwitchCasesBuilder from "./SwitchCasesBuilder";
import CodeField from "@/components/CodeField";

export interface NodeConfigPanelProps {
  node: Node | null;
  components: ComponentMeta[];
  onChange: (nodeId: string, patch: Partial<RuleNodeData>) => void;
  onDelete: (nodeId: string) => void;
  // 当前规则链全部节点（跨子画布打平），供节点引用类字段做下拉
  allNodes?: NodeOption[];
}

export default function NodeConfigPanel({
  node,
  components,
  onChange,
  onDelete,
  allNodes = [],
}: NodeConfigPanelProps) {
  const [form] = Form.useForm();
  const d = node?.data as RuleNodeData | undefined;

  const schema = useMemo(
    () => components.find((c) => c.type === d?.ruleType)?.configSchema,
    [components, d?.ruleType],
  );

  // 可见配置字段：过滤掉前端隐藏的（如 delay 的 deprecated 字段）。
  const visibleFields = useMemo(
    () =>
      (schema?.fields ?? []).filter(
        (f) => !isHiddenField(d?.ruleType ?? "", f.name),
      ),
    [schema, d?.ruleType],
  );

  // 长表单分组（restApiCall/sendEmail/mqttClient 等）；未配置则平铺。
  const groups = useMemo(
    () => fieldGroupsFor(d?.ruleType ?? ""),
    [d?.ruleType],
  );
  // 折叠状态：key 为分组 title；默认收起 defaultCollapsed 的组。
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  useEffect(() => {
    const init: Record<string, boolean> = {};
    (groups ?? []).forEach((g) => {
      init[g.title] = !!g.defaultCollapsed;
    });
    setCollapsed(init);
  }, [groups, node?.id]);

  useEffect(() => {
    form.resetFields();
    if (node && d) {
      form.setFieldsValue({
        __name: d.name,
        __debug: !!d.debugMode,
        ...(d.configuration ?? {}),
      });
    }
  }, [form, node?.id]);

  if (!node || !d) {
    return (
      <div className="bf-config-panel">
        <div className="bf-config-empty">
          <Empty
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            description="选中画布中的节点进行配置"
          />
        </div>
      </div>
    );
  }

  const handleValues = (
    changed: Record<string, unknown>,
    all: Record<string, unknown>,
  ) => {
    // 只保留真正的配置字段：剔除 __name/__debug 及 antd Form 内部键（fields 等），
    // 避免把 Form 内部状态误写进 configuration（尤其 switch.cases 会被污染）。
    const { __name, __debug, ...rest } = all;
    const config: Record<string, unknown> = {};
    Object.entries(rest).forEach(([k, v]) => {
      if (!k.startsWith("__") && k !== "fields") config[k] = v;
    });
    const patch: Partial<RuleNodeData> = { configuration: config };
    if ("__name" in changed) patch.name = __name as string;
    if ("__debug" in changed) patch.debugMode = !!__debug;
    onChange(node.id, patch);
  };

  // 渲染单个配置字段（switch.cases 走专用构建器）。
  const renderField = (f: ComponentFormField) =>
    d.ruleType === "switch" && f.name === "cases" ? (
      <SwitchCasesField key={f.name} field={f} />
    ) : (
      <SchemaField
        key={f.name}
        field={f}
        ruleType={d.ruleType}
        allNodes={allNodes}
        selfId={node.id}
      />
    );

  // 分组渲染：按 FIELD_GROUPS 顺序，未列入任何组的字段追加到「其它」组（保持兜底，不丢字段）。
  const renderGrouped = () => {
    const byName = new Map(visibleFields.map((f) => [f.name, f]));
    const assigned = new Set<string>();
    const sections: React.ReactNode[] = [];
    (groups ?? []).forEach((g) => {
      const gfs = g.fields
        .map((n) => byName.get(n))
        .filter((x): x is ComponentFormField => !!x);
      gfs.forEach((f) => assigned.add(f.name));
      if (gfs.length === 0) return;
      const isCollapsed = !!collapsed[g.title];
      sections.push(
        <div key={g.title} className="bf-cfg-group">
          <button
            type="button"
            className="bf-cfg-group-head"
            onClick={() =>
              setCollapsed((c) => ({ ...c, [g.title]: !c[g.title] }))
            }
          >
            {isCollapsed ? <RightOutlined /> : <DownOutlined />}
            <span>{g.title}</span>
            <span className="bf-cfg-group-count">{gfs.length}</span>
          </button>
          {!isCollapsed && <div className="bf-cfg-group-body">{gfs.map(renderField)}</div>}
        </div>,
      );
    });
    // 兜底：未进任何分组的字段（schema 新增字段时不会丢）。
    const rest = visibleFields.filter((f) => !assigned.has(f.name));
    if (rest.length > 0) {
      sections.push(
        <div key="__rest" className="bf-cfg-group">
          <div className="bf-cfg-group-head bf-cfg-group-head-static">
            <span>其它</span>
            <span className="bf-cfg-group-count">{rest.length}</span>
          </div>
          <div className="bf-cfg-group-body">{rest.map(renderField)}</div>
        </div>,
      );
    }
    return sections;
  };

  return (
    <div className="bf-config-panel">
      <div className="bf-config-body">
        {/* 节点标题卡：中文名 + 英文 type + 组件简述（只读，替代原占整行的禁用输入框） */}
        <div className="bf-config-head">
          <div className="bf-config-head-title">
            <span className="bf-config-head-name">{componentZhName(d.ruleType)}</span>
            <span className="bf-config-head-type">{d.ruleType}</span>
          </div>
          {componentZhDesc(d.ruleType) && (
            <div className="bf-config-head-desc">{componentZhDesc(d.ruleType)}</div>
          )}
        </div>

        <Form
          form={form}
          layout="vertical"
          size="middle"
          onValuesChange={handleValues}
        >
          <Form.Item
            name="__name"
            label={
              <span className="bf-node-name-label">
                节点名称
                <Tooltip title="节点 ID（只读，点击复制）">
                  <Typography.Text
                    className="bf-node-id-chip"
                    copyable={{ text: node.id, tooltips: ["复制 ID", "已复制"] }}
                  >
                    {node.id}
                  </Typography.Text>
                </Tooltip>
              </span>
            }
            style={{ marginBottom: 10 }}
          >
            <Input placeholder={componentZhName(d.ruleType)} />
          </Form.Item>
          <Form.Item
            name="__debug"
            label="调试模式"
            valuePropName="checked"
            tooltip="运行时输出该节点逐条事件"
            style={{ marginBottom: 10 }}
          >
            <Switch size="small" />
          </Form.Item>

          {visibleFields.length > 0 && (
            <Divider style={{ margin: "4px 0 12px" }} plain>
              配置
            </Divider>
          )}

          {groups ? renderGrouped() : visibleFields.map(renderField)}

          {/* comment 节点在 RuleGo 无配置字段，这里补一个本地注释框（写入 configuration.comment） */}
          {d.ruleType === "comment" && (
            <Form.Item
              name="comment"
              label={
                <span>
                  注释内容
                  <div
                    style={{
                      color: "#a2a9bd",
                      fontSize: 11,
                      fontWeight: 400,
                      lineHeight: 1.4,
                    }}
                  >
                    仅作画布说明，不参与执行
                  </div>
                </span>
              }
            >
              <Input.TextArea
                rows={4}
                placeholder="给这个节点写点说明…"
              />
            </Form.Item>
          )}
        </Form>

        <div className="bf-config-foot">
          <Button
            danger
            size="small"
            icon={<DeleteOutlined />}
            onClick={() => onDelete(node.id)}
          >
            删除节点
          </Button>
        </div>
      </div>
    </div>
  );
}

function SwitchCasesField({ field: f }: { field: ComponentFormField }) {
  const labelText = fieldZh("switch", f.name)?.label ?? f.label ?? "分支条件";
  const descText = fieldZh("switch", f.name)?.desc ?? f.desc;
  const label = (
    <span>
      {labelText}
      {descText && (
        <div style={{ color: "#a2a9bd", fontSize: 11, fontWeight: 400, lineHeight: 1.4 }}>
          {descText}
        </div>
      )}
    </span>
  );

  // 可视化构建器：IF/ELIF 分支（规则行 ⇄ 表达式），ELSE=Default 固定。
  // 作为普通受控字段交给 antd Form（value/onChange 注入），随 onValuesChange 写回 configuration.cases。
  return (
    <Form.Item name="cases" label={label}>
      <SwitchCasesBuilder />
    </Form.Item>
  );
}

// 判断字段是否为代码/表达式类（用 JS 编辑器渲染）。
function isCodeField(f: ComponentFormField): {
  code: boolean;
  language: string;
} {
  const compType = f.component?.type;
  if (compType === "codeEditor" || compType === "code") {
    return {
      code: true,
      language: (f.component?.language as string) ?? "javascript",
    };
  }
  const n = f.name.toLowerCase();
  if (/(^|_)js(script)?$|jsscript|script/.test(n))
    return { code: true, language: "javascript" };
  if (/sql/.test(n)) return { code: true, language: "sql" };
  if (/expr/.test(n)) return { code: true, language: "javascript" }; // expr-lang 近似 JS 高亮
  if (/template/.test(n)) return { code: true, language: "template" };
  return { code: false, language: "text" };
}

// 字段标签（中文 label + 灰色描述），各分支共用。
function useFieldLabel(ruleType: string, f: ComponentFormField) {
  const zh = fieldZh(ruleType, f.name);
  const labelText = zh?.label ?? f.label ?? f.name;
  const descText = zh?.desc ?? f.desc;
  const label = (
    <span>
      {labelText}
      {descText && (
        <div
          style={{
            color: "#a2a9bd",
            fontSize: 11,
            fontWeight: 400,
            lineHeight: 1.4,
          }}
        >
          {descText}
        </div>
      )}
    </span>
  );
  return { label, labelText };
}

function SchemaField({
  field: f,
  ruleType,
  allNodes = [],
  selfId,
}: {
  field: ComponentFormField;
  ruleType: string;
  allNodes?: NodeOption[];
  selfId?: string;
}) {
  const { label, labelText } = useFieldLabel(ruleType, f);
  const required = f.required || (f.rules ?? []).some((r) => r.required);
  const reqMsg = `请填写${labelText}`;

  // 1. 关系下拉（引用其它平台资源：子链 / Agent / LLM / MCP / Skill / 本链节点）
  const relation = relationFor(ruleType, f.name);
  if (relation) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: `请选择${labelText}` }]}
      >
        <RelationSelect
          relation={relation}
          placeholder={`请选择${labelText}`}
          nodes={allNodes}
          excludeId={selfId}
        />
      </Form.Item>
    );
  }

  // 2. 单选按钮（schema 声明的少量静态枚举选项）
  const radioOptions =
    (f.component?.options as
      | Array<{ label: string; value: unknown }>
      | undefined) ?? parseOptions(f);
  if (useRadio(ruleType, f.name, radioOptions.length)) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: `请选择${labelText}` }]}
      >
        <Radio.Group
          optionType="button"
          buttonStyle="solid"
          options={radioOptions.map((o) => ({
            label: optionZh(f.name, o.value) ?? o.label,
            value: o.value,
          }))}
        />
      </Form.Item>
    );
  }

  // 2b. 前端补的静态枚举（schema 未给 options，但取值集合明确）：下拉/单选。
  // 少数允许自定义的（如 groupAction.matchRelationType）用 AutoComplete 可输可选。
  const staticOpts = staticOptionsFor(ruleType, f.name);
  if (staticOpts && staticOpts.length > 0) {
    const rules = [{ required, message: `请选择${labelText}` }];
    if (isFreeInputSelect(ruleType, f.name)) {
      return (
        <Form.Item name={f.name} label={label} rules={rules}>
          <AutoComplete
            allowClear
            options={staticOpts}
            placeholder={`请选择或输入${labelText}`}
          />
        </Form.Item>
      );
    }
    if (useRadio(ruleType, f.name, staticOpts.length)) {
      return (
        <Form.Item name={f.name} label={label} rules={rules}>
          <Radio.Group
            optionType="button"
            buttonStyle="solid"
            options={staticOpts}
          />
        </Form.Item>
      );
    }
    return (
      <Form.Item name={f.name} label={label} rules={rules}>
        <Select
          options={staticOpts}
          allowClear
          placeholder={`请选择${labelText}`}
        />
      </Form.Item>
    );
  }

  // 2c. struct 数组（cache.keys/items）：按子字段 schema 渲染可增删的行编辑器。
  if (isStructArrayField(ruleType, f.name) && (f.fields?.length ?? 0) > 0) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: reqMsg }]}
      >
        <StructArrayField subFields={f.fields ?? []} ruleType={ruleType} />
      </Form.Item>
    );
  }

  // 3. JSON 对象 / 键值对（在代码编辑器之前判断，避免被 name 正则误判）
  const widget = widgetFor(ruleType, f);
  if (widget === "json") {
    return (
      <Form.Item name={f.name} label={label}>
        <JsonField placeholder={`请输入${labelText}（JSON）`} />
      </Form.Item>
    );
  }
  if (widget === "kv") {
    return (
      <Form.Item name={f.name} label={label}>
        <KeyValueField />
      </Form.Item>
    );
  }

  // 代码/表达式编辑器（JS 语法高亮）
  const cf = isCodeField(f);
  if (cf.code) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: reqMsg }]}
      >
        <CodeField language={cf.language} rows={7} />
      </Form.Item>
    );
  }
  // 下拉选择
  const options = radioOptions;
  if (f.component?.type === "select" || options.length > 0) {
    const localizedOptions = options.map((o) => ({
      ...o,
      label: optionZh(f.name, o.value) ?? o.label,
    }));
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: `请选择${labelText}` }]}
      >
        <Select
          options={localizedOptions}
          allowClear
          placeholder={`请选择${labelText}`}
        />
      </Form.Item>
    );
  }
  // 布尔
  if (f.type === "bool" || f.type === "boolean") {
    return (
      <Form.Item name={f.name} label={label} valuePropName="checked">
        <Switch size="small" />
      </Form.Item>
    );
  }
  // 数字
  if (
    f.type === "int" ||
    f.type === "int64" ||
    f.type === "float" ||
    f.type === "number"
  ) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: reqMsg }]}
      >
        <InputNumber style={{ width: "100%" }} placeholder={labelText} />
      </Form.Item>
    );
  }
  // 多行文本（body/content 等非代码长文本；headers 已被 kv 分支接管）
  if (f.component?.type === "textarea" || /body|content/i.test(f.name)) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: reqMsg }]}
      >
        <Input.TextArea
          rows={4}
          style={{ fontFamily: "monospace", fontSize: 12 }}
          placeholder={labelText}
        />
      </Form.Item>
    );
  }
  // 默认单行
  return (
    <Form.Item
      name={f.name}
      label={label}
      rules={[{ required, message: reqMsg }]}
    >
      <Input placeholder={labelText} />
    </Form.Item>
  );
}

// 从 validate 串解析 enum 选项（如 "oneof=a b c"）。
function parseOptions(
  f: ComponentFormField,
): Array<{ label: string; value: string }> {
  const v = f.validate ?? "";
  const m = v.match(/oneof=(.+)$/);
  if (!m) return [];
  return m[1]
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .map((x) => ({ label: x, value: x }));
}
