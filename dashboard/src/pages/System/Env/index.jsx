import React from 'react';
import {PageContainer} from '@ant-design/pro-layout';
import {Button, Table, Space, Modal, Form, Input} from 'antd';
import {ExclamationCircleOutlined} from '@ant-design/icons';
import {Link} from 'umi';
import './index.less';
import {connect} from "dva";

const {confirm} = Modal;

function showDeleteConfirm(record) {
  confirm({
    title: `Are you sure delete env ${record.name}?`,
    icon: <ExclamationCircleOutlined/>,
    width: 500,
    okText: 'Yes',
    okType: 'danger',
    cancelText: 'No',
    onOk() {
      console.log('OK');
    }
  });
}

function specifyNamespace(record) {

}

const layout = {
  labelCol: {
    span: 6,
  },
  wrapperCol: {
    span: 18,
  },
};

const columns = [
  {
    title: 'Env',
    dataIndex: 'name',
    key: 'name'
  },
  {
    title: 'Namespace',
    dataIndex: 'namespace',
    key: 'namespace',
  },
  {
    title: 'Operations',
    dataIndex: 'Operations',
    key: 'Operations',
    render: (text, record) => {
      return (
        <Space>
          <Button onClick={() => specifyNamespace(record)}>specify namespace</Button>
          <Button onClick={() => showDeleteConfirm(record)}>remove</Button>
        </Space>
      );
    },
  },
];

@connect(() => ({}))
class TableList extends React.PureComponent {
  formRef = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      visible: false,
      envs: []
    };
  }

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = async (values) => {
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

  getInitalData = async () => {
    const envs = await this.props.dispatch({
      type: 'envs/getEnvs',
    });
    this.setState({
      envs
    });
  }

  componentDidMount() {
    this.getInitalData();
  }

  render() {
    return (
      <PageContainer>
        <div style={{marginBottom: '16px'}}>
          <Space>
            <Button type="primary" onClick={this.showModal}>
              Create
            </Button>
          </Space>
        </div>
        <Modal
          title="Create Env"
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
              label="Env"
              rules={[
                {
                  required: true, message: 'Please input Evn!'
                },
              ]}
            >
              <Input/>
            </Form.Item>
            <Form.Item
              name="namespace"
              label="Namespace"
              rules={[
                {
                  required: true, message: 'Please specify a Namespace!'
                },
              ]}
            >
              <Input/>
            </Form.Item>
          </Form>
        </Modal>
        <Table
          rowKey={(record) => record.name}
          columns={columns} dataSource={this.state.envs}/>
      </PageContainer>
    );
  }
}

export default TableList;
