// 组件类别 -> 配色 / 图标（基于 RuleGo 真实类别）。
import {
  ApiOutlined, FilterOutlined, SwapOutlined, ThunderboltOutlined,
  BlockOutlined, GlobalOutlined, RobotOutlined,
} from '@ant-design/icons';
import React from 'react';

export interface CatStyle {
  color: string;
  label: string;
  icon: React.ReactNode;
}

export const CATEGORY_STYLE: Record<string, CatStyle> = {
  endpoint: { color: '#7c5cff', label: '端点', icon: React.createElement(ApiOutlined) },
  filter: { color: '#e6a23c', label: '过滤', icon: React.createElement(FilterOutlined) },
  transform: { color: '#4f8cff', label: '转换', icon: React.createElement(SwapOutlined) },
  action: { color: '#3fbf6b', label: '动作', icon: React.createElement(ThunderboltOutlined) },
  external: { color: '#3fbf6b', label: '外部', icon: React.createElement(GlobalOutlined) },
  common: { color: '#13c2c2', label: '流程', icon: React.createElement(BlockOutlined) },
  flow: { color: '#13c2c2', label: '子链', icon: React.createElement(BlockOutlined) },
  agent: { color: '#eb2f96', label: 'Agent', icon: React.createElement(RobotOutlined) },
};

export const DEFAULT_CAT: CatStyle = { color: '#9095a5', label: '通用', icon: React.createElement(BlockOutlined) };

export function catStyle(category: string): CatStyle {
  return CATEGORY_STYLE[category] ?? DEFAULT_CAT;
}

// 类别中文名（调色板分组标题）
export const CATEGORY_LABEL: Record<string, string> = {
  endpoint: '端点',
  filter: '过滤',
  transform: '转换',
  action: '动作',
  external: '外部调用',
  common: '流程控制',
  flow: '子链',
  agent: 'Agent',
};

export function categoryLabel(c: string): string {
  return CATEGORY_LABEL[c] ?? c;
}
