import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Row, Col, Modal, Select, Breadcrumb, Form } from 'antd';
import { connect } from 'dva';
import { Link } from 'umi';
import _ from 'lodash';
import './index.less';

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
export default class Workload extends React.Component {
  formRefStep2 = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
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
    const submitData = await this.formRefStep2.current.validateFields();
    const { history } = this.props.propsObj;
    history.push({
      pathname: `/ApplicationList/${submitData.appName}/createComponent`,
      state: {
        ...submitData,
        isCreate: false,
        envName: this.props.currentEnv,
        WorkloadType: this.props.propsObj.title,
      },
    });
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  onChange = () => {};

  onSearch = () => {};

  render() {
    const { btnValue, title, crdInfo, settings, btnIsShow } = this.props.propsObj;
    let appList = _.get(this.props, 'returnObj', []);
    if (!appList) {
      appList = [];
    }
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Workloads</Breadcrumb.Item>
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
                <p className="title">Configurable Settings:</p>
                {settings.map((item, index) => {
                  if (item.name === 'name') {
                    return <Fragment key={index.toString()} />;
                  }
                  return (
                    <Row key={index.toString()}>
                      <Col span="8">
                        <p>{item.name}</p>
                      </Col>
                      <Col span="16">
                        {
                          // eslint-disable-next-line consistent-return
                        }
                        <p>{item.default || item.usage}</p>
                      </Col>
                    </Row>
                  );
                })}
              </div>
              {/* <Link to={{ pathname, state }} style={{ display: btnIsShow ? 'block' : 'none' }}>
                <Button type="primary" className="create-button">
                  {btnValue}
                </Button>
              </Link> */}
              <Button
                type="primary"
                className="create-button"
                onClick={() => this.showModal()}
                style={{ display: btnIsShow ? 'block' : 'none' }}
              >
                {btnValue}
              </Button>
            </Col>
          </Row>
          <Modal
            title="Add Component"
            visible={this.state.visible}
            onOk={this.handleOk}
            onCancel={this.handleCancel}
            footer={[
              <Button key="back" onClick={this.handleCancel}>
                Cancel
              </Button>,
              <Button key="submit" type="primary" onClick={this.handleOk}>
                Next
              </Button>,
            ]}
          >
            <Form
              labelAlign="left"
              {...layout}
              ref={this.formRefStep2}
              name="control-ref"
              className="traitItem"
            >
              <Form.Item
                label="App"
                name="appName"
                rules={[{ required: true, message: 'Please Select a Application!' }]}
              >
                <Select
                  showSearch
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
            </Form>
          </Modal>
        </PageContainer>
      </div>
    );
  }
}
