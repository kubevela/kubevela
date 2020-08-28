import React, { useEffect, useState } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Table, Space, Modal, Form, Input, Tooltip } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import './index.less';
import { connect } from 'dva';
import * as _ from 'lodash';

const { confirm } = Modal;

const layout = {
  labelCol: {
    span: 6,
  },
  wrapperCol: {
    span: 18,
  },
};

const TableList = (props) => {
  const { dispatch } = props;
  const tableEnvs = _.get(props, 'getEnvs.envs', []);
  const [form] = Form.useForm();
  const [visible, setVisible] = useState(false);
  const [env, setEnv] = useState([]);

  const showModal = () => {
    setVisible(true);
  };

  const handleOk = async () => {
    const fieldsValue = await form.validateFields();
    await dispatch({
      type: 'envs/initialEnvs',
      payload: {
        params: fieldsValue,
      },
    });
    setEnv(null);
    form.resetFields();
    setVisible(false);
  };

  const handleCancel = () => {
    setEnv(null);
    setVisible(false);
    form.resetFields();
  };

  const deleteEnv = async (record) => {
    await dispatch({
      type: 'envs/deleteEnv',
      payload: {
        envName: record.name,
      },
    });
  };

  const showDeleteConfirm = (record) => {
    confirm({
      title: `Are you sure delete env ${record.name}?`,
      icon: <ExclamationCircleOutlined />,
      width: 500,
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      onOk() {
        deleteEnv(record);
      },
    });
  };

  const specifyNamespace = (record) => {
    form.setFieldsValue({
      name: record.name,
      namespace: record.namespace,
    });
    setEnv(record);
    setVisible(true);
  };

  const getInitialData = async () => {
    await dispatch({
      type: 'envs/getEnvs',
    });
  };
  useEffect(() => {
    getInitialData();
  }, []);

  const columns = [
    {
      title: 'Env',
      dataIndex: 'name',
      key: 'name',
      render: (text) => {
        if (text.length > 20) {
          return <Tooltip title={text}>{text.substr(0, 20)}...</Tooltip>;
        }
        return text;
      },
    },
    {
      title: 'Namespace',
      dataIndex: 'namespace',
      key: 'namespace',
      render: (text) => {
        if (text.length > 20) {
          return <Tooltip title={text}>{text.substr(0, 20)}...</Tooltip>;
        }
        return text;
      },
    },
    {
      title: 'Current',
      dataIndex: 'current',
      align: 'center',
      key: 'current',
      render: (text) => {
        return text === '*' ? 'active' : '';
      },
    },
    {
      title: 'Operations',
      dataIndex: 'Operations',
      key: 'Operations',
      render: (text, record) => {
        return (
          <Space>
            <Button onClick={() => specifyNamespace(record)}>update</Button>
            <Button disabled={record.current} onClick={() => showDeleteConfirm(record)}>
              remove
            </Button>
          </Space>
        );
      },
    },
  ];
  return (
    <PageContainer>
      <div style={{ marginBottom: '16px' }}>
        <Space>
          <Button type="primary" onClick={showModal}>
            Create
          </Button>
        </Space>
      </div>
      <Modal
        getContainer={false}
        title={env && env.name ? 'Update Env' : 'Create Env'}
        visible={visible}
        onOk={handleOk}
        onCancel={handleCancel}
        footer={[
          <Button key="submit" type="primary" onClick={handleOk}>
            {env && env.name ? 'Update' : 'Create'}
          </Button>,
        ]}
      >
        <Form {...layout} form={form} name="control-ref" labelAlign="left">
          <Form.Item
            name="name"
            label="Env"
            rules={[
              {
                required: true,
                message: 'Please input Evn!',
              },
              {
                pattern: new RegExp('^[0-9a-zA-Z_]{1,32}$', 'g'),
                message: 'Should be combination of numbers,alphabets,underline',
              },
            ]}
          >
            <Input disabled={!!(env && env.name)} />
          </Form.Item>
          <Form.Item
            name="namespace"
            label="Namespace"
            rules={[
              {
                required: true,
                message: 'Please specify a Namespace!',
              },
              {
                pattern: new RegExp('^[0-9a-zA-Z_]{1,32}$', 'g'),
                message:
                  'The maximum length is 63, should be combination of numbers,alphabets,underline',
              },
            ]}
          >
            <Input />
          </Form.Item>
        </Form>
      </Modal>
      <Table rowKey={(record) => record.name} columns={columns} dataSource={tableEnvs} />
    </PageContainer>
  );
};
export default connect((env) => {
  return {
    getEnvs: env.envs,
  };
})(TableList);
