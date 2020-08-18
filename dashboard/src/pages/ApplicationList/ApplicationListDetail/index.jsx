import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import './index.less';
import { Button, Row, Col, Tabs, Popconfirm, message } from 'antd';

const { TabPane } = Tabs;

class TableList extends React.Component {
  confirm = (e) => {
    e.stopPropagation();
    message.success('Click on Yes');
  };

  cancel = (e) => {
    e.stopPropagation();
    message.error('Click on No');
  };

  createTrait = () => {
    this.props.history.push({
      pathname: '/ApplicationList/CreateApplication',
      state: { activeStep: 1, TraitType: 'Autoscaling' },
    });
  };

  hrefClick = (e) => {
    e.stopPropagation();
  };

  gotoWorkloadDetail = () => {
    this.props.history.push({ pathname: '/Workload/Detail' });
  };

  gotoTraitDetail = () => {
    this.props.history.push({ pathname: '/Traits/Detail' });
  };

  render() {
    return (
      <PageContainer>
        <div className="card-container app-detial">
          <h2>app-foo</h2>
          <p style={{ marginBottom: '20px' }}>
            core.oam.dev/v1alpha2, Kind=ApplicationConfiguration
          </p>
          <Tabs>
            <TabPane tab="Summary" key="1">
              <Row>
                <Col span="11">
                  <div className="summaryBox1" onClick={this.gotoWorkloadDetail}>
                    <Row>
                      <Col span="22">
                        <p className="title">Deployment</p>
                        <p>apps/v1</p>
                      </Col>
                      <Col span="2">
                        {/* <a href="JavaScript:;">?</a> */}
                        <p className="title hasCursor" onClick={this.hrefClick}>
                          ?
                        </p>
                      </Col>
                    </Row>
                    <p className="title">
                      Name:<span>app-foo-comp-v02wqwd</span>
                    </p>
                    <p className="title">Settings:</p>
                    <p>#可编辑</p>
                    <Row>
                      <Col span="8">
                        <p>Deployment Strategy</p>
                        <p>Rolling Update Strategy</p>
                        <p>Min Ready Seconds</p>
                        <p>Revision History Limit</p>
                        <p>Replicas</p>
                      </Col>
                      <Col span="16">
                        <p>RollingUpdate</p>
                        <p>Max Surge 25%, Max Unavaiable 25%</p>
                        <p>0</p>
                        <p>10</p>
                        <p>0</p>
                      </Col>
                    </Row>
                  </div>
                  <div className="summaryBox2">
                    <p className="title">Status:</p>
                    <Row>
                      <Col span="8">
                        <p>Available Replicas</p>
                        <p>Ready Replicas</p>
                      </Col>
                      <Col span="16">
                        <p>1</p>
                        <p>1</p>
                      </Col>
                    </Row>
                  </div>
                  <Popconfirm
                    title="Are you sure delete this task?"
                    onConfirm={this.confirm}
                    onCancel={this.cancel}
                    okText="Yes"
                    cancelText="No"
                  >
                    <Button danger>Delete</Button>
                  </Popconfirm>
                </Col>
                <Col span="1" />
                <Col span="10">
                  <div className="summaryBox" onClick={this.gotoTraitDetail}>
                    <Row>
                      <Col span="22">
                        <p className="title">Autoscaling</p>
                        <p>core.oam.dev/v1alpha2</p>
                      </Col>
                      <Col span="2">
                        <p
                          className="title hasCursor"
                          onClick={(e) => {
                            e.stopPropagation();
                          }}
                        >
                          ?
                        </p>
                      </Col>
                    </Row>
                    <p className="title">Properties:</p>
                    <p>#可编辑</p>
                    <Row>
                      <Col span="8">
                        <p>Min</p>
                        <p>Max</p>
                      </Col>
                      <Col span="16">
                        <p>1</p>
                        <p>100</p>
                      </Col>
                    </Row>
                    <div style={{ clear: 'both', height: '32px' }}>
                      <Popconfirm
                        title="Are you sure delete this task?"
                        onConfirm={this.confirm}
                        onCancel={this.cancel}
                        okText="Yes"
                        cancelText="No"
                      >
                        <Button
                          danger
                          className="floatRight"
                          onClick={(e) => {
                            e.stopPropagation();
                          }}
                        >
                          Delete
                        </Button>
                      </Popconfirm>
                    </div>
                  </div>
                  {/* <div className="summaryBox">
                    <Row>
                      <Col span="22">
                        <p className="title">Autoscaling</p>
                        <p>core.oam.dev/v1alpha2</p>
                      </Col>
                      <Col span="2">
                        <p
                          className="title hasCursor"
                        >
                          ?
                        </p>
                      </Col>
                    </Row>
                    <p className="title">Properties:</p>
                    <p>#可编辑</p>
                    <Row>
                      <Col span="8">
                        <p>Min</p>
                        <p>Max</p>
                      </Col>
                      <Col span="16">
                        <p>1</p>
                        <p>100</p>
                      </Col>
                    </Row>
                    <div style={{ clear: "both", height: "32px" }}>
                      <Popconfirm
                        title="Are you sure delete this task?"
                        onConfirm={this.confirm}
                        onCancel={this.cancel}
                        okText="Yes"
                        cancelText="No"
                      >
                        <Button danger className="floatRight">
                          Delete
                        </Button>
                      </Popconfirm>
                    </div>
                  </div> */}
                  <p
                    className="hasCursor"
                    style={{
                      fontSize: '30px',
                    }}
                    onClick={this.createTrait}
                  >
                    +
                  </p>
                </Col>
              </Row>
            </TabPane>
            <TabPane tab="Topology" key="2">
              <p>Topology</p>
            </TabPane>
          </Tabs>
        </div>
      </PageContainer>
    );
  }
}

export default TableList;
