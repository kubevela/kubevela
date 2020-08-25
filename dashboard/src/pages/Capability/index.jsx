import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Table, Space, Modal, Form, Input, message } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import { Link } from 'umi';
import './index.less';
import { connect } from 'dva';

const { confirm } = Modal;
const { Column } = Table;

// const syncSignle = async (text,record) =>{
//   // console.log(text,record)
//   const res = await this.props.dispatch({
//     type: 'capability/syncCapability',
//     payload: {
//       capabilityCenterName: record.name
//     }
//   })
//   if(res){
//     message.success(res);
//   }
// };

// function showDeleteConfirm(record) {
//   confirm({
//     title: `Are you sure delete ${record.name}?`,
//     icon: <ExclamationCircleOutlined />,
//     width: 500,
//     content: (
//       <div>
//         <p>您本次移除 { record.name }，将会删除的应用列表：</p>
//         <Space>
//           <span>abc</span>
//           <span>abc</span>
//           <span>abc</span>
//           <span>abc</span>
//         </Space>
//         <p>确认后，移除{ record.name }，并且删除相应的应用？</p>
//       </div>
//     ),
//     okText: 'Yes',
//     okType: 'danger',
//     cancelText: 'No',
//     async onOk() {
//       const res = await this.props.dispatch({
//         type: 'capability/deleteOneCapability',
//         payload: {
//           capabilityName: record.name
//         }
//       })
//       if(res){
//         message.success(res);
//         this.getInitialData()
//       }
//     },
//     onCancel() {
//       // console.log('Cancel');
//     },
//   });
// }

const layout = {
  labelCol: {
    span: 4,
  },
  wrapperCol: {
    span: 20,
  },
};

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.applist,
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
      this.setState({
        capabilityList: res,
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
      type: 'capability/createCapability',
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

  syncSignle = async (record) => {
    const res = await this.props.dispatch({
      type: 'capability/syncCapability',
      payload: {
        capabilityCenterName: record.name,
      },
    });
    if (res) {
      message.success(res);
      this.getInitialData();
    }
  };

  showDeleteConfirm = (record) => {
    // eslint-disable-next-line
    const _this = this;
    confirm({
      title: `Are you sure delete ${record.name}?`,
      icon: <ExclamationCircleOutlined />,
      width: 500,
      content: (
        <div>
          <p>您本次移除 {record.name}，将会删除的应用列表：</p>
          <Space>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
          </Space>
          <p>确认后，移除{record.name}，并且删除相应的应用？</p>
        </div>
      ),
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      async onOk() {
        const res = await _this.props.dispatch({
          type: 'capability/deleteOneCapability',
          payload: {
            capabilityName: record.name,
          },
        });
        if (res) {
          message.success(res);
          _this.getInitialData();
        }
      },
      onCancel() {
        // console.log('Cancel');
      },
    });
  };

  render() {
    let { capabilityList } = this.state;
    capabilityList = Array.isArray(capabilityList) ? capabilityList : [];
    return (
      <PageContainer>
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
                },
              ]}
            >
              <Input />
            </Form.Item>
            <Form.Item
              name="Address"
              label="URL"
              rules={[
                {
                  required: true,
                },
              ]}
            >
              <Input />
            </Form.Item>
          </Form>
        </Modal>
        <Table dataSource={capabilityList}>
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
            render={(text, record) => {
              return (
                <Space>
                  <Button onClick={() => this.syncSignle(record)}>sync</Button>
                  <Button onClick={() => this.showDeleteConfirm(record)}>remove</Button>
                </Space>
              );
            }}
          />
        </Table>
      </PageContainer>
    );
  }
}

export default TableList;
