import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import './index.less';
import { Button, Row, Col, Tabs, Popconfirm, message, Tooltip, Modal } from 'antd';
import { connect } from 'dva';
import _ from 'lodash';
import CreateTraitItem from '../../../components/AttachOneTrait/index.jsx';
import Topology from './Topology.jsx';

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

  getInitialData = async () => {
    const appName = _.get(this.props, 'location.state.appName', '');
    const envName = _.get(this.props, 'location.state.envName', '');
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
      this.setState({
        traitList: traits,
      });
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
    const { currentEnv: envName } = this.props;
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
    const kind = _.get(item, 'trait.kind', '');
    let traitName = '';
    if (kind && kind === 'SimpleRolloutTrait') {
      traitName = 'rollout';
    } else if (kind && kind === 'ManualScalerTrait') {
      traitName = 'scale';
    }
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
        this.getInitialData();
      }
    }
  };

  cancel = (e) => {
    e.stopPropagation();
  };

  createTrait = async () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
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
          this.getInitialData();
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

  gotoWorkloadDetail = () => {
    this.props.history.push({ pathname: '/Workload/Detail' });
  };

  gotoTraitDetail = () => {
    this.props.history.push({ pathname: '/Traits/Detail' });
  };

  render() {
    const status = _.get(this.state.appDetailData, 'Status', '');
    const Workload = _.get(this.state.appDetailData, 'Workload.workload', {});
    const Traits = _.get(this.state.appDetailData, 'Traits', []);
    const containers = _.get(Workload, 'spec.containers[0]', {});
    const ports = _.get(Workload, 'spec.containers[0].ports[0]', {});
    return (
      <PageContainer>
        <div className="card-container app-detial">
          <h2>{_.get(Workload, 'metadata.name')}</h2>
          <p style={{ marginBottom: '20px' }}>
            {Workload.apiVersion}, Kind={Workload.kind}
          </p>
          <Tabs>
            <TabPane tab="Summary" key="1">
              <Row>
                <Col span="11">
                  <div className="summaryBox1" onClick={this.gotoWorkloadDetail}>
                    <Row>
                      <Col span="22">
                        <p className="title">{Workload.kind}</p>
                        <p>{Workload.apiVersion}</p>
                      </Col>
                      <Col span="2">
                        {/* <a href="JavaScript:;">?</a> */}
                        <p className="title hasCursor" onClick={this.hrefClick}>
                          ?
                        </p>
                      </Col>
                    </Row>
                    <p className="title">
                      Name:<span>{_.get(Workload, 'metadata.name')}</span>
                    </p>
                    <p className="title">Settings:</p>
                    <p>#可编辑</p>
                    <Row>
                      {Object.keys(containers).map((currentKey) => {
                        if (currentKey !== 'ports') {
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
                        }
                        return Object.keys(ports).map((currentKey1) => {
                          return (
                            <Fragment key={currentKey1}>
                              <Col span="8">
                                <p>{currentKey1}</p>
                              </Col>
                              <Col span="16">
                                <p>{ports[currentKey1]}</p>
                              </Col>
                            </Fragment>
                          );
                        });
                      })}
                    </Row>
                  </div>
                  <div className="summaryBox2">
                    <p className="title">Status:</p>
                    <p>{status}</p>
                    {/* <Row>
                      <Col span="8">
                        <p>Available Replicas</p>
                        <p>Ready Replicas</p>
                      </Col>
                      <Col span="16">
                        <p>1</p>
                        <p>1</p>
                      </Col>
                    </Row> */}
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
                      // const spec = _.get(traitItem, 'spec', {});
                      const annotations = _.get(traitItem, 'metadata.annotations', {});
                      // const traitPorts =  _.get(Workload, 'spec.containers[0].ports[0]',{});
                      return (
                        <div
                          className="summaryBox"
                          onClick={this.gotoTraitDetail}
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
                          <p>#可编辑</p>
                          <Row>
                            {/* {Object.keys(spec).map((currentKey) => {
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
                            })} */}
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
              {/* <p>Topology</p> */}
              <Topology />
            </TabPane>
          </Tabs>
        </div>
        <Modal
          title="attach a trait"
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
      </PageContainer>
    );
  }
}

export default TableList;
