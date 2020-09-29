import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import './index.less';
import { Button, Row, Col, Form, Input, Select, Steps, message, Breadcrumb } from 'antd';
import { connect } from 'dva';
import { Link } from 'umi';
import _ from 'lodash';
import CreateTraitItem from '../createTrait/index.jsx';

const { Option } = Select;
const { Step } = Steps;

const layout = {
  labelCol: {
    span: 8,
  },
  wrapperCol: {
    span: 16,
  },
};

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.workload,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.Component {
  formRefStep1 = React.createRef();

  formRefStep2All = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      current: 0,
      isShowMore: false,
      traitNum: [
        {
          refname: null,
          initialData: {},
          uniq: new Date().valueOf(),
        },
      ],
      traitList: [],
      availableTraitList: [],
      workloadList: [],
      workloadSettings: [],
      step1SubmitObj: {},
      step1InitialValues: {
        workloadType: '',
      },
      step1Settings: [],
      appName: '',
      envName: '',
      isCreate: '',
    };
  }

  componentDidMount() {
    this.getInitalData();
  }

  getInitalData = async () => {
    let appName = '';
    let envName = '';
    let isCreate = '';
    if (this.props.location.state) {
      appName = _.get(this.props, 'location.state.appName', '');
      envName = _.get(this.props, 'location.state.envName', '');
      isCreate = _.get(this.props, 'location.state.isCreate', false);
      sessionStorage.setItem('appName', appName);
      sessionStorage.setItem('envName', envName);
      sessionStorage.setItem('isCreate', isCreate);
    } else {
      appName = sessionStorage.getItem('appName');
      envName = sessionStorage.getItem('envName');
      isCreate = sessionStorage.getItem('isCreate');
    }
    this.setState({
      appName,
      envName,
      isCreate,
    });
    const res = await this.props.dispatch({
      type: 'workload/getWorkload',
    });
    const traits = await this.props.dispatch({
      type: 'trait/getTraits',
    });
    this.setState({
      traitList: traits,
    });
    if (Array.isArray(res) && res.length) {
      this.setState(
        () => ({
          workloadList: res,
        }),
        () => {
          if (this.state.current === 0) {
            let WorkloadType = '';
            if (this.props.location.state) {
              WorkloadType = _.get(this.props, 'location.state.WorkloadType', '');
              sessionStorage.setItem('WorkloadType', WorkloadType);
            } else {
              WorkloadType = sessionStorage.getItem('WorkloadType');
            }
            this.formRefStep1.current.setFieldsValue({
              workloadType: WorkloadType || this.state.workloadList[0].name,
            });
            this.workloadTypeChange(WorkloadType || this.state.workloadList[0].name);
          }
        },
      );
    }
  };

  onFinishStep1 = (values) => {
    this.setState({
      current: 1,
      step1InitialValues: values,
      isShowMore: false,
    });
  };

  onFinishStep2 = async () => {
    const asyncValidateArray = [];
    this.state.traitNum.forEach((item) => {
      asyncValidateArray.push(item.refname.validateFields());
    });
    await Promise.all(asyncValidateArray);
    const newTraitNum = this.state.traitNum.map((item) => {
      // eslint-disable-next-line no-param-reassign
      item.initialData = item.refname.getSelectValue();
      return item;
    });
    // 进行trait数据整理，便于第三步展示
    this.setState(() => ({
      traitNum: newTraitNum,
      current: 2,
    }));
  };

  gotoStep2 = () => {
    this.setState({
      current: 1,
      isShowMore: false,
    });
  };

  gotoStep1 = () => {
    this.setState({
      current: 0,
    });
  };

  changeShowMore = () => {
    this.setState({
      isShowMore: true,
    });
  };

  addMore = (e) => {
    e.preventDefault();
    this.setState((prev) => ({
      traitNum: prev.traitNum.concat([
        {
          refname: null,
          initialData: {},
          uniq: new Date().valueOf(),
        },
      ]),
    }));
  };

  createApp = async () => {
    const { traitNum, isCreate } = this.state;
    const { step1SubmitObj } = this.state;
    if (isCreate === true || isCreate === 'true') {
      step1SubmitObj.envName = this.props.currentEnv;
    } else {
      step1SubmitObj.envName = this.state.envName;
    }
    step1SubmitObj.appName = this.state.appName;
    const submitObj = _.cloneDeep(step1SubmitObj);
    const { workloadName, appName } = step1SubmitObj;
    submitObj.flags.push({
      name: 'name',
      value: workloadName.toString(),
    });
    // 处理数据为提交的格式
    if (traitNum.length) {
      const { envName } = step1SubmitObj;
      const step2SubmitObj = [];
      traitNum.forEach(({ initialData }) => {
        if (initialData.name) {
          const initialObj = {
            name: initialData.name,
            envName,
            workloadName,
            appName,
            flags: [],
          };
          Object.keys(initialData).forEach((key) => {
            if (key !== 'name' && initialData[key]) {
              initialObj.flags.push({
                name: key,
                value: initialData[key].toString(),
              });
            }
          });
          step2SubmitObj.push(initialObj);
        }
      });
      submitObj.traits = step2SubmitObj;
    }
    const res = await this.props.dispatch({
      type: 'workload/createWorkload',
      payload: {
        params: submitObj,
      },
    });
    if (res) {
      message.success(res);
      this.props.history.push({
        pathname: `/ApplicationList/${appName}/Components`,
        state: { appName, envName: step1SubmitObj.envName },
      });
    }
  };

  createWorkload = async () => {
    await this.formRefStep1.current.validateFields();
    const currentData = this.formRefStep1.current.getFieldsValue();
    const submitObj = {
      envName: this.props.currentEnv,
      workloadType: currentData.workloadType,
      workloadName: currentData.workloadName,
      flags: [],
    };
    Object.keys(currentData).forEach((key) => {
      if (key !== 'workloadName' && key !== 'workloadType' && currentData[key]) {
        submitObj.flags.push({
          name: key,
          value: currentData[key].toString(),
        });
      }
    });
    this.setState({
      current: 1,
      step1InitialValues: currentData,
      step1Settings: submitObj.flags,
      step1SubmitObj: submitObj,
    });
    this.getAcceptTrait(currentData.workloadType);
  };

  workloadTypeChange = (value) => {
    const content = this.formRefStep1.current.getFieldsValue();
    this.formRefStep1.current.resetFields();
    const initialObj = {
      workloadType: content.workloadType,
      workloadName: content.workloadName,
    };
    this.formRefStep1.current.setFieldsValue(initialObj);
    const currentWorkloadSetting = this.state.workloadList.filter((item) => {
      return item.name === value;
    });
    if (currentWorkloadSetting.length) {
      this.setState(
        {
          workloadSettings: currentWorkloadSetting[0].parameters,
        },
        () => {
          this.state.workloadSettings.forEach((item) => {
            if (item.default) {
              initialObj[item.name] = item.default;
            }
          });
          this.formRefStep1.current.setFieldsValue(initialObj);
        },
      );
    }
    this.setState({
      traitNum: [
        {
          refname: null,
          initialData: {},
          uniq: new Date().valueOf(),
        },
      ],
    });
  };

  getAcceptTrait = (workloadType) => {
    const res = this.state.traitList.filter((item) => {
      if (item.appliesTo) {
        if (item.appliesTo === '*') {
          return true;
        }
        if (item.appliesTo.indexOf(workloadType) !== -1) {
          return true;
        }
        return false;
      }
      return false;
    });
    this.setState(() => ({
      availableTraitList: res,
    }));
  };

  deleteTraitItem = (uniq) => {
    // 删除的时候不要依据数组的index删除,要一个唯一性的值
    this.state.traitNum = this.state.traitNum.filter((item) => {
      return item.uniq !== uniq;
    });
    this.setState((prev) => ({
      traitNum: prev.traitNum,
    }));
  };

  render() {
    const { appName, envName } = this.state;
    const { current, step1InitialValues, traitNum, workloadSettings, isCreate } = this.state;
    let { workloadList } = this.state;
    workloadList = Array.isArray(workloadList) ? workloadList : [];
    let currentDetail;
    if (current === 0) {
      currentDetail = (
        <div>
          <div className="minBox">
            <Form
              initialValues={step1InitialValues}
              labelAlign="left"
              {...layout}
              ref={this.formRefStep1}
              name="control-ref"
              onFinish={this.onFinishStep1}
              style={{ width: '60%' }}
            >
              <div style={{ padding: '16px 48px 0px 16px' }}>
                <Form.Item
                  name="workloadName"
                  label="Name"
                  rules={[
                    {
                      pattern: /^[a-z0-9-_]+$/,
                      message:
                        'Names can only use digits(0-9),lowercase letters(a-z),and dashes(-),Underline.',
                    },
                    {
                      required: true,
                      message: 'Please input name!',
                    },
                  ]}
                >
                  <Input />
                </Form.Item>
                <Form.Item
                  name="workloadType"
                  label="Workload Type"
                  rules={[
                    {
                      required: true,
                      message: 'Please select Workload Type!',
                    },
                  ]}
                >
                  <Select
                    placeholder="Select a Workload Type"
                    allowClear
                    onChange={this.workloadTypeChange}
                  >
                    {workloadList.length ? (
                      workloadList.map((item) => {
                        return (
                          <Option value={item.name} key={item.name}>
                            {item.name}
                          </Option>
                        );
                      })
                    ) : (
                      <></>
                    )}
                  </Select>
                </Form.Item>
              </div>
              <Form.Item
                label="Settings"
                style={{
                  background: 'rgba(0, 0, 0, 0.04)',
                  paddingLeft: '16px',
                  marginLeft: '-10px',
                }}
              />
              <div className="relativeBox">
                <p className="hasMore">?</p>
                {Array.isArray(workloadSettings) && workloadSettings.length ? (
                  workloadSettings.map((item) => {
                    if (item.name === 'name') {
                      return <Fragment key={item.name} />;
                    }
                    return item.type === 4 ? (
                      <Form.Item
                        name={item.name}
                        label={item.name}
                        key={item.name}
                        rules={[
                          {
                            required: item.required,
                            message: `Please input ${item.name}!`,
                          },
                          { pattern: /^[0-9]*$/, message: `${item.name} only use digits(0-9).` },
                        ]}
                      >
                        <Input />
                      </Form.Item>
                    ) : (
                      <Form.Item
                        name={item.name}
                        label={item.name}
                        key={item.name}
                        rules={[
                          {
                            required: item.required,
                            message: `Please input ${item.name}!`,
                          },
                          { pattern: /^[^\s]*$/, message: 'Spaces are not allowed!' },
                        ]}
                      >
                        <Input />
                      </Form.Item>
                    );
                  })
                ) : (
                  <></>
                )}
              </div>
              <div className="buttonBox">
                <Button type="primary" className="floatRightGap" onClick={this.createWorkload}>
                  Next
                </Button>
                {isCreate === true || isCreate === 'true' ? (
                  <Link
                    to={{
                      pathname: `/ApplicationList`,
                    }}
                  >
                    <Button className="floatRightGap">Cancle</Button>
                  </Link>
                ) : (
                  <Link
                    to={{
                      pathname: `/ApplicationList/${appName}/Components`,
                      state: { appName, envName },
                    }}
                  >
                    <Button className="floatRightGap">Cancle</Button>
                  </Link>
                )}
              </div>
            </Form>
          </div>
        </div>
      );
    } else if (current === 1) {
      currentDetail = (
        <div>
          <div className="minBox" style={{ width: '60%' }}>
            <div style={{ padding: '0px 48px 0px 16px', width: '60%' }}>
              <p style={{ fontSize: '18px', lineHeight: '32px' }}>
                Name:<span>{step1InitialValues.workloadName}</span>
              </p>
            </div>
            <div style={{ border: '1px solid #eee', padding: '16px 48px 16px 16px' }}>
              <p className="title">{step1InitialValues.workloadType}</p>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span>apps/v1</span>
                <span
                  style={{
                    color: '#1890ff',
                    cursor: 'pointer',
                    display: this.state.isShowMore ? 'none' : 'black',
                  }}
                  onClick={this.changeShowMore}
                >
                  more...
                </span>
              </div>
              {this.state.isShowMore ? (
                <div>
                  <p className="title" style={{ marginTop: '16px' }}>
                    Settings:
                  </p>
                  <Row>
                    {this.state.step1Settings.map((item) => {
                      return (
                        <Fragment key={item.name}>
                          <Col span="8">
                            <p>{item.name}:</p>
                          </Col>
                          <Col span="16">
                            <p>{item.value}</p>
                          </Col>
                        </Fragment>
                      );
                    })}
                  </Row>
                </div>
              ) : (
                ''
              )}
            </div>
            <div ref={this.formRefStep2All}>
              {traitNum.map((item) => {
                return (
                  <CreateTraitItem
                    onRef={(ref) => {
                      // eslint-disable-next-line no-param-reassign
                      item.refname = ref;
                    }}
                    key={item.uniq.toString()}
                    availableTraitList={this.state.availableTraitList}
                    uniq={item.uniq}
                    initialValues={item.initialData}
                    deleteTraitItem={this.deleteTraitItem}
                  />
                );
              })}
            </div>
            <button style={{ marginTop: '16px' }} onClick={this.addMore} type="button">
              Add More...
            </button>
            <div className="buttonBox">
              <Button type="primary" className="floatRight" onClick={this.onFinishStep2}>
                Next
              </Button>
              <Button className="floatRightGap" onClick={this.gotoStep1}>
                Back
              </Button>
            </div>
          </div>
        </div>
      );
    } else {
      currentDetail = (
        <div>
          <div className="minBox">
            <p>
              Name:<span>{step1InitialValues.workloadName}</span>
            </p>
            <Row>
              <Col span="11">
                <div className="summaryBox1">
                  <Row>
                    <Col span="22">
                      <p className="title">{step1InitialValues.workloadType}</p>
                      <p>apps/v1</p>
                    </Col>
                  </Row>
                  <p className="title hasMargin">Settings:</p>
                  <Row>
                    {this.state.step1Settings.map((item) => {
                      return (
                        <Fragment key={item.name}>
                          <Col span="8">
                            <p>{item.name}:</p>
                          </Col>
                          <Col span="16">
                            <p>{item.value}</p>
                          </Col>
                        </Fragment>
                      );
                    })}
                  </Row>
                </div>
              </Col>
              <Col span="1" />
              <Col span="10">
                {traitNum.map(({ initialData }, index) => {
                  if (initialData.name) {
                    return (
                      <div className="summaryBox" key={index.toString()}>
                        <Row>
                          <Col span="22">
                            <p className="title">{initialData.name}</p>
                            <p>core.oam.dev/v1alpha2</p>
                          </Col>
                        </Row>
                        <p className="title hasMargin">Properties:</p>
                        <Row>
                          {Object.keys(initialData).map((currentKey) => {
                            if (currentKey !== 'name') {
                              return (
                                <Fragment key={currentKey}>
                                  <Col span="8">
                                    <p>{currentKey}:</p>
                                  </Col>
                                  <Col span="16">
                                    <p>{initialData[currentKey]}</p>
                                  </Col>
                                </Fragment>
                              );
                            }
                            return <Fragment key={currentKey} />;
                          })}
                        </Row>
                      </div>
                    );
                  }
                  return <Fragment key={index.toString()} />;
                })}
              </Col>
            </Row>
          </div>
          <div className="buttonBox">
            <Button
              type="primary"
              className="floatRight"
              onClick={() => {
                this.createApp();
              }}
            >
              Confirm
            </Button>
            <Button className="floatRightGap" onClick={this.gotoStep2}>
              Back
            </Button>
          </div>
        </div>
      );
    }
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
              {isCreate === true || isCreate === 'true' ? (
                <span>{appName}</span>
              ) : (
                <Link
                  to={{
                    pathname: `/ApplicationList/${appName}/Components`,
                    state: { appName, envName },
                  }}
                >
                  {appName}
                </Link>
              )}
            </Breadcrumb.Item>
            <Breadcrumb.Item>createComponent</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <div className="create-container create-app">
            <Steps current={current}>
              <Step title="Step 1" description="Choose Workload" />
              <Step title="Step 2" description="Attach Trait" />
              <Step title="Step 3" description="Review and confirm" />
            </Steps>
            {currentDetail}
          </div>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
