// prevent Ant design style from being overridden
import 'antd/dist/antd.css';

import React, { useState } from 'react';

import { Alert, Button, Form, Input, message, Space, Spin } from 'antd';
import { history, useModel } from 'umi';

import { createApplication } from '@/services/application';
import { CloseOutlined, SaveOutlined } from '@ant-design/icons';
import { FooterToolbar, PageContainer } from '@ant-design/pro-layout';

import FormGroup from './components/FormGroup';
import ServiceForm from './components/ServiceForm';

export default (): React.ReactNode => {
  const [services, setServices] = useState<{ [key: string]: any }>({});

  const [errorFields, setErrorFields] = useState<{ [key: string]: any }>({});
  const [showError, setShowError] = useState<boolean>(false);
  const { currentEnvironment } = useModel('useEnvironmentModel');

  const [loading, setLoading] = useState<boolean>(false);

  const saveApp = (app: API.AppFile) => {
    const envName = currentEnvironment?.envName;
    if (envName == null) {
      message.error('Unrecognized environment!');
      return;
    }
    setLoading(true);
    createApplication(envName, app)
      .then(({ data }) => {
        message.success({ content: data, key: 'created' });
        history.push('/applications');
      })
      .catch(() => setLoading(false));
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
                  : Object.entries(errorFields[f]).map((ff) => {
                      return (
                        <li key={ff[0]}>
                          - {f}.{ff[0]}
                        </li>
                      );
                    }),
              )}
            </ul>
          }
        />
      ) : null}
      <Spin spinning={loading}>
        <Form
          labelCol={{ span: 4 }}
          onFinishFailed={({ errorFields: ef }) => {
            setShowError(true);
            const appErrorFields = {};
            ef.forEach((f) => {
              appErrorFields[f.name.join('.')] = f.errors.join(',');
            });
            setErrorFields({ ...errorFields, app: appErrorFields });
          }}
          onFinish={(values) => {
            delete errorFields['app.name'];
            Object.keys(errorFields).forEach((f) => {
              if (errorFields[f] == null) {
                delete errorFields[f];
              }
            });
            if (Object.keys(errorFields).length > 0) {
              setShowError(true);
              return;
            }
            const { name } = values;

            saveApp({
              name,
              services,
            });
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
            <FormGroup title="Services">
              <ServiceForm
                onChange={(value) => {
                  const servicesObj = {};
                  value.forEach((service) => {
                    const { name, type, data, traits } = service;
                    if (name == null || type == null) {
                      return;
                    }
                    const serviceObj: any = { type };
                    servicesObj[name] = serviceObj;

                    if (data != null) {
                      Object.keys(data).forEach((k) => {
                        serviceObj[k] = data[k];
                      });
                    }
                    if (traits != null) {
                      Object.keys(traits).forEach((k) => {
                        serviceObj[k] = traits[k];
                      });
                    }
                  });
                  if (Object.keys(servicesObj).length < value.length) {
                    setErrorFields({ ...errorFields, services: { name: '' } });
                    setShowError(true);
                  } else {
                    if (errorFields.services?.name != null) {
                      delete errorFields.services.name;
                      setErrorFields({ ...errorFields });
                    }
                    setShowError(false);
                  }
                  setServices(servicesObj);
                }}
                onValidate={(errors) => {
                  setErrorFields(errors);
                }}
              />
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
      </Spin>
    </PageContainer>
  );
};
