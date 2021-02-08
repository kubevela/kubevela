// prevent Ant design style from being overridden
import 'antd/dist/antd.css';

import React, { useState } from 'react';

import { Alert, Button, Form, Input, Space } from 'antd';
import FormRender from 'form-render/lib/antd';
import { history } from 'umi';

import { CloseOutlined, SaveOutlined } from '@ant-design/icons';
import { FooterToolbar, PageContainer } from '@ant-design/pro-layout';

import CapabilityFormItem, { CapabilityFormItemData } from './components/CapabilityFormItem';
import FormGroup from './components/FormGroup';
import TraitsFrom from './components/TraitsFrom';

interface App {
  name: string;
  workload: { name: string; capabilityType: string; data: object };
  traits: { [type: string]: object };
}

export default (): React.ReactNode => {
  const [data, setData] = useState<{
    workloadData?: CapabilityFormItemData;
    traitsData: { [key: string]: object };
  }>({ traitsData: {} });

  const [errorFields, setErrorFields] = useState<{ [key: string]: any }>({});
  const [showError, setShowError] = useState<boolean>(false);

  const saveApp = (app: App) => {
    /* todo: request api server */
    console.info(app);
  };

  return (
    <PageContainer>
      {showError ? (
        <Alert
          showIcon
          type="error"
          message="The following fields failed validation:"
          description={
            <ul>
              {Object.keys(errorFields).map((f) =>
                errorFields[f] == null
                  ? null
                  : Object.entries(errorFields[f]).map((ff) => (
                      <li key={ff[0]}>
                        - {f}.{ff[0]}
                      </li>
                    )),
              )}
            </ul>
          }
        />
      ) : null}
      <Form
        labelCol={{ span: 4 }}
        onFinish={(values) => {
          Object.keys(errorFields).forEach((f) => {
            if (errorFields[f] == null) {
              delete errorFields[f];
            }
          });
          if (Object.keys(errorFields).length > 0) {
            setShowError(true);
            return;
          }

          const {
            name,
            service: { name: serviceName },
          } = values;

          // build app data by form
          const app = {
            name,
            workload: {
              name: serviceName,
              ...data.workloadData,
            } as any,
            traits: { ...data.traitsData },
          };
          if (app.workload.capabilityType == null) {
            errorFields.workload = { serviceType: '' };
            setShowError(true);
            return;
          }
          setShowError(false);

          saveApp(app);
        }}
      >
        <Space direction="vertical" style={{ width: '100%' }}>
          <FormGroup title="Basic">
            <Form.Item
              name="name"
              label="Application Name"
              required
              rules={[{ required: true, max: 200 }]}
            >
              <Input placeholder="Application name" />
            </Form.Item>
          </FormGroup>
          <FormGroup title="Service">
            <Form.Item
              name={['service', 'name']}
              label="Service Name"
              required
              rules={[{ required: true, max: 200 }]}
            >
              <Input placeholder="Service name" />
            </Form.Item>
            <Form.Item label="Service Type" required>
              <CapabilityFormItem
                capability="workloads"
                onChange={(wd) => setData({ ...data, workloadData: wd })}
                onValidate={(errors) => {
                  setErrorFields({
                    ...errorFields,
                    workload: Object.keys(errors).length > 0 ? { ...errors } : undefined,
                  });
                }}
              />
            </Form.Item>
          </FormGroup>
          <FormGroup title="Operations">
            <Form.Item label="Traits">
              <TraitsFrom
                onChange={(td) => setData({ ...data, traitsData: td })}
                onValidate={(errors) => {
                  setErrorFields({
                    ...errorFields,
                    traits: Object.keys(errors).length > 0 ? { ...errors } : undefined,
                  });
                }}
              />
            </Form.Item>
          </FormGroup>
        </Space>
        <FooterToolbar>
          <Button icon={<CloseOutlined />} onClick={() => history.push('/applications')}>
            Cancel
          </Button>
          <Button type="primary" icon={<SaveOutlined />} htmlType="submit">
            Save
          </Button>
        </FooterToolbar>
      </Form>
    </PageContainer>
  );
};
