// prevent Ant design style from being overridden
import 'antd/dist/antd.css';

import React, { useState } from 'react';

import { Button, Col, Divider, Dropdown, Input, Menu, Row } from 'antd';
import FormRender from 'form-render/lib/antd';

import { getCapabilityOpenAPISchema } from '@/services/capability';
import { useModel } from '@@/plugin-model/useModel';
import { DownOutlined, UserOutlined } from '@ant-design/icons';
import { PageContainer } from '@ant-design/pro-layout';

export default (): React.ReactNode => {
  // @ts-ignore
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { workloadsLoading, workloadList } = useModel('useWorkloadsModel');
  // @ts-ignore
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  const { traitsLoading, traitsList } = useModel('useTraitsModel');

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
    } else {
      alert(JSON.stringify(formData, null, 2));
    }
  };

  return (
    <PageContainer>
      <Row>
        <Col span="4">Application</Col>
        <Col span="20" />
      </Row>

      <Row>
        <Col span="4">Name:</Col>
        <Col span="8">
          <Input placeholder="Basic usage" />
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
          <Input placeholder="Basic usage" />
        </Col>
        <Col span="12" />
      </Row>

      <Row>
        <Col span="4">Type:</Col>
        <Col span="20">
          <Dropdown overlay={workloadsMenu}>
            <a className="ant-dropdown-link">
              Select <DownOutlined />
            </a>
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
    </PageContainer>
  );
};
