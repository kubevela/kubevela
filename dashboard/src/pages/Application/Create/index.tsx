// prevent Ant design style from being overridden
import 'antd/dist/antd.css';

import React, { useState } from 'react';

import { Button, Col, Divider, Dropdown, Input, Menu, Row } from 'antd';
import FormRender from 'form-render/lib/antd';

import { createApplication } from '@/services/application';
import { getCapabilityOpenAPISchema } from '@/services/capability';
import { useModel } from '@@/plugin-model/useModel';
import { DownOutlined, UserOutlined } from '@ant-design/icons';

export default (): React.ReactNode => {
  const { currentEnvironment } = useModel('useEnvironmentModel');
  const { workloadList } = useModel('useWorkloadsModel');
  const { traitsList } = useModel('useTraitsModel');

  const [workloadType, setWorkloadType] = useState('');

  const workloadMenuList = workloadList?.map((i) => (
    <Menu.Item
      key={i.name}
      icon={<UserOutlined />}
      onClick={() => handleMenuClick('workload_type', i.name)}
    >
      {i.name}
    </Menu.Item>
  ));

  const traitMenuList = traitsList?.map((i) => (
    <Menu.Item
      key={i.name}
      icon={<UserOutlined />}
      onClick={() => handleMenuClick('trait', i.name)}
    >
      {i.name}
    </Menu.Item>
  ));

  const workloadsMenu = <Menu>{workloadMenuList}</Menu>;

  const traitsMenu = <Menu>{traitMenuList}</Menu>;

  const [applicationName, setApplicationName] = useState('');
  const [serviceName, setServiceName] = useState('');

  // Capability parameters form render
  const [formData, setData] = useState({});
  // schema is OpenAPI Schema JSON data
  const [workloadSchema, setWorkloadSchema] = useState({});
  const [traitSchema, setTraitSchema] = useState({});
  const [valid, setValid] = useState([]);

  function handleMenuClick(capabilityType: string, capabilityName: string) {
    console.log('click', capabilityName);
    getCapabilityOpenAPISchema(capabilityName).then((result) => {
      const data = JSON.parse(result.data);
      if (capabilityType === 'workload_type') {
        setWorkloadType(capabilityName);
        setWorkloadSchema(data);
      } else if (capabilityType === 'trait') {
        setTraitSchema(data);
      }
    });
  }

  const onSubmit = () => {
    // valid == 0: validation passed
    if (valid.length > 0) {
      alert(`invalid：${valid.toString()}`);
    }
    const servicesDict = {};
    servicesDict[serviceName] = {
      type: workloadType,
    };

    Object.entries(formData).forEach(([key, value]) => {
      // eslint-disable-next-line default-case
      switch (typeof value) {
        case 'number':
          servicesDict[serviceName][key] = value;
          break;
        case 'string':
          if (value === null || value === '') {
            break;
          }
          servicesDict[serviceName][key] = value;
          break;
        case 'object':
          if (Array.isArray(value) && value.length) {
            servicesDict[serviceName][key] = value;
            break;
          }
      }
    });

    const appFile = {
      name: applicationName,
      services: servicesDict,
    };

    if (currentEnvironment?.envName == null) {
      alert('could not get current environment name');
      return;
    }
    createApplication(currentEnvironment.envName, appFile).then((r) => {
      alert(r.data);
    });
  };

  return (
    <div style={{ maxWidth: 600 }}>
      <Row>
        <Col span="4">Application</Col>
        <Col span="20" />
      </Row>

      <Row>
        <Col span="4">Name:</Col>
        <Col span="8">
          <Input
            placeholder="Basic usage"
            onChange={(e) => {
              const v = e.target.value;
              setApplicationName(v);
            }}
          />
        </Col>
        <Col span="12" />
      </Row>

      <Row>
        <Col span="24">
          <Divider />
        </Col>
      </Row>

      <Row>
        <Col span="4">Services</Col>
        <Col span="20" />
      </Row>

      <Row>
        <Col span="4">Name:</Col>
        <Col span="8">
          <Input
            placeholder="Basic usage"
            onChange={(e) => {
              const v = e.target.value;
              setServiceName(v);
            }}
          />
        </Col>
        <Col span="12" />
      </Row>

      <Row>
        <Col span="4">Type:</Col>
        <Col span="20">
          <Dropdown overlay={workloadsMenu}>
            <Button>
              Select <DownOutlined />
            </Button>
          </Dropdown>
        </Col>
      </Row>

      <Row>
        <Col span="4">Settings:</Col>
        <Col span="20">
          <FormRender
            schema={workloadSchema}
            formData={formData}
            onChange={setData}
            onValidate={setValid}
            displayType="column"
          />
        </Col>
      </Row>

      <Row>
        <Col span="24">
          <Divider />
        </Col>
      </Row>

      <Row>
        <Col span="4">Traits</Col>
        <Col span="20" />
      </Row>

      <Row>
        <Col span="4">Type:</Col>
        <Col span="20">
          <Dropdown overlay={traitsMenu}>
            <a className="ant-dropdown-link" onClick={(e) => e.preventDefault()}>
              Select <DownOutlined />
            </a>
          </Dropdown>
        </Col>
      </Row>

      <Row>
        <Col span="4">Properties:</Col>
        <Col span="20">
          <FormRender
            schema={traitSchema}
            formData={formData}
            onChange={setData}
            onValidate={setValid}
            displayType="column"
          />
        </Col>
      </Row>

      <Row>
        <Col span="8" />
        <Col span="8">
          <Button onClick={onSubmit} type="primary">
            Submit
          </Button>
        </Col>
        <Col span="8" />
      </Row>
    </div>
  );
};
