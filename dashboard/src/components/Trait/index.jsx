import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Row, Col, Modal, Select, message, Breadcrumb, Form, Input } from 'antd';
import './index.less';
import { connect } from 'dva';
import { Link } from 'umi';
import _ from 'lodash';

const { Option } = Select;

const layout = {
  labelCol: {
    span: 8,
  },
  wrapperCol: {
    span: 16,
  },
};

@connect(({ loading, applist, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
  returnObj: applist.returnObj,
}))
class Trait extends React.Component {
  formRefStep2 = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
      selectValue: null,
      compList: [],
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  shouldComponentUpdate(nextProps) {
    if (nextProps.currentEnv === this.props.currentEnv) {
      return true;
    }
    this.props.dispatch({
      type: 'applist/getList',
      payload: {
        url: `/api/envs/${nextProps.currentEnv}/apps/`,
      },
    });
    return true;
  }

  getInitialData = async () => {
    if (this.props.currentEnv) {
      await this.props.dispatch({
        type: 'applist/getList',
        payload: {
          url: `/api/envs/${this.props.currentEnv}/apps/`,
        },
      });
    }
  };

  showModal = () => {
    this.setState(
      {
        visible: true,
      },
      () => {
        if (this.formRefStep2.current) {
          this.formRefStep2.current.resetFields();
        }
      },
    );
  };

  handleOk = async () => {
    await this.formRefStep2.current.validateFields();
    const { title } = this.props.propsObj;
    if (title) {
      const submitObj = {
        name: title,
        flags: [],
      };
      const submitData = this.formRefStep2.current.getFieldValue();
      Object.keys(submitData).forEach((currentKey) => {
        if (
          currentKey !== 'name' &&
          currentKey !== 'appName' &&
          currentKey !== 'compName' &&
          submitData[currentKey]
        ) {
          submitObj.flags.push({
            name: currentKey,
            value: submitData[currentKey].toString(),
          });
        }
      });
      const { currentEnv: envName } = this.props;
      const { appName, compName } = submitData;
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
          const { history } = this.props.propsObj;
          history.push({
            pathname: `/ApplicationList/${appName}/Components`,
            state: { appName, envName },
          });
        }
      }
    }
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  onChange = async (value) => {
    this.setState({
      selectValue: value,
      compList: [],
    });
    const res = await this.props.dispatch({
      type: 'applist/getAppDetail',
      payload: {
        envName: this.props.currentEnv,
        appName: value,
      },
    });
    if (res) {
      const compData = _.get(res, 'components', []);
      const compList = [];
      compData.forEach((item) => {
        compList.push({
          compName: item.name,
        });
      });
      this.setState({
        compList,
      });
    }
  };

  onSearch = () => {};

  render() {
    const { btnValue, title, settings = [], btnIsShow, crdInfo, appliesTo } = this.props.propsObj;
    const initialObj = {};
    if (settings.length) {
      settings.forEach((item) => {
        if (item.default) {
          initialObj[item.name] = item.default;
        }
      });
    }
    let appList = _.get(this.props, 'returnObj', []);
    if (!appList) {
      appList = [];
    }
    const { compList = [] } = this.state;
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Traits</Breadcrumb.Item>
            <Breadcrumb.Item>{title}</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Row>
            <Col span="11">
              <div className="deployment">
                <Row>
                  <Col span="22">
                    <p className="title">{title}</p>
                    {crdInfo ? (
                      <p>
                        {crdInfo.apiVersion}
                        <span>,kind=</span>
                        {crdInfo.kind}
                      </p>
                    ) : (
                      <p />
                    )}
                  </Col>
                </Row>
                <Row>
                  <Col span="22">
                    <p className="title">Applies To:</p>
                    <p>{Array.isArray(appliesTo) ? appliesTo.join(', ') : appliesTo}</p>
                  </Col>
                </Row>
                <p className="title">Configurable Properties:</p>
                {settings.map((item, index) => {
                  return (
                    <Row key={index.toString()}>
                      <Col span="8">
                        <p>{item.name}</p>
                      </Col>
                      <Col span="16">
                        <p>{item.default || item.usage}</p>
                      </Col>
                    </Row>
                  );
                })}
              </div>
              <Button
                type="primary"
                className="create-button"
                onClick={this.showModal}
                style={{ display: btnIsShow ? 'block' : 'none' }}
              >
                {btnValue}
              </Button>
              <Modal
                title="Attach Trait"
                visible={this.state.visible}
                onOk={this.handleOk}
                onCancel={this.handleCancel}
                footer={[
                  <Button key="back" onClick={this.handleCancel}>
                    Cancel
                  </Button>,
                  <Button key="submit" type="primary" onClick={this.handleOk}>
                    Submit
                  </Button>,
                ]}
              >
                <Form
                  labelAlign="left"
                  {...layout}
                  ref={this.formRefStep2}
                  name="control-ref"
                  className="traitItem"
                  initialValues={initialObj}
                >
                  <Form.Item
                    label="App"
                    name="appName"
                    rules={[{ required: true, message: 'Please Select a Application!' }]}
                  >
                    <Select
                      showSearch
                      value={this.state.selectValue}
                      style={{ width: '100%' }}
                      placeholder="Select a Application"
                      optionFilterProp="children"
                      onChange={this.onChange}
                      onSearch={this.onSearch}
                      filterOption={(input, option) =>
                        option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
                      }
                    >
                      {appList.length ? (
                        appList.map((item, index) => {
                          return (
                            <Option key={index.toString()} value={item.app}>
                              {item.app}
                            </Option>
                          );
                        })
                      ) : (
                        <Fragment />
                      )}
                    </Select>
                  </Form.Item>
                  <Form.Item
                    label="Component"
                    name="compName"
                    rules={[{ required: true, message: 'Please Select a Component!' }]}
                  >
                    <Select
                      allowClear
                      // value={this.state.selectValue}
                      style={{ width: '100%' }}
                      placeholder="Select a Component"
                    >
                      {compList.length ? (
                        compList.map((item) => {
                          return (
                            <Option key={item.compName} value={item.compName}>
                              {item.compName}
                            </Option>
                          );
                        })
                      ) : (
                        <Fragment />
                      )}
                    </Select>
                  </Form.Item>
                  <div className="relativeBox">
                    <Form.Item label="Properties" />
                    {settings ? (
                      settings.map((item) => {
                        return item.type === 4 ? (
                          <Form.Item
                            name={item.name}
                            label={item.name}
                            key={item.name}
                            rules={[
                              {
                                required: item.required || false,
                                message: `Please input ${item.name} !`,
                              },
                              {
                                pattern: /^[0-9]*$/,
                                message: `${item.name} only use digits(0-9).`,
                              },
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
                                required: item.required || false,
                                message: `Please input ${item.name} !`,
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
                </Form>
              </Modal>
            </Col>
          </Row>
        </PageContainer>
      </div>
    );
  }
}

export default Trait;
