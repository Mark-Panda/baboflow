import { useEffect, useState } from 'react';
import { Button, Card, Form, Input, App } from 'antd';
import { LockOutlined, UserOutlined } from '@ant-design/icons';
import { useLocation, useNavigate } from 'react-router-dom';

import { authApi } from '@/api/auth';
import { useAuthStore } from '@/stores/authStore';

export default function LoginPage() {
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();
  const location = useLocation() as { state?: { from?: { pathname?: string } } };
  const { message } = App.useApp();
  const setUser = useAuthStore((s) => s.setUser);
  const user = useAuthStore((s) => s.user);

  useEffect(() => {
    if (user) navigate('/', { replace: true });
  }, [user, navigate]);

  const onFinish = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      const u = await authApi.login(values.username, values.password);
      setUser(u);
      message.success(`欢迎，${u.displayName || u.username}`);
      const to = location.state?.from?.pathname || '/';
      navigate(to, { replace: true });
    } catch {
      // 错误已在拦截器提示
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      style={{
        height: '100vh',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        background: 'linear-gradient(135deg, #1b2340 0%, #2a3560 50%, #3b2f63 100%)',
      }}
    >
      <Card
        style={{ width: 380, boxShadow: '0 12px 40px rgba(0,0,0,0.35)', borderRadius: 12 }}
        styles={{ body: { padding: '36px 36px 28px' } }}
      >
        <div style={{ textAlign: 'center', marginBottom: 28 }}>
          <div
            style={{
              width: 46,
              height: 46,
              borderRadius: 12,
              margin: '0 auto 12px',
              background: 'linear-gradient(135deg,#4f8cff,#7c5cff)',
            }}
          />
          <div style={{ fontSize: 22, fontWeight: 700 }}>BaboFlow</div>
          <div style={{ color: '#888', fontSize: 13, marginTop: 4 }}>规则链 + Agent 编排平台</div>
        </div>
        <Form onFinish={onFinish} size="large">
          <Form.Item name="username" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<UserOutlined />} placeholder="用户名" autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="密码" autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block loading={loading}>
            登 录
          </Button>
        </Form>
      </Card>
    </div>
  );
}
