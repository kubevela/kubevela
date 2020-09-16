import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import './index.less';
import { Form, Input, Button, Row, Col, Tabs, Table, Spin, Breadcrumb } from 'antd';
import { CheckCircleOutlined } from '@ant-design/icons';
import { connect } from 'dva';
import { Link } from 'umi';
import _ from 'lodash';

const { TabPane } = Tabs;
const layout = {
  labelCol: {
    span: 8,
  },
  wrapperCol: {
    span: 16,
  },
};
const layout1 = {
  labelCol: {
    span: 4,
  },
  wrapperCol: {
    span: 16,
  },
};
const columns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
    render: (text) => (
      <div>
        <CheckCircleOutlined style={{ fontSize: '20px', color: '#4CAF51' }} />
        <a style={{ marginLeft: '6px' }}>{text}</a>
      </div>
    ),
  },
  {
    title: 'Ready',
    dataIndex: 'Ready',
    key: 'Ready',
  },
  {
    title: 'Phase',
    dataIndex: 'Phase',
    key: 'Phase',
  },
  {
    title: 'Restarts',
    dataIndex: 'Restarts',
    key: 'Restarts',
  },
  {
    title: 'Node',
    dataIndex: 'Node',
    key: 'Node',
  },
  {
    title: 'Age',
    dataIndex: 'Age',
    key: 'Age',
  },
];

const data = [
  {
    key: '1',
    name: 'cool-aryabhata-v0fnxq6-788767b7f5-9zx96',
    Ready: '1/2',
    Phase: 'Pending',
    Restarts: 0,
    Node: 'cn-hongkong.10.0.1.229',
    Age: '2d',
  },
];
const columns1 = [
  {
    title: 'Type',
    dataIndex: 'Type',
    key: 'Type',
  },
  {
    title: 'Reason',
    dataIndex: 'Reason',
    key: 'Reason',
  },
  {
    title: 'Status',
    dataIndex: 'Status',
    key: 'Status',
  },
  {
    title: 'Message',
    dataIndex: 'Message',
    key: 'Message',
  },
  {
    title: 'Last Update',
    dataIndex: 'LastUpdate',
    key: 'LastUpdate',
  },
  {
    title: 'Last Transition',
    dataIndex: 'LastTransition',
    key: 'LastTransition',
  },
];

