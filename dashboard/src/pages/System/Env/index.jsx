import React, {useEffect, useState} from 'react';
import {PageContainer} from '@ant-design/pro-layout';
import {Button, Table, Space, Modal, Form, Input} from 'antd';
import {ExclamationCircleOutlined} from '@ant-design/icons';
import './index.less';
import {connect} from "dva";

const {confirm} = Modal;

const layout = {
  labelCol: {
    span: 6,
  },
  wrapperCol: {
    span: 18,
  },
};

const TableList = props => {
  const {dispatch, getEnvs} = props;
  const [form] = Form.useForm();
  const [visible, setVisible] = useState(
    false
  );
  const [envs, setEnvs] = useState(
    []
  );
  const [env, setEnv] = useState(
    []
  );

  const showModal = () => {
    setVisible(true)
  };

  const handleOk = async (values) => {
    setVisible(false)
    // fieldsValue为通过验证的数据，现在可进行提交
    const fieldsValue = await form.validateFields();
    const envs = await dispatch({
      type: 'envs/initialEnvs',
      payload: {
        params: fieldsValue
      },
    });
    setEnvs(envs)
    form.resetFields();
  };

  const handleCancel = () => {
    setEnv(null);
    setVisible(false);
    form.resetFields();
  };

  const showDeleteConfirm = (record) => {
    // const self = this;
    confirm({
      title: `Are you sure delete env ${record.name}?`,
      icon: <ExclamationCircleOutlined/>,
      width: 500,
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      onOk() {
        deleteEnv(record)
      }
    });
  }

  const deleteEnv = async (record) => {
    const envs = await dispatch({
      type: 'envs/deleteEnv',
      payload: {
        envName: record.name
      },
    });
    setEnvs(envs)
  }

  const specifyNamespace = (record) => {
    form.setFieldsValue({
      name: record.name,
      namespace: record.namespace
    })
    setEnv(record);
    setVisible(true)
  }

  const getInitalData = async () => {
    const envs = await dispatch({
      type: 'envs/getEnvs',
    });
    setEnvs(envs)
  }
  useEffect(() => {
    getInitalData();
  }, [])


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
  return (
    <PageContainer>
      <div style={{marginBottom: '16px'}}>
        <Space>
          <Button type="primary" onClick={showModal}>
            Create
          </Button>
        </Space>
      </div>
      <Modal
        getContainer={false}
        title={env && env.name ? "Update Env" : "Create Env"}
        visible={visible}
        onOk={handleOk}
        onCancel={handleCancel}
        footer={[
          <Button key="submit" type="primary" onClick={handleOk}>
            {env && env.name ? "Update" : "Create"}
          </Button>,
        ]}
      >
        <Form {...layout}
              form={form}
              name="control-ref" labelAlign="left">
          <Form.Item
            name="name"
            label="Env"
            rules={[
              {
                required: true, message: 'Please input Evn!'
              },
              {pattern: new RegExp('^[0-9a-zA-Z_]{1,}$', 'g'), message: '只允许包含数字、字母、下划线'}
            ]}
          >
            <Input disabled={!!(env && env.name)}/>
          </Form.Item>
          <Form.Item
            name="namespace"
            label="Namespace"
            rules={[
              {
                required: true, message: 'Please specify a Namespace!'
              },
              {pattern: new RegExp('^[0-9a-zA-Z_]{1,}$', 'g'), message: '只允许包含数字、字母、下划线'}
            ]}
          >
            <Input/>
          </Form.Item>
        </Form>
      </Modal>
      <Table
        rowKey={(record) => record.name}
        columns={columns} dataSource={envs}/>
    </PageContainer>
  );
}
export default connect((env) => {
  return {
    getEnvs: env.envs
  }
})(TableList)
