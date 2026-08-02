import { useEffect, useMemo } from "react";
import {
  Button,
  Divider,
  Empty,
  Form,
  Input,
  InputNumber,
  Radio,
  Select,
  Space,
  Switch,
} from "antd";
import { DeleteOutlined } from "@ant-design/icons";
import type { Node } from "@xyflow/react";

import { ComponentFormField, ComponentMeta } from "@/api/component";
import { RuleNodeData } from "./chainDsl";
import { componentZhName, fieldZh, optionZh } from "./componentZh";
import { relationFor, useRadio, widgetFor } from "./fieldWidgets";
import RelationSelect from "./RelationSelect";
import JsonField from "./JsonField";
import KeyValueField from "./KeyValueField";
import CodeField from "@/components/CodeField";

export interface NodeConfigPanelProps {
  node: Node | null;
  components: ComponentMeta[];
  onChange: (nodeId: string, patch: Partial<RuleNodeData>) => void;
  onDelete: (nodeId: string) => void;
}

export default function NodeConfigPanel({
  node,
  components,
  onChange,
  onDelete,
}: NodeConfigPanelProps) {
  const [form] = Form.useForm();
  const d = node?.data as RuleNodeData | undefined;

  const schema = useMemo(
    () => components.find((c) => c.type === d?.ruleType)?.configSchema,
    [components, d?.ruleType],
  );

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
      <div className="bf-config">
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
    const { __name, __debug, ...config } = all;
    const patch: Partial<RuleNodeData> = { configuration: config };
    if ("__name" in changed) patch.name = __name as string;
    if ("__debug" in changed) patch.debugMode = !!__debug;
    onChange(node.id, patch);
  };

  return (
    <div className="bf-config">
      <div style={{ padding: 14 }}>
        <Form
          form={form}
          layout="vertical"
          size="middle"
          onValuesChange={handleValues}
        >
          <Form.Item name="__name" label="节点名称">
            <Input placeholder={componentZhName(d.ruleType)} />
          </Form.Item>
          <Form.Item label="组件类型">
            <Input
              value={`${componentZhName(d.ruleType)}（${d.ruleType}）`}
              disabled
            />
          </Form.Item>
          <Form.Item
            name="__debug"
            label="调试模式"
            valuePropName="checked"
            tooltip="运行时输出该节点逐条事件"
          >
            <Switch size="small" />
          </Form.Item>

          {(schema?.fields?.length ?? 0) > 0 && (
            <Divider style={{ margin: "8px 0" }} plain>
              配置
            </Divider>
          )}

          {(schema?.fields ?? []).map((f) => (
            d.ruleType === "switch" && f.name === "cases" ? (
              <SwitchCasesField key={f.name} field={f} />
            ) : (
              <SchemaField key={f.name} field={f} ruleType={d.ruleType} />
            )
          ))}
        </Form>

        <Divider style={{ margin: "12px 0" }} />
        <Button
          danger
          block
          icon={<DeleteOutlined />}
          onClick={() => onDelete(node.id)}
        >
          删除节点
        </Button>
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

  return (
    <Form.List name="cases">
      {(fields, { add, remove }) => (
        <Form.Item label={label}>
          <Space direction="vertical" style={{ width: "100%" }} size="small">
            {fields.map((field, index) => (
              <Space key={field.key} align="start" style={{ width: "100%" }}>
                <Form.Item
                  {...field}
                  name={[field.name, "case"]}
                  rules={[{ required: true, message: "请填写条件表达式" }]}
                  style={{ flex: 1, marginBottom: 0 }}
                >
                  <Input.TextArea
                    rows={2}
                    placeholder="如 msg.temperature > 50"
                    aria-label={`第${index + 1}条条件`}
                  />
                </Form.Item>
                <Form.Item
                  {...field}
                  name={[field.name, "then"]}
                  rules={[{ required: true, message: "请填写关系名称" }]}
                  style={{ width: 110, marginBottom: 0 }}
                >
                  <Input placeholder="关系名" aria-label={`第${index + 1}条关系名`} />
                </Form.Item>
                <Button
                  danger
                  type="text"
                  icon={<DeleteOutlined />}
                  aria-label={`删除第${index + 1}条分支`}
                  onClick={() => remove(field.name)}
                />
              </Space>
            ))}
            <Button
              type="dashed"
              block
              onClick={() => add({ case: "", then: `Case${fields.length + 1}` })}
            >
              添加分支条件
            </Button>
          </Space>
        </Form.Item>
      )}
    </Form.List>
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
}: {
  field: ComponentFormField;
  ruleType: string;
}) {
  const { label, labelText } = useFieldLabel(ruleType, f);
  const required = f.required || (f.rules ?? []).some((r) => r.required);
  const reqMsg = `请填写${labelText}`;

  // 1. 关系下拉（引用其它平台资源：子链 / Agent / LLM / MCP / Skill）
  const relation = relationFor(ruleType, f.name);
  if (relation) {
    return (
      <Form.Item
        name={f.name}
        label={label}
        rules={[{ required, message: `请选择${labelText}` }]}
      >
        <RelationSelect relation={relation} placeholder={`请选择${labelText}`} />
      </Form.Item>
    );
  }

  // 2. 单选按钮（少量静态枚举选项）
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
