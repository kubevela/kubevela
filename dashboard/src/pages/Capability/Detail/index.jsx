import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Space, Button, Row, Col, message, Spin, Breadcrumb, Modal } from 'antd';
import { Link } from 'umi';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

const { confirm } = Modal;

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
      capabilityCenterName: '',
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
        let capabilityCenterName = '';
        if (this.props.location.state) {
          capabilityCenterName = _.get(this.props, 'location.state.name', '');
          sessionStorage.setItem('capabilityCenterName', capabilityCenterName);
        } else {
          capabilityCenterName = sessionStorage.getItem('capabilityCenterName');
        }
        this.setState({
          capabilityCenterName,
        });
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
    const { capabilityCenterName } = this.state;
    const res = await this.props.dispatch({
      type: 'capability/syncOneCapability',
      payload: {
        capabilityCenterName,
        capabilityName: name,
      },
    });
    if (res) {
      message.success(res);
      this.getInitialData();
      await this.props.dispatch({
        type: 'menus/getMenuData',
      });
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
        this.getInitialData();
        await this.props.dispatch({
          type: 'menus/getMenuData',
        });
      }
    }
  };

  syncAllSignle = async () => {
    const { capabilityCenterName } = this.state;
    if (capabilityCenterName) {
      const res = await this.props.dispatch({
        type: 'capability/syncCapability',
        payload: {
          capabilityCenterName,
        },
      });
      if (res) {
        message.success(res);
        this.getInitialData();
        await this.props.dispatch({
          type: 'menus/getMenuData',
        });
      }
    }
  };

  showDeleteConfirm = () => {
    // eslint-disable-next-line
    const _this = this;
    const { capabilityCenterName } = this.state;
    if (capabilityCenterName) {
      confirm({
        title: `Are you sure delete ${capabilityCenterName}?`,
        icon: <ExclamationCircleOutlined />,
        width: 500,
        content: (
          <div>
            <p style={{ margin: '0px' }}>您本次移除 {capabilityCenterName}，将会删除的应用列表：</p>
            <p style={{ margin: '0px' }}>
              确认后，移除 {capabilityCenterName}，并且删除相应的应用？
            </p>
          </div>
        ),
        okText: 'Yes',
        okType: 'danger',
        cancelText: 'No',
        async onOk() {
          const res = await _this.props.dispatch({
            type: 'capability/deleteCapability',
            payload: {
              capabilityCenterName,
            },
          });
          if (res) {
            message.success(res);
            _this.props.history.push({ pathname: '/Capability' });
          }
        },
        onCancel() {
          // console.log('Cancel');
        },
      });
    }
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
