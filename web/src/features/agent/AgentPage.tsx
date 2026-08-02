import { useCallback, useEffect, useState } from 'react';
import { Avatar, Button, Card, Col, Drawer, Empty, Input, Row, Spin, Tag } from 'antd';
import { RobotOutlined, MessageOutlined, SearchOutlined, ReloadOutlined, ToolOutlined } from '@ant-design/icons';

import * as api from '@/api/agent';
import AgentChat from './AgentChat';

// Agent 页：卡片网格（含内置通用助手）+ 点击打开对话抽屉。
export default function AgentPage() {
  const [list, setList] = useState<api.Agent[]>([]);
  const [loading, setLoading] = useState(false);
  const [keyword, setKeyword] = useState('');
  const [chatAgent, setChatAgent] = useState<api.Agent | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const { list } = await api.listAgents(keyword || undefined);
      setList(list || []);
    } catch {
      /* 拦截器已提示 */
    } finally {
      setLoading(false);
    }
  }, [keyword]);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div style={{ padding: 24 }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 16 }}>
        <Input
          placeholder="搜索 Agent"
          prefix={<SearchOutlined />}
          allowClear
          style={{ width: 280 }}
          onChange={(e) => setKeyword(e.target.value)}
        />
        <Button icon={<ReloadOutlined />} onClick={load}>
          刷新
        </Button>
      </div>

      <Spin spinning={loading}>
        {list.length === 0 && !loading ? (
          <Empty description="暂无 Agent" />
        ) : (
          <Row gutter={[16, 16]}>
            {list.map((a) => (
              <Col key={a.key} xs={24} sm={12} md={8} lg={6}>
                <Card
                  hoverable
                  actions={[
                    <Button
                      key="chat"
                      type="link"
                      icon={<MessageOutlined />}
                      onClick={() => setChatAgent(a)}
                      disabled={!a.enabled}
                    >
                      对话
                    </Button>,
                  ]}
                >
                  <Card.Meta
                    avatar={
                      <Avatar size={44} icon={<RobotOutlined />} style={{ background: a.isBuiltin ? '#722ed1' : '#1677ff' }} />
                    }
                    title={
                      <span>
                        {a.name}{' '}
                        {a.isBuiltin && <Tag color="purple">内置</Tag>}
                        {!a.enabled && <Tag>停用</Tag>}
                      </span>
                    }
                    description={
                      <div>
                        <div className="agent-desc">{a.instruction || '无描述'}</div>
                        <div style={{ marginTop: 8 }}>
                          {(a.builtinTools || []).slice(0, 5).map((t) => (
                            <Tag key={t} icon={<ToolOutlined />} style={{ fontSize: 11 }}>
                              {t}
                            </Tag>
                          ))}
                        </div>
                      </div>
                    }
                  />
                </Card>
              </Col>
            ))}
          </Row>
        )}
      </Spin>

      <Drawer
        title={
          chatAgent ? (
            <span>
              <RobotOutlined style={{ marginRight: 8, color: '#722ed1' }} />
              {chatAgent.name}
            </span>
          ) : (
            ''
          )
        }
        width={920}
        open={!!chatAgent}
        onClose={() => setChatAgent(null)}
        destroyOnClose
        styles={{ body: { padding: 0 } }}
      >
        {chatAgent && <AgentChat agent={chatAgent} />}
      </Drawer>
    </div>
  );
}
