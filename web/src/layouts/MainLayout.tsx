import { useMemo } from 'react';
import { Outlet, useLocation, useNavigate } from 'react-router-dom';
import { Dropdown, Layout, Menu, Modal, Form, Input, App } from 'antd';
import {
  DashboardOutlined,
  DeploymentUnitOutlined,
  RobotOutlined,
  ApartmentOutlined,
  SettingOutlined,
  LogoutOutlined,
  KeyOutlined,
  DownOutlined,
  FileTextOutlined,
  BookOutlined,
  ApiOutlined,
  SafetyOutlined,
  ClockCircleOutlined,
  DatabaseOutlined,
} from '@ant-design/icons';
import { useState } from 'react';

import { useAuthStore } from '@/stores/authStore';
import { authApi } from '@/api/auth';

const { Sider, Header, Content } = Layout;

export default function MainLayout() {
  const navigate = useNavigate();
  const location = useLocation();
  const { user, logout, setUser } = useAuthStore();
  const { message } = App.useApp();
  const [pwdOpen, setPwdOpen] = useState(false);
  const [form] = Form.useForm();

  const selectedKey = useMemo(() => {
    const p = location.pathname;
    if (p.startsWith('/chains')) return '/chains';
    if (p.startsWith('/runs')) return '/runs';
    if (p.startsWith('/agents')) return '/agents';
    if (p.startsWith('/skills')) return '/skills';
    if (p.startsWith('/mcp')) return '/mcp';
    if (p.startsWith('/boards')) return '/boards';
    if (p.startsWith('/cron')) return '/cron';
    if (p.startsWith('/audit')) return '/audit';
    if (p.startsWith('/settings/llm')) return '/settings/llm';
    if (p.startsWith('/settings/archery')) return '/settings/archery';
    return '/';
  }, [location.pathname]);

  const menuItems = [
    { key: '/', icon: <DashboardOutlined />, label: '总览' },
    { key: '/chains', icon: <DeploymentUnitOutlined />, label: '规则链' },
    { key: '/runs', icon: <FileTextOutlined />, label: '运行日志' },
    { key: '/agents', icon: <RobotOutlined />, label: 'Agent' },
    { key: '/boards', icon: <ApartmentOutlined />, label: '看板' },
    { key: '/cron', icon: <ClockCircleOutlined />, label: '定时任务' },
    { key: '/skills', icon: <BookOutlined />, label: 'SKILL' },
    { key: '/mcp', icon: <ApiOutlined />, label: 'MCP' },
    { type: 'divider' as const },
    { key: '/settings/llm', icon: <SettingOutlined />, label: 'LLM 配置' },
    { key: '/settings/archery', icon: <DatabaseOutlined />, label: 'Archery 连接' },
    { key: '/audit', icon: <SafetyOutlined />, label: '审计日志' },
  ];

  const onLogout = async () => {
    await logout();
    navigate('/login');
  };

  const onChangePwd = async () => {
    const v = await form.validateFields();
    await authApi.changePassword(v.oldPassword, v.newPassword);
    message.success('密码已修改');
    setPwdOpen(false);
    form.resetFields();
    if (user) setUser({ ...user, mustChangePwd: false });
  };

  return (
    <Layout style={{ height: '100vh' }}>
      <Sider theme="dark" width={208}>
        <div className="bf-logo">
          <span className="dot" />
          BaboFlow
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[selectedKey]}
          items={menuItems}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header className="bf-topbar" style={{ padding: 0 }}>
          <div style={{ paddingLeft: 20, fontWeight: 600, color: '#1f1f1f' }}>
            规则链 · Agent · 看板 编排平台
          </div>
          <div className="right">
            <Dropdown
              menu={{
                items: [
                  { key: 'pwd', icon: <KeyOutlined />, label: '修改密码', onClick: () => setPwdOpen(true) },
                  { type: 'divider' },
                  { key: 'logout', icon: <LogoutOutlined />, label: '退出登录', onClick: onLogout },
                ],
              }}
            >
              <span style={{ cursor: 'pointer', color: '#333' }}>
                {user?.displayName || user?.username} <DownOutlined style={{ fontSize: 10 }} />
              </span>
            </Dropdown>
          </div>
        </Header>
        <Content style={{ overflow: 'auto', background: '#f5f6fa' }}>
          <Outlet />
        </Content>
      </Layout>

      <Modal
        title="修改密码"
        open={pwdOpen}
        onOk={onChangePwd}
        onCancel={() => setPwdOpen(false)}
        okText="确定"
        cancelText="取消"
        destroyOnClose
      >
        <Form form={form} layout="vertical" style={{ marginTop: 12 }}>
          <Form.Item name="oldPassword" label="原密码" rules={[{ required: true, message: '请输入原密码' }]}>
            <Input.Password placeholder="原密码" />
          </Form.Item>
          <Form.Item
            name="newPassword"
            label="新密码"
            rules={[
              { required: true, message: '请输入新密码' },
              { min: 8, message: '至少 8 位' },
            ]}
          >
            <Input.Password placeholder="至少 8 位" />
          </Form.Item>
        </Form>
      </Modal>
    </Layout>
  );
}
