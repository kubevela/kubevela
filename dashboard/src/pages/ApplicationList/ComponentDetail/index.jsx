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
      compDetailData: {},
      visible: false,
      traitList: [],
      availableTraitList: [],
      envName: '',
      appName: '',
      compName: '',
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  UNSAFE_componentWillReceiveProps(nextProps) {
    if (this.props.compName !== nextProps.compName) {
      this.getInitialData(nextProps.compName);
    }
  }

  getInitialData = async (nextCompName) => {
    const appName = _.get(this.props, 'appName', '');
    const envName = _.get(this.props, 'envName', '');
    let compName = _.get(this.props, 'compName', '');
    compName = nextCompName || compName;
    if (appName && envName && compName) {
      this.setState({
        envName,
        appName,
        compName,
      });
      const res = await this.props.dispatch({
        type: 'components/getComponentDetail',
        payload: {
          envName,
          appName,
          compName,
        },
      });
      if (res) {
        this.setState({
          compDetailData: res,
        });
        const traits = await this.props.dispatch({
          type: 'trait/getTraits',
        });
        if (traits) {
          this.setState({
            traitList: traits,
          });
        }
        const workloadType = _.get(res, 'workload.kind', '');
        if (workloadType) {
          this.getAcceptTrait(workloadType.toLowerCase());
        }
        // if (workloadType && workloadType === '') {
        //   this.getAcceptTrait('containerized');
        // } else if (workloadType && workloadType === 'Deployment') {
        //   this.getAcceptTrait('deployment');
        // }
      }
    }
  };

  // getAcceptTrait = (workloadType) => {
  //   const res = this.state.traitList.filter((item) => {
  //     if (item.appliesTo) {
  //       if(item.appliesTo==='*'){
  //         return true;
  //       }
  //       if (item.appliesTo.indexOf(workloadType) !== -1) {
  //         return true;
  //       }
  //       return false;
  //     }
  //     return false;
  //   });
  //   this.setState(() => ({
  //     availableTraitList: res,
  //   }));
  // };

  getAcceptTrait = () => {
    const res = this.state.traitList;
    this.setState(() => ({
      availableTraitList: res,
    }));
  };

  deleteComp = async (e) => {
    e.stopPropagation();
    const { envName, appName, compName } = this.state;
    if (appName && envName && compName) {
      const res = await this.props.dispatch({
        type: 'components/deleteComponent',
        payload: {
          appName,
          envName,
          compName,
        },
      });
      if (res) {
        message.success(res);
        // 删除当前component成功后，刷新当前页面
        this.props.getInitCompList();
      }
    }
  };

  deleteTrait = async (e, item) => {
    e.stopPropagation();
    const { appName, envName, compName } = this.state;
    const traitNameObj = _.get(item, 'trait.metadata.annotations', '');
    const traitName = traitNameObj['vela.oam.dev/traitDef'] || traitNameObj['trait.oam.dev/name'];
    if (traitName && appName && envName && compName) {
      const res = await this.props.dispatch({
        type: 'trait/deleteOneTrait',
        payload: {
          envName,
          appName,
          traitName,
          compName,
        },
      });
      if (res) {
        message.success(res);
        this.getInitialData(compName);
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
      const { envName, appName, compName } = this.state;
      if (envName && appName && compName) {
        const res = await this.props.dispatch({
          type: 'trait/attachOneTraits',
          payload: {
            envName,
            appName,
            compName,
            params: submitObj,
          },
        });
        if (res) {
          this.setState({
            visible: false,
          });
          message.success(res);
          this.getInitialData(compName);
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
    const { compDetailData } = this.state;
    const status = _.get(compDetailData, 'status', '');
    const Workload = _.get(compDetailData, 'workload', {});
    const Traits = _.get(compDetailData, 'traits', []);
    let containers = {};
    if (_.get(Workload, 'kind', '') === 'Job') {
      containers = _.get(Workload, 'spec.template.spec.containers[0]', {});
    } else {
      containers = _.get(Workload, 'spec.podSpec.containers[0]', {});
    }
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
                        {_.get(Workload, 'kind', '') === 'Job' ? (
                          <Fragment>
                            <Col span="8">
                              <p>count</p>
                            </Col>
                            <Col span="16">
                              <p>
                                {_.get(Workload, 'spec.completions', '') ||
                                  _.get(Workload, 'spec.parallelism', '')}
                              </p>
                            </Col>
                          </Fragment>
                        ) : (
                          <Fragment />
                        )}
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
                      title="Are you sure delete this component?"
                      onConfirm={(e) => this.deleteComp(e)}
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
                        const spec = _.get(traitItem, 'spec', {});
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
                              {Object.keys(spec).map((currentKey) => {
                                if (spec[currentKey].constructor === Object) {
                                  const backend = _.get(spec, `${currentKey}`, {});
                                  return Object.keys(backend).map((currentKey1) => {
                                    return (
                                      <Fragment key={currentKey1}>
                                        <Col span="8">
                                          <p>{currentKey1}</p>
                                        </Col>
                                        <Col span="16">
                                          <p>{backend[currentKey1]}</p>
                                        </Col>
                                      </Fragment>
                                    );
                                  });
                                }
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
                              })}
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
