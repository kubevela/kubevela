import { PageContainer } from "@ant-design/pro-layout";
import { Button, Row, Col } from "antd";
import { Link } from "umi";
import './index.less'

import React from "react";

export default class Workload extends React.Component {
  constructor(props){
    super(props);
  }
  render() {
    const { btnValue, pathname, title, state, settings, hrefAddress, btnIsShow } = this.props.propsObj
    return (
      <PageContainer>
        <Row>
          <Col span="11">
            <div className="deployment">
              <a href={hrefAddress}>?</a>
              <Row>
                <Col span="22">
                  <p className="title">{title}</p>
                  <p>apps/v1</p>
                </Col>
              </Row>
              <p className="title">Configurable Settings:</p>
              {
                settings.map((item,index)=>{
                  return (
                    <Row key={index}>
                      <Col span="8">
                        <p>{item.name}</p>
                      </Col>
                      <Col span="16">
                        <p>{item.value}</p>
                      </Col>
                    </Row>
                  )
                })
              }
            </div>
            <Link to={{pathname:pathname,state:state}} style={{ display: btnIsShow?'block':'none' }}>
              <Button type="primary" className="create-button">{btnValue}</Button>
            </Link>
          </Col>
        </Row>
      </PageContainer>
    );
  }
}