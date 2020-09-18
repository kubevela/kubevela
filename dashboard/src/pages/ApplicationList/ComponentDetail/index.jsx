import React, { Fragment } from 'react';
import './index.less';
import { Button, Row, Col, Tabs, Popconfirm, message, Tooltip, Modal, Spin } from 'antd';
import { connect } from 'dva';
import _ from 'lodash';
import CreateTraitItem from '../../../components/AttachOneTrait/index.jsx';

const { TabPane } = Tabs;

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      appDetailData: {},
      visible: false,
      traitList: [],
      availableTraitList: [],
      envName: '',
      appName: '',
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  UNSAFE_componentWillReceiveProps(nextProps) {
    if (this.props.appName !== nextProps.appName) {
      this.getInitialData(nextProps.appName);
    }
  }

  getInitialData = async (nextAppName) => {
    let appName = _.get(this.props, 'appName', '');
    const envName = _.get(this.props, 'envName', '');
    appName = nextAppName || appName;
    if (appName && envName) {
      this.setState({
        envName,
        appName,
      });
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
      const traits = await this.props.dispatch({
        type: 'trait/getTraits',
      });
      if (traits) {
        this.setState({
          traitList: traits,
        });
      }
      const workloadType = _.get(res, 'Workload.workload.kind', '');
      if (workloadType && workloadType === 'ContainerizedWorkload') {
        this.getAcceptTrait('containerized');
      } else if (workloadType && workloadType === 'Deployment') {
        this.getAcceptTrait('deployment');
      }
    }
  };

  getAcceptTrait = (workloadType) => {
    const res = this.state.traitList.filter((item) => {
      if (item.appliesTo.indexOf(workloadType) !== -1) {
        return true;
      }
      return false;
    });
    this.setState(() => ({
      availableTraitList: res,
    }));
  };

  deleteApp = async (e) => {
    e.stopPropagation();
    const { envName } = this.state;
    const { appDetailData } = this.state;
    const appName = _.get(appDetailData, 'Workload.workload.metadata.name', '');
    if (appName && envName) {
      const res = await this.props.dispatch({
        type: 'applist/deleteApp',
        payload: {
          appName,
          envName,
        },
      });
      if (res) {
        message.success(res);
        this.props.history.push({ pathname: '/ApplicationList' });
      }
    }
  };

  deleteTrait = async (e, item) => {
    e.stopPropagation();
    const { appName, envName } = this.state;
    const traitNameObj = _.get(item, 'trait.metadata.annotations', '');
    const traitName = traitNameObj['vela.oam.dev/traitDef'] || traitNameObj['trait.oam.dev/name'];
    if (traitName && appName && envName) {
      const res = await this.props.dispatch({
        type: 'trait/deleteOneTrait',
        payload: {
          envName,
          appName,
          traitName,
        },
      });
      if (res) {
        message.success(res);
        this.getInitialData(2);
      }
    }
  };

  cancel = (e) => {
    e.stopPropagation();
  };

  createTrait = async () => {
    await this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
    await this.child.validateFields();
    const submitData = this.child.getSelectValue();
    if (submitData.name) {
      const submitObj = {
        name: submitData.name,
        flags: [],
      };
      Object.keys(submitData).forEach((currentKey) => {
        if (currentKey !== 'name' && submitData[currentKey]) {
          submitObj.flags.push({
            name: currentKey,
            value: submitData[currentKey].toString(),
          });
        }
      });
      const { envName, appName } = this.state;
      if (envName && appName) {
        const res = await this.props.dispatch({
          type: 'trait/attachOneTraits',
          payload: {
            envName,
            appName,
            params: submitObj,
          },
        });
        if (res) {
          this.setState({
            visible: false,
          });
          message.success(res);
          this.getInitialData(2);
        }
      }
    } else {
      message.warning('please select a trait type');
    }
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  hrefClick = (e) => {
    e.stopPropagation();
  };

  gotoWorkloadDetail = (e) => {
    e.stopPropagation();
  };

  gotoTraitDetail = (e) => {
    e.stopPropagation();
  };

  render() {
    const status = _.get(this.state.appDetailData, 'Status', '');
    const Workload = _.get(this.state.appDetailData, 'Workload.workload', {});
    const Traits = _.get(this.state.appDetailData, 'Traits', []);
    let containers = {};
    containers = _.get(Workload, 'spec.containers[0]', {});
    let { loadingAll } = this.props;
    loadingAll = loadingAll || false;
    const colorObj = {
      Deployed: '#4CAF51',
      Staging: '#F44337',
      UNKNOWN: '#1890ff',
    };
    return (
      <div style={{ margin: '8px' }}>
        <Spin spinning={loadingAll}>
          <div className="card-container app-detial">
            <h2>{_.get(Workload, 'metadata.name')}</h2>
            <p style={{ marginBottom: '20px' }}>
              {Workload.apiVersion}, Kind={Workload.kind}
            </p>
            <Tabs>
              <TabPane tab="Summary" key="1">
                <Row>
                  <Col span="11">
                    <div
                      className="summaryBox1"
                      onClick={(e) => this.gotoWorkloadDetail(e)}
                      style={{ background: colorObj[status] || '#1890ff' }}
                    >
                      <Row>
                        <Col span="22">
                          <p className="title">{Workload.kind}</p>
                          <p>{Workload.apiVersion}</p>
                        </Col>
                        <Col span="2">
                          <p className="title hasCursor" onClick={this.hrefClick}>
                            ?
                          </p>
                        </Col>
                      </Row>
                      <p className="title">
                        Name:<span>{_.get(Workload, 'metadata.name')}</span>
                      </p>
                      <p className="title">Settings:</p>
                      <Row>
                        {Object.keys(containers).map((currentKey) => {
                          if (currentKey === 'ports') {
                            return (
                              <Fragment key={currentKey}>
                                <Col span="8">
                                  <p>port</p>
                                </Col>
                                <Col span="16">
                                  <p>{_.get(containers[currentKey], '[0].containerPort', '')}</p>
                                </Col>
                              </Fragment>
                            );
                            // eslint-disable-next-line no-else-return
                          } else if (currentKey === 'name') {
                            return <Fragment key={currentKey} />;
                            // eslint-disable-next-line no-else-return
                          } else if (currentKey === 'env') {
                            return (
                              <Fragment key={currentKey}>
                                <Col span="8">
                                  <p>env</p>
                                </Col>
                                <Col span="16">
                                  <p>{_.get(containers[currentKey], '[0].value', '')}</p>
                                </Col>
                              </Fragment>
                            );
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
                    <Popconfirm
                      title="Are you sure delete this app?"
                      onConfirm={(e) => this.deleteApp(e)}
                      onCancel={this.cancel}
                      okText="Yes"
                      cancelText="No"
                    >
                      <Button danger>Delete</Button>
                    </Popconfirm>
                  </Col>
                  <Col span="1" />
                  <Col span="10">
                    {Traits.length ? (
                      Traits.map((item, index) => {
                        const traitItem = _.get(item, 'trait', {});
                        const annotations = _.get(traitItem, 'metadata.annotations', {});
                        let traitType = 1;
                        const spec = _.get(traitItem, 'spec', {});
                        if (traitItem.kind === 'Ingress') {
                          traitType = 2;
                        }
                        return (
                          <div
                            className="summaryBox"
                            onClick={(e) => this.gotoTraitDetail(e, traitItem)}
                            key={index.toString()}
                          >
                            <Row>
                              <Col span="22">
                                <p className="title">{traitItem.kind}</p>
                                <p>{traitItem.apiVersion}</p>
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
                            <Row>
                              {Object.keys(annotations).map((currentKey3) => {
                                return (
                                  <Fragment key={currentKey3}>
                                    <Col span="8">
                                      <p>{currentKey3}:</p>
                                    </Col>
                                    <Col span="8">
                                      <p>{annotations[currentKey3]}</p>
                                    </Col>
                                  </Fragment>
                                );
                              })}
                            </Row>
                            <p className="title">Properties:</p>
                            <Row>
                              {traitType === 2 ? (
                                <Fragment>
                                  <Col span="8">
                                    <p>domain</p>
                                  </Col>
                                  <Col span="16">
                                    <p>{_.get(spec, 'rules[0].host', '')}</p>
                                  </Col>
                                  <Col span="8">
                                    <p>service</p>
                                  </Col>
                                  <Col span="16">
                                    <p>
                                      {_.get(
                                        spec,
                                        'rules[0].http.paths[0].backend.serviceName',
                                        '',
                                      )}
                                    </p>
                                  </Col>
                                  <Col span="8">
                                    <p>port</p>
                                  </Col>
                                  <Col span="16">
                                    <p>
                                      {_.get(
                                        spec,
                                        'rules[0].http.paths[0].backend.servicePort',
                                        '',
                                      )}
                                    </p>
                                  </Col>
                                </Fragment>
                              ) : (
                                Object.keys(spec).map((currentKey) => {
                                  return (
                                    <Fragment key={currentKey}>
                                      <Col span="8">
                                        <p>{currentKey}</p>
                                      </Col>
                                      <Col span="16">
                                        <p>{spec[currentKey]}</p>
                                      </Col>
                                    </Fragment>
                                  );
                                })
                              )}
                            </Row>
                            <div style={{ clear: 'both', height: '32px' }}>
                              <Popconfirm
                                title="Are you sure delete this trait?"
                                onConfirm={(e) => this.deleteTrait(e, item)}
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
                        );
                      })
                    ) : (
                      <Fragment />
                    )}
                    <Tooltip placement="top" title="Attach Trait">
                      <p
                        className="hasCursor"
                        style={{
                          fontSize: '30px',
                          display: 'inline-flex',
                        }}
                        onClick={this.createTrait}
                      >
                        +
                      </p>
                    </Tooltip>
                  </Col>
                </Row>
              </TabPane>
              <TabPane tab="Topology" key="2">
                <p>Topology</p>
              </TabPane>
            </Tabs>
          </div>
          <Modal
            title="Attach a Trait"
            visible={this.state.visible}
            onOk={this.handleOk}
            onCancel={this.handleCancel}
            footer={[
              <Button key="back" onClick={this.handleCancel}>
                Cancel
              </Button>,
              <Button key="submit" type="primary" onClick={this.handleOk}>
                Confirm
              </Button>,
            ]}
          >
            <CreateTraitItem
              onRef={(ref) => {
                this.child = ref;
              }}
              availableTraitList={this.state.availableTraitList}
              initialValues={{}}
            />
          </Modal>
        </Spin>
      </div>
    );
  }
}

export default TableList;
