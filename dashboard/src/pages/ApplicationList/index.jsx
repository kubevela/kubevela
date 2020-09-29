import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Form, Spin, Breadcrumb, Modal, Input, Table, Space, message } from 'antd';
import { connect } from 'dva';
import moment from 'moment';
import './index.less';
import { Link } from 'umi';

const layout = {
  labelCol: {
    span: 6,
  },
  wrapperCol: {
    span: 18,
  },
};

@connect(({ loading, applist, globalData }) => ({
  loadingAll: loading.models.applist,
  returnObj: applist.returnObj,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.Component {
  formRef = React.createRef();

  columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => {
        return (
          <Link
            to={{
              pathname: `/ApplicationList/${record.name}/Components`,
              state: { appName: record.name, envName: this.props.currentEnv },
            }}
          >
            {text}
          </Link>
        );
      },
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (text) => {
        return text;
      },
    },
    {
      title: 'Created Time',
      dataIndex: 'createdTime',
      key: 'createdTime',
      render: (text) => {
        return this.getFormatDate(text);
      },
    },
    {
      title: 'Actions',
      dataIndex: 'Actions',
      key: 'Actions',
      render: (text, record) => {
        return (
          <Space>
            <Button onClick={() => this.goToDetail(record)}>Details</Button>
            <Button onClick={() => this.deleteApp(record)}>Delete</Button>
          </Space>
        );
      },
    },
  ];

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
    };
  }

  componentDidMount() {
    this.getInitData();
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

  getInitData = () => {
    const { currentEnv } = this.props;
    if (currentEnv) {
      this.props.dispatch({
        type: 'applist/getList',
        payload: {
          url: `/api/envs/${currentEnv}/apps/`,
        },
      });
    }
  };

  goToDetail = (record) => {
    this.props.history.push({
      pathname: `/ApplicationList/${record.name}/Components`,
      state: { appName: record.name, envName: this.props.currentEnv },
    });
  };

  deleteApp = async (record) => {
    const appName = record.name;
    const envName = this.props.currentEnv;
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
        this.getInitData();
      }
    }
  };

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
    const submitData = await this.formRef.current.validateFields();
    this.props.history.push({
      pathname: `/ApplicationList/${submitData.appName}/createComponent`,
      state: { ...submitData, isCreate: true },
    });
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  onFinish = () => {};

  handleChange = () => {};

  handleAdd = () => {};

  onSelect = () => {};

  getHeight = (num) => {
    return `${num * 43}px`;
  };

  getFormatDate = (time) => {
    return moment(new Date(time)).utc().utcOffset(-6).format('YYYY-MM-DD HH:mm:ss');
  };

  render() {
    let { loadingAll, returnObj } = this.props;
    returnObj = returnObj || [];
    loadingAll = loadingAll || false;
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Applications</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Spin spinning={loadingAll}>
            <div className="applist">
              <Form name="horizontal_login" layout="inline" onFinish={this.onFinish}>
                <Form.Item>
                  <Button onClick={this.showModal} type="primary" style={{ marginBottom: 16 }}>
                    create
                  </Button>
                </Form.Item>
              </Form>
            </div>
            <Table
              rowKey={(record) => record.name + Math.random(1, 100)}
              columns={this.columns}
              dataSource={returnObj}
            />
          </Spin>
          <Modal
            title="Create Application"
            visible={this.state.visible}
            onOk={this.handleOk}
            onCancel={this.handleCancel}
            footer={[
              <Button key="submit" type="primary" onClick={this.handleOk}>
                Create
              </Button>,
            ]}
          >
            <Form {...layout} ref={this.formRef} name="control-ref" labelAlign="left">
              <Form.Item
                name="appName"
                label="Name"
                rules={[
                  {
                    required: true,
                    message: 'Please input application name!',
                  },
                  {
                    pattern: /^[a-z0-9-_]+$/,
                    message:
                      'Name can only use digits(0-9),lowercase letters(a-z),and dashes(-),Underline.',
                  },
                ]}
              >
                <Input />
              </Form.Item>
              <Form.Item name="description" label="Description">
                <Input.TextArea />
              </Form.Item>
            </Form>
          </Modal>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
