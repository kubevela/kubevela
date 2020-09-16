import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Space, Button, Row, Col, message, Spin, Breadcrumb } from 'antd';
import { Link } from 'umi';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.capability,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      workloadList: [],
      traitList: [],
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  getInitialData = async () => {
    const res = await this.props.dispatch({
      type: 'capability/capabilityList',
    });
    if (res) {
      const workloadList = [];
      const traitList = [];
      if (Array.isArray(res)) {
        const capabilityCenterName = _.get(this.props, 'location.state.name', '');
        res.forEach((item) => {
          if (item.center === capabilityCenterName) {
            if (item.type === 'workload') {
              workloadList.push(item);
            } else if (item.type === 'trait') {
              traitList.push(item);
            }
          }
        });
        this.setState({
          workloadList,
          traitList,
        });
      }
    }
  };

  gotoOtherPage = () => {
    // window.open('https://github.com/oam-dev/catalog/blob/master/workloads/cloneset/README.md');
  };

  installSignle = async (e, name) => {
    e.stopPropagation();
    const capabilityCenterName = _.get(this.props, 'location.state.name', '');
    const res = await this.props.dispatch({
      type: 'capability/syncOneCapability',
      payload: {
        capabilityCenterName,
        capabilityName: name,
      },
    });
    if (res) {
      message.success(res);
      window.location.reload();
    }
  };

  uninstallSignle = async (e, name) => {
    e.stopPropagation();
    if (name) {
      const res = await this.props.dispatch({
        type: 'capability/deleteOneCapability',
        payload: {
          capabilityName: name,
        },
      });
      if (res) {
        message.success(res);
        window.location.reload();
      }
    }
  };

  syncAllSignle = async () => {
    const capabilityCenterName = _.get(this.props, 'location.state.name', '');
    if (capabilityCenterName) {
      const res = await this.props.dispatch({
        type: 'capability/syncCapability',
        payload: {
          capabilityCenterName,
        },
      });
      if (res) {
        message.success(res);
        window.location.reload();
      }
    }
  };

  showDeleteConfirm = () => {
    message.info('正在开发中...');
  };

  render() {
    const { workloadList = [], traitList = [] } = this.state;
    let { loadingAll } = this.props;
    loadingAll = loadingAll || false;
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>
              <Link to="/Capability">Capability</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Detail</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Spin spinning={loadingAll}>
            <div style={{ marginBottom: '16px' }}>
              <Space>
                <Button type="primary" onClick={this.syncAllSignle}>
                  Install all
                </Button>
                <Button type="default" onClick={this.showDeleteConfirm}>
                  Remove
                </Button>
              </Space>
            </div>
            <div>
              <h3>Workloads</h3>
              <Row>
                {workloadList.length ? (
                  workloadList.map((item) => {
                    return (
                      <Col span="4" key={item.name}>
                        <div className="itemBox" onClick={this.gotoOtherPage}>
                          <div className="title">{item.name.substr(0, 3).toUpperCase()}</div>
                          <p>{item.name}</p>
                          {item.status === 'installed' ? (
                            <Button onClick={(e) => this.uninstallSignle(e, item.name)}>
                              uninstall
                            </Button>
                          ) : (
                            <Button
                              onClick={(e) => this.installSignle(e, item.name)}
                              type="primary"
                              ghost
                            >
                              install
                            </Button>
                          )}
                        </div>
                      </Col>
                    );
                  })
                ) : (
                  <Fragment>
                    <div>暂无可用的workload</div>
                  </Fragment>
                )}
              </Row>
            </div>
            <div>
              <h3>Traits</h3>
              <Row>
                {traitList.length ? (
                  traitList.map((item) => {
                    return (
                      <Col span="4" key={item.name}>
                        <div className="itemBox" onClick={this.gotoOtherPage}>
                          <div className="title">{item.name.substr(0, 3).toUpperCase()}</div>
                          <p>{item.name}</p>
                          {item.status === 'installed' ? (
                            <Button onClick={(e) => this.uninstallSignle(e, item.name)}>
                              uninstall
                            </Button>
                          ) : (
                            <Button
                              onClick={(e) => this.installSignle(e, item.name)}
                              type="primary"
                              ghost
                            >
                              install
                            </Button>
                          )}
                        </div>
                      </Col>
                    );
                  })
                ) : (
                  <Fragment>
                    <div>暂无可用的trait</div>
                  </Fragment>
                )}
              </Row>
            </div>
          </Spin>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
