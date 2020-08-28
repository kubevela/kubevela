import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Table, Space, Modal, Form, Input, message, Spin } from 'antd';
// import { ExclamationCircleOutlined } from '@ant-design/icons';
import { Link } from 'umi';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

// const { confirm } = Modal;
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

  // handleTest = async () => {
  //   await this.formRef.current.validateFields();
  //   this.setState({
  //     visible: false,
  //   });
  // };

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
        // this.getInitialData();
      }
      const newList1 = _.cloneDeep(this.state.capabilityList);
      newList1[index].btnSyncLoading = false;
      this.setState(() => ({
        capabilityList: newList1,
      }));
    }
  };

  showDeleteConfirm = () => {
    message.info('正在开发中...');
    // if (record) {
    //   // eslint-disable-next-line
    //   const _this = this;
    //   confirm({
    //     title: `Are you sure delete ${record}?`,
    //     icon: <ExclamationCircleOutlined />,
    //     width: 500,
    //     content: (
    //       <div>
    //         <p>您本次移除 {record}，将会删除的应用列表：</p>
    //         <Space>
    //           <span>abc</span>
    //           <span>abc</span>
    //           <span>abc</span>
    //           <span>abc</span>
    //         </Space>
    //         <p>确认后，移除{record}，并且删除相应的应用？</p>
    //       </div>
    //     ),
    //     okText: 'Yes',
    //     okType: 'danger',
    //     cancelText: 'No',
    //     async onOk() {
    //       const res = await _this.props.dispatch({
    //         type: 'capability/deleteCapability',
    //         payload: {
    //           capabilityName: record,
    //         },
    //       });
    //       if (res) {
    //         message.success(res);
    //         _this.getInitialData();
    //       }
    //     },
    //     onCancel() {
    //       // console.log('Cancel');
    //     },
    //   });
    // }
  };

  render() {
    let { capabilityList } = this.state;
    let { loadingList } = this.props;
    loadingList = loadingList || false;
    capabilityList = Array.isArray(capabilityList) ? capabilityList : [];
    return (
      <PageContainer>
        <Spin spinning={loadingList}>
          <div style={{ marginBottom: '16px' }}>
            <Space>
              <Button type="primary" onClick={this.showModal}>
                Create
              </Button>
              {/* <Button type="default">Sync All</Button> */}
            </Space>
          </div>
          <Modal
            title="Create Capability Center"
            visible={this.state.visible}
            onOk={this.handleOk}
            onCancel={this.handleCancel}
            footer={[
              // <Button key="test" onClick={this.handleTest}>
              //   Test
              // </Button>,
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
                ]}
              >
                <Input />
              </Form.Item>
              <Form.Item
                name="Address"
                label="URL"
                rules={[
                  // { pattern: '/^((https|http|ftp|rtsp|mms){0,1}(:\/\/){0,1})\.(([A-Za-z0-9-~]+)\.)+([A-Za-z0-9-~\/])+$/',
                  //   message: 'please input correct URL'
                  // },
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
              render={(text) => {
                return (
                  <a href={text} target="_blank" rel="noreferrer">
                    {text}
                  </a>
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
    );
  }
}

export default TableList;
