import { PageContainer } from '@ant-design/pro-layout';
import { Button, Row, Col } from 'antd';
import { Link } from 'umi';
import './index.less';

import React from 'react';

export default class Workload extends React.PureComponent {
  render() {
    const {
      btnValue,
      pathname,
      title,
      crdInfo,
      state,
      settings,
      hrefAddress,
      btnIsShow,
    } = this.props.propsObj;
    return (
      <PageContainer>
        <Row>
          <Col span="11">
            <div className="deployment">
              <a href={hrefAddress}>?</a>
              <Row>
                <Col span="22">
                  <p className="title">{title}</p>
                  <p>
                    {crdInfo.apiVersion},kind={crdInfo.kind}
                  </p>
                </Col>
              </Row>
              <p className="title">Configurable Settings:</p>
              {settings.map((item, index) => {
                return (
                  <Row key={index.toString()}>
                    <Col span="8">
                      <p>{item.name}</p>
                    </Col>
                    <Col span="16">
                      {
                        // eslint-disable-next-line consistent-return
                      }
                      <p>{item.default || item.usage}</p>
                    </Col>
                  </Row>
                );
              })}
            </div>
            <Link to={{ pathname, state }} style={{ display: btnIsShow ? 'block' : 'none' }}>
              <Button type="primary" className="create-button">
                {btnValue}
              </Button>
            </Link>
          </Col>
        </Row>
      </PageContainer>
    );
  }
}
