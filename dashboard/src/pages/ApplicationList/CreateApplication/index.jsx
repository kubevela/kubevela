import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import './index.less';
import { Button, Row, Col, Form, Input, Select, Steps } from 'antd';
import { connect } from 'dva';
import { Link } from 'umi';

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

@connect(() => ({}))
class TableList extends React.Component {
  formRefStep1 = React.createRef();

  formRefStep2 = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      current: 0,
      isShowMore: false,
      traitNum: [1],
      step1InitialValues: {
        WorkloadType: 'Deployment',
      },
      step2InitialValues: {
        TraitType0: 'Autoscaling',
      },
    };
  }

  UNSAFE_componentWillMount() {
    if (this.props.location.state) {
      const WorkloadType = this.props.location.state.WorkloadType || 'Deployment';
      const TraitType = this.props.location.state.TraitType || 'Autoscaling';
      const activeStep = this.props.location.state.activeStep || 0;
      const tempState1 = { ...this.state.step1InitialValues, WorkloadType };
      const tempState2 = { ...this.state.step2InitialValues, TraitType0: TraitType };
      this.setState(() => ({
        current: activeStep,
        step1InitialValues: tempState1,
        step2InitialValues: tempState2,
      }));
    }
  }

  onFinishStep1 = (values) => {
    this.setState({
      current: 1,
      step1InitialValues: values,
      isShowMore: false,
    });
  };

  onFinishStep2 = (values) => {
    this.setState({
      current: 2,
      step2InitialValues: values,
    });
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
      traitNum: prev.traitNum.concat([1]),
    }));
  };

  createApp = async () => {
    await this.props.dispatch({
      type: 'applist/createApp', // applist对应models层的命名空间namespace
      payload: {
        a: 1,
        b: 3,
      },
    });
  };

  render() {
    const { current, step1InitialValues, step2InitialValues, traitNum } = this.state;
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
                  name="WorkloadName"
                  label="Name"
                  rules={[
                    {
                      required: true,
                      message: 'Please input name!',
                    },
                  ]}
                >
                  <Input />
                </Form.Item>
                <Form.Item
                  name="WorkloadType"
                  label="Workload Type"
                  rules={[
                    {
                      required: true,
                      message: 'Please select Workload Type!',
                    },
                  ]}
                >
                  <Select placeholder="Select a Workload Type" allowClear>
                    <Option value="Deployment">Deployment</Option>
                    <Option value="Task">Task</Option>
                  </Select>
                </Form.Item>
                <Form.Item label="Settings" />
              </div>
              <div className="relativeBox">
                <p className="hasMore">?</p>
                <Form.Item name="setting1" label="Deployment Strategy">
                  <Input />
                </Form.Item>
                <Form.Item name="setting2" label="Rolling Update Strategy">
                  <Input />
                </Form.Item>
                <Form.Item name="setting3" label="Min Ready Seconds">
                  <Input />
                </Form.Item>
                <Form.Item name="setting4" label="Revision History Limit">
                  <Input />
                </Form.Item>
                <Form.Item name="setting5" label="Replicas">
                  <Input />
                </Form.Item>
              </div>
              <div className="buttonBox">
                <Button type="primary" className="floatRight" htmlType="submit">
                  Next
                </Button>
                <Link to="/ApplicationList">
                  <Button className="floatRightGap">Cancle</Button>
                </Link>
              </div>
            </Form>
          </div>
        </div>
      );
    } else if (current === 1) {
      currentDetail = (
        <div>
          <div className="minBox">
            <div style={{ padding: '0px 48px 0px 16px', width: '60%' }}>
              <p style={{ fontSize: '18px', lineHeight: '32px' }}>
                Name:<span>{step1InitialValues.WorkloadName}</span>
              </p>
            </div>
            <Form
              initialValues={step2InitialValues}
              labelAlign="left"
              {...layout}
              ref={this.formRefStep2}
              name="control-ref"
              onFinish={this.onFinishStep2}
              style={{ width: '60%' }}
            >
              <div style={{ border: '1px solid #eee', padding: '16px 48px 16px 16px' }}>
                <p className="title">{step1InitialValues.WorkloadType}</p>
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
                      <Col span="8">
                        <p>Deployment Strategy</p>
                        <p>Rolling Update Strategy</p>
                        <p>Min Ready Seconds</p>
                        <p>Revision History Limit</p>
                        <p>Replicas</p>
                      </Col>
                      <Col span="16">
                        <p>{step1InitialValues.setting1}</p>
                        <p>{step1InitialValues.setting2}</p>
                        <p>{step1InitialValues.setting3}</p>
                        <p>{step1InitialValues.setting4}</p>
                        <p>{step1InitialValues.setting5}</p>
                      </Col>
                    </Row>
                  </div>
                ) : (
                  ''
                )}
              </div>
              {traitNum.map((item, index) => {
                return (
                  <div
                    style={{ border: '1px solid #eee', margin: '16px 0px 8px' }}
                    key={index.toString()}
                  >
                    <div style={{ padding: '16px 48px 0px 16px' }}>
                      <Form.Item name={`TraitType${index}`} label="Trait">
                        <Select placeholder="Select a Trait" allowClear>
                          <Option value="Autoscaling">Autoscaling</Option>
                          <Option value="Rollout">Rollout</Option>
                        </Select>
                      </Form.Item>
                      <Form.Item label="Properties" />
                    </div>
                    <div className="relativeBox">
                      <p className="hasMore">?</p>
                      <Form.Item name={`setting1${index}`} label="Min Instances">
                        <Input />
                      </Form.Item>
                      <Form.Item name={`setting2${index}`} label="Max Instances">
                        <Input />
                      </Form.Item>
                    </div>
                  </div>
                );
              })}
              <button style={{ marginTop: '16px' }} onClick={this.addMore} type="button">
                Add More...
              </button>
              <div className="buttonBox">
                <Button type="primary" className="floatRight" htmlType="submit">
                  Next
                </Button>
                <Button className="floatRightGap" onClick={this.gotoStep1}>
                  Back
                </Button>
              </div>
            </Form>
          </div>
        </div>
      );
    } else {
      currentDetail = (
        <div>
          <div className="minBox">
            <p>
              Name:<span>{step1InitialValues.WorkloadName}</span>
            </p>
            <Row>
              <Col span="11">
                <div className="summaryBox1">
                  <Row>
                    <Col span="22">
                      <p className="title">{step1InitialValues.WorkloadType}</p>
                      <p>apps/v1</p>
                    </Col>
                  </Row>
                  <p className="title hasMargin">Settings:</p>
                  <Row>
                    <Col span="8">
                      <p>Deployment Strategy</p>
                      <p>Rolling Update Strategy</p>
                      <p>Min Ready Seconds</p>
                      <p>Revision History Limit</p>
                      <p>Replicas</p>
                    </Col>
                    <Col span="16">
                      <p>{step1InitialValues.setting1}</p>
                      <p>{step1InitialValues.setting2}</p>
                      <p>{step1InitialValues.setting3}</p>
                      <p>{step1InitialValues.setting4}</p>
                      <p>{step1InitialValues.setting5}</p>
                    </Col>
                  </Row>
                </div>
              </Col>
              <Col span="1" />
              <Col span="10">
                {traitNum.map((item, index) => {
                  return (
                    <div className="summaryBox" key={index.toString()}>
                      <Row>
                        <Col span="22">
                          <p className="title">{step2InitialValues[`TraitType${index}`]}</p>
                          <p>core.oam.dev/v1alpha2</p>
                        </Col>
                      </Row>
                      <p className="title hasMargin">Properties:</p>
                      <Row>
                        <Col span="8">
                          <p>Min Instances</p>
                          <p>Max Instances</p>
                        </Col>
                        <Col span="16">
                          <p>{step2InitialValues[`setting1${index}`]}</p>
                          <p>{step2InitialValues[`setting2${index}`]}</p>
                        </Col>
                      </Row>
                    </div>
                  );
                })}
              </Col>
            </Row>
          </div>
          <div className="buttonBox">
            {/* <Link to="/ApplicationList">
              <Button type="primary" className="floatRight">
                Confirm
              </Button>
            </Link> */}
            <Button
              type="primary"
              className="floatRight"
              onClick={(event) => {
                this.createApp(event);
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
    );
  }
}

export default TableList;
