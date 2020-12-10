import React, { useEffect, useState } from 'react';

import { Form, Input, message, Modal } from 'antd';
import { useModel } from 'umi';

import { EditorState, Environment } from '../../types';

export default ({ mode, environment }: EditorState) => {
  const { createEnvironment, updateEnvironment } = useModel('useEnvironmentModel');
  const [localVisible, setLocalVisible] = useState(false);
  const [posting, setPosting] = useState(false);
  const [form] = Form.useForm();

  useEffect(() => {
    setLocalVisible(environment != null);
    setPosting(false);
    form.resetFields();
    form.setFieldsValue(environment ?? ({ envName: '', namespace: '' } as Environment));
  }, [environment]);

  const isCreate = mode === 'create';
  const title = isCreate ? 'Create' : 'Update';

  return (
    <Modal
      forceRender
      title={`${title} Environment`}
      visible={localVisible}
      onCancel={() => setLocalVisible(false)}
      onOk={form.submit}
      okText={title}
      maskClosable
      okButtonProps={{ disabled: posting }}
    >
      <Form
        form={form}
        labelCol={{ span: 6 }}
        onFinish={async (values: Environment) => {
          setPosting(true);
          try {
            switch (mode) {
              case 'create':
                await createEnvironment({ ...values, email: '', domain: '' });
                break;
              case 'update':
                await updateEnvironment(values.envName, { namespace: values.namespace });
                break;
              default:
                throw new Error(`Invalid mode: ${mode}`);
            }
            message.success({ content: `${title} ${values.envName} environment ok!`, key: 'save' });
            setLocalVisible(false);
          } catch (e) {
            setPosting(false);
          }
        }}
      >
        <Form.Item label="Environment" name="envName" rules={[{ required: true, min: 1, max: 32 }]}>
          <Input disabled={!isCreate} />
        </Form.Item>
        <Form.Item label="Namespace" name="namespace" rules={[{ required: true, min: 1, max: 32 }]}>
          <Input />
        </Form.Item>
      </Form>
    </Modal>
  );
};
