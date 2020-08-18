import React from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Table, Space, Modal, Form, Input } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import { Link } from 'umi';
import './index.less';

const { confirm } = Modal;

// const syncSignle = (text,record) =>{
//   console.log(text,record)
// };
const syncSignle = () => {};

function showDeleteConfirm(record) {
  confirm({
    title: `Are you sure delete ${record.name}?`,
    icon: <ExclamationCircleOutlined />,
    width: 500,
    content: (
      <div>
        <p>您本次移除 capability center，将会删除的应用列表：</p>
        <Space>
          <span>abc</span>
          <span>abc</span>
          <span>abc</span>
          <span>abc</span>
        </Space>
        <p>确认后，移除该 capability center，并且删除相应的应用？</p>
      </div>
    ),
    okText: 'Yes',
    okType: 'danger',
    cancelText: 'No',
    onOk() {
      // console.log('OK');
    },
    onCancel() {
      // console.log('Cancel');
    },
  });
}

const layout = {
  labelCol: {
    span: 4,
  },
  wrapperCol: {
    span: 20,
  },
};

const columns = [
  {
    title: 'Name',
    dataIndex: 'name',
    key: 'name',
    render: (text, record) => {
      return (
        <Link to={{ pathname: '/Capability/Detail', query: { id: record.index } }}>{text}</Link>
      );
    },
  },
  {
    title: 'URL',
    dataIndex: 'URL',
    key: 'URL',
    render: (text) => {
      return (
        <a href={text} target="_blank" rel="noreferrer">
          {text}
        </a>
      );
    },
  },
  {
    title: 'Status',
    dataIndex: 'Status',
    key: 'Status',
  },
  {
    title: 'Operations',
    dataIndex: 'Operations',
    key: 'Operations',
    render: (text, record) => {
      return (
        <Space>
          <Button onClick={() => syncSignle(text, record)}>sync</Button>
          <Button onClick={() => showDeleteConfirm(record)}>remove</Button>
        </Space>
      );
    },
  },
];
const data = [
  {
    key: '1',
    name: 'OAM-extended',
    URL: 'https://github.com/oam-dev/catalog/repository',
    Status: '?',
    Operations: 0,
  },
];

class TableList extends React.PureComponent {
  formRef = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
    };
  }

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async () => {
    await this.formRef.current.validateFields();
    this.setState({
      visible: false,
    });
    // const fieldsValue = await this.formRef.current.validateFields();
    // fieldsValue为通过验证的数据，现在可进行提交
    // console.log(fieldsValue)
  };

  handleTest = async () => {
    await this.formRef.current.validateFields();
    this.setState({
      visible: false,
    });
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  render() {
    return (
      <PageContainer>
        <div style={{ marginBottom: '16px' }}>
          <Space>
            <Button type="primary" onClick={this.showModal}>
              Create
            </Button>
            <Button type="default">Sync All</Button>
          </Space>
        </div>
        <Modal
          title="Create Capability"
          visible={this.state.visible}
          onOk={this.handleOk}
          onCancel={this.handleCancel}
          footer={[
            <Button key="test" onClick={this.handleTest}>
              Test
            </Button>,
            <Button key="submit" type="primary" onClick={this.handleOk}>
              Create
            </Button>,
          ]}
        >
          <Form {...layout} ref={this.formRef} name="control-ref" labelAlign="left">
            <Form.Item
              name="name"
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
              name="url"
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
        <Table columns={columns} dataSource={data} />
      </PageContainer>
    );
  }
}

export default TableList;
