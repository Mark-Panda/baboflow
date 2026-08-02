import { Card, Col, Row, Statistic, Empty } from 'antd';
import { DeploymentUnitOutlined, RobotOutlined, ThunderboltOutlined, CheckCircleOutlined } from '@ant-design/icons';

export default function DashboardPage() {
  return (
    <div className="bf-page">
      <h2 style={{ marginTop: 0 }}>总览</h2>
      <Row gutter={[16, 16]}>
        <Col xs={12} md={6}>
          <Card><Statistic title="规则链" value={0} prefix={<DeploymentUnitOutlined />} /></Card>
        </Col>
        <Col xs={12} md={6}>
          <Card><Statistic title="Agent" value={0} prefix={<RobotOutlined />} /></Card>
        </Col>
        <Col xs={12} md={6}>
          <Card><Statistic title="今日运行" value={0} prefix={<ThunderboltOutlined />} /></Card>
        </Col>
        <Col xs={12} md={6}>
          <Card><Statistic title="成功率" value={0} suffix="%" prefix={<CheckCircleOutlined />} /></Card>
        </Col>
      </Row>
      <Card style={{ marginTop: 16 }} title="最近运行">
        <Empty description="统计数据将在后续里程碑接入" />
      </Card>
    </div>
  );
}