const data1 = [
  {
    key: '1',
    Type: 'Available',
    Reason: 'MinimumReplicasUnavailable',
    Status: 'false',
    Message: 'Deployment does not have minimum availability.',
    LastUpdate: '2d',
    LastTransition: '2d',
  },
  {
    key: '2',
    Type: 'Progressing',
    Reason: 'ProgressDeadlineExceeded',
    Status: 'false',
    Message: 'ReplicaSet "cool-aryabhata-v0fnxq6-788767b7f5" has timed out progressing.',
    LastUpdate: '2d',
    LastTransition: '2d',
  },
];

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.PureComponent {
  formRefStep1 = React.createRef();

  formRefStep2 = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      hasShowEdit: false,
      hasShowEdit2: false,
      appDetailData: {},
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  async getInitialData() {
    const appName = _.get(this.props, 'location.state.appName', '');
    const envName = _.get(this.props, 'location.state.envName', '');
    if (appName && envName) {
      const res = await this.props.dispatch({
        type: 'applist/getAppDetail',
        payload: {
          envName,
          appName,
        },
      });
      if (res) {
        this.setState({
          appDetailData: res,
        });
      }
    }
  }

  onFinishStep1 = () => {
    this.setState({
      hasShowEdit: false,
    });
  };

  onFinishStep2 = () => {
    this.setState({
      hasShowEdit2: false,
    });
  };

  changeShowEdit = () => {
    this.setState((prev) => ({
      hasShowEdit: !prev.hasShowEdit,
    }));
  };

  changeShowEdit2 = () => {
    this.setState((prev) => ({
      hasShowEdit2: !prev.hasShowEdit2,
    }));
  };

  render() {
    const { hasShowEdit, hasShowEdit2 } = this.state;
    const status = _.get(this.state.appDetailData, 'Status', '');
    const Workload = _.get(this.state.appDetailData, 'Workload.workload', {});
    const metadata = _.get(Workload, 'metadata', {});
    let containers = {};
    containers = _.get(Workload, 'spec.containers[0]', {});
    let { loadingAll } = this.props;
    loadingAll = loadingAll || false;
    const appName = _.get(this.props, 'location.state.appName', '');
    const envName = _.get(this.props, 'location.state.envName', '');
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Applications</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>
              <Link
                to={{
                  pathname: '/ApplicationList/ApplicationListDetail',
                  state: { appName, envName },
                }}
              >
                ApplicationListDetail
              </Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>WorkloadDetail</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Spin spinning={loadingAll}>
            <div className="card-container workload-detail">
              <h2>{Workload.kind}</h2>
              <p style={{ marginBottom: '20px' }}>
                <i>
                  {Workload.apiVersion},Name={_.get(Workload, 'metadata.name', '')}
                </i>
              </p>
              <Tabs>
                <TabPane tab="Summary" key="1">
                  <div>
                    <Row>
                      <Col span="12">
                        <div className="hasBorder">
                          <div
                            className="hasPadding"
                            style={{ display: !hasShowEdit ? 'block' : 'none' }}
                          >
                            <p className="title">Configuration</p>
                            <Row>
                              {Object.keys(containers).map((currentKey) => {
                                if (currentKey === 'ports') {
                                  return (
                                    <Fragment key={currentKey}>
                                      <Col span="8">
                                        <p>port</p>
                                      </Col>
                                      <Col span="16">
                                        <p>
                                          {_.get(containers[currentKey], '[0].containerPort', '')}
                                        </p>
                                      </Col>
                                    </Fragment>
                                  );
                                  // eslint-disable-next-line no-else-return
                                } else if (currentKey === 'name') {
                                  return <Fragment key={currentKey} />;
                                }
                                return (
                                  <Fragment key={currentKey}>
                                    <Col span="8">
                                      <p>{currentKey}</p>
                                    </Col>
                                    <Col span="16">
                                      <p>{containers[currentKey]}</p>
                                    </Col>
                                  </Fragment>
                                );
                              })}
                            </Row>
                          </div>
                          <div
                            className="hasPadding"
                            style={{ display: hasShowEdit ? 'block' : 'none' }}
                          >
                            <p className="title">Deployment Editor</p>
                            <Form
                              labelAlign="left"
                              {...layout}
                              ef={this.formRefStep1}
                              name="control-ref"
                              onFinish={this.onFinishStep1}
                            >
                              <div className="relativeBox">
                                <Form.Item name="Replicas" label="Replicas">
                                  <Input type="number" />
                                </Form.Item>
                              </div>
                              <div style={{ marginBottom: '16px' }}>
                                <Button type="primary" htmlType="submit">
                                  Submit
                                </Button>
                                <Button
                                  style={{ marginLeft: '16px' }}
                                  onClick={this.changeShowEdit}
                                >
                                  Cancle
                                </Button>
                              </div>
                            </Form>
                          </div>
                          <div style={{ display: !hasShowEdit ? 'block' : 'none' }}>
                            <div
                              style={{ width: '100%', borderTop: '1px solid #eee', height: '0px' }}
                            />
                            <div>
                              <Button
                                className="textAlignLeft"
                                type="link"
                                block
                                onClick={this.changeShowEdit}
                              >
                                Edit
                              </Button>
                            </div>
                          </div>
                        </div>
                      </Col>
                      <Col span="1" />
                      <Col span="10">
                        <div className="hasBorder">
                          <div className="hasPadding">
                            <p className="title">Status</p>
                            <p>{status}</p>
                          </div>
                        </div>
                      </Col>
                    </Row>
                    <p className="title hasBG">Pods</p>
                    <Table columns={columns} dataSource={data} pagination={false} />
                    <p className="title hasBG">Conditions</p>
                    <Table columns={columns1} dataSource={data1} pagination={false} />
                    <p className="title hasBG">Pod Template</p>
                    <div className="hasBorder">
                      <div
                        className="hasPadding"
                        style={{ display: !hasShowEdit2 ? 'block' : 'none' }}
                      >
                        <p className="title">Container cool-aryabhata-v0fnxq6</p>
                        <Row>
                          <Col span="2">
                            <div style={{ color: 'black' }}>
                              <p>Image</p>
                              <p>Args</p>
                            </div>
                          </Col>
                          <Col>
                            <p>secret</p>
                            <p>[&apos;-h&apos;]</p>
                          </Col>
                        </Row>
                      </div>
                      <div
                        className="hasPadding"
                        style={{ display: hasShowEdit2 ? 'block' : 'none' }}
                      >
                        <p className="title">Deployment Editor</p>
                        <Form
                          style={{ width: '50%' }}
                          {...layout1}
                          labelAlign="left"
                          ef={this.formRefStep2}
                          name="control-ref"
                          onFinish={this.onFinishStep2}
                        >
                          <div className="relativeBox">
                            <Form.Item name="Image" label="Image">
                              <Input />
                            </Form.Item>
                          </div>
                          <div style={{ marginBottom: '16px' }}>
                            <Button type="primary" htmlType="submit">
                              Submit
                            </Button>
                            <Button style={{ marginLeft: '16px' }} onClick={this.changeShowEdit2}>
                              Cancle
                            </Button>
                          </div>
                        </Form>
                      </div>
                      <div style={{ display: !hasShowEdit2 ? 'block' : 'none' }}>
                        <div
                          style={{ width: '100%', borderTop: '1px solid #eee', height: '0px' }}
                        />
                        <div>
                          <Button
                            className="textAlignLeft"
                            type="link"
                            block
                            onClick={this.changeShowEdit2}
                          >
                            Edit
                          </Button>
                        </div>
                      </div>
                    </div>
                  </div>
                </TabPane>
                <TabPane tab="Metadata" key="2">
                  <div className="hasBorder">
                    <div className="hasPadding">
                      <p className="title">Metadata</p>
                      {Object.keys(metadata).map((currentKey6) => {
                        return (
                          <Row key={currentKey6}>
                            <Col span="4">
                              <p>{currentKey6}</p>
                            </Col>
                            <Col>
                              <p>{metadata[currentKey6]}</p>
                            </Col>
                          </Row>
                        );
                      })}
                    </div>
                  </div>
                </TabPane>
                <TabPane tab="Resource Viewer" key="3">
                  <p>Resource Viewer</p>
                </TabPane>
                <TabPane tab="YAML" key="4">
                  <p>YAML</p>
                </TabPane>
              </Tabs>
            </div>
          </Spin>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
