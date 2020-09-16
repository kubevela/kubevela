import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Table, Space, Modal, Form, Input, message, Spin, Breadcrumb } from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { Link } from 'umi';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

const { Column } = Table;

const layout = {
  labelCol: {
    span: 4,
  },
  wrapperCol: {
    span: 20,
  },
};

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.capability,
  loadingList: loading.effects['capability/getCapabilityCenterlist'],
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.PureComponent {
  formRef = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
      capabilityList: [],
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  getInitialData = async () => {
    const res = await this.props.dispatch({
      type: 'capability/getCapabilityCenterlist',
    });
    if (res) {
      let newRes = _.cloneDeep(res);
      newRes = newRes.map((item) => {
        // eslint-disable-next-line no-param-reassign
        item.btnSyncLoading = false;
        return item;
      });
      this.setState({
        capabilityList: newRes,
      });
    }
  };

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
    const submitData = await this.formRef.current.validateFields();
    const res = await this.props.dispatch({
      type: 'capability/createCapabilityCenter',
      payload: {
        params: submitData,
      },
    });
    if (res) {
      message.success(res);
      this.setState({
        visible: false,
      });
      this.getInitialData();
    } else {
      // 目前创建分为两部，创建列表和安装相关依赖，如果成功一个，目前返回500，而此时可能列表已经创建成功，只是依赖安装失败
      this.setState({
        visible: false,
      });
      this.getInitialData();
    }
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  syncSignle = async (text, index) => {
    if (text) {
      const newList = _.cloneDeep(this.state.capabilityList);
      newList[index].btnSyncLoading = true;
      this.setState(() => ({
        capabilityList: newList,
      }));
      const res = await this.props.dispatch({
        type: 'capability/syncCapability',
        payload: {
          capabilityCenterName: text,
        },
      });
      if (res) {
        message.success(res);
      }
      const newList1 = _.cloneDeep(this.state.capabilityList);
      newList1[index].btnSyncLoading = false;
      this.setState(() => ({
        capabilityList: newList1,
      }));
      window.location.reload();
    }
  };

  copyURL = (text) => {
    const oInput = document.createElement('input');
    oInput.value = text;
    document.body.appendChild(oInput);
    oInput.select();
    document.execCommand('Copy');
    oInput.className = 'oInput';
    oInput.style.display = 'none';
    message.success('copy success');
  };

  showDeleteConfirm = () => {
    message.info('正在开发中...');
  };

  render() {
    let { capabilityList } = this.state;
    let { loadingList } = this.props;
    loadingList = loadingList || false;
    capabilityList = Array.isArray(capabilityList) ? capabilityList : [];
    return (
      <div>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>Capability</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <PageContainer>
          <Spin spinning={loadingList}>
            <div style={{ marginBottom: '16px' }}>
              <Space>
                <Button type="primary" onClick={this.showModal}>
                  Create
                </Button>
              </Space>
            </div>
            <Modal
              title="Create Capability Center"
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
                  name="Name"
                  label="Name"
                  rules={[
                    {
                      required: true,
                      message: 'Please input name!',
                    },
                    {
                      pattern: new RegExp('^[0-9a-zA-Z_]{1,60}$', 'g'),
                      message:
                        'The maximum length is 60,should be combination of numbers,alphabets,underline!',
                    },
                  ]}
                >
                  <Input />
                </Form.Item>
                <Form.Item
                  name="Address"
                  label="URL"
                  rules={[
                    { pattern: /^[^\s]*$/, message: 'Spaces are not allowed!' },
                    {
                      required: true,
                      message: 'Please input URL!',
                    },
                  ]}
                >
                  <Input />
                </Form.Item>
              </Form>
            </Modal>
            <Table dataSource={capabilityList} pagination={false} rowKey={(record) => record.name}>
              <Column
                title="Name"
                dataIndex="name"
                key="name"
                render={(text, record) => {
                  return (
                    <Link to={{ pathname: '/Capability/Detail', state: { name: record.name } }}>
                      {text}
                    </Link>
                  );
                }}
              />
              <Column
                title="URL"
                dataIndex="url"
                key="url"
                width="60%"
                render={(text) => {
                  return (
                    <div className="hoverItem">
                      <a href={text} target="_blank" rel="noreferrer">
                        {text}
                      </a>
                      <div className="copyIcon" onClick={() => this.copyURL(text)}>
                        <CopyOutlined />
                      </div>
                    </div>
                  );
                }}
              />
              <Column
                title="Operations"
                dataIndex="name"
                key="name"
                render={(text, record, index) => {
                  return (
                    <Space>
                      <Button
                        loading={record.btnSyncLoading}
                        onClick={() => this.syncSignle(text, index)}
                        type="primary"
                        ghost
                      >
                        sync
                      </Button>
                      <Button onClick={() => this.showDeleteConfirm(text)}>remove</Button>
                    </Space>
                  );
                }}
              />
            </Table>
          </Spin>
        </PageContainer>
      </div>
    );
  }
}

export default TableList;
