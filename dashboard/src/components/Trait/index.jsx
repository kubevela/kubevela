import React from "react";
import { PageContainer } from "@ant-design/pro-layout";
import { Button, Row, Col, Modal, Select, message } from "antd";
import './index.less'
const { Option } = Select;

class Trait extends React.Component {
  constructor(props){
    super(props);
    this.state = {
      visible: false,
      selectValue: null
    }
  }
  showModal = () => {
    this.setState({
      visible: true,
    });
  };
  handleOk = e => {
    const { selectValue } = this.state;
    if(selectValue){
      this.setState({
        visible: false,
      });
      const { pathname, state, history } = this.props.propsObj;
      history.push({pathname,state})
    }else{
      message.warn('please select a application')
    }
  };
  handleCancel = e => {
    console.log(e);
    this.setState({
      visible: false,
    });
  };
  onChange = (value)=> {
    this.setState({
      selectValue: value
    })
  };
  onSearch = (val)=> {
    console.log('search:', val);
  };
  render() {
    const { btnValue, title, settings, btnIsShow } = this.props.propsObj
    return (
      <PageContainer>
        <Row>
          <Col span="11">
            <div className="deployment">
              <Row>
                <Col span="22">
                  <p className="title">{title}</p>
                  <p>core.oam.dev/v1alpha2</p>
                </Col>
              </Row>
              <Row>
                <Col span="22">
                  <p className="title">Applies To</p>
                  <p>*.apps/v1</p>
                </Col>
              </Row>
              <p className="title">Conflicts With:</p>
              <p className="title">Configurable Properties:</p>
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
            <Button type="primary" className="create-button" onClick={this.showModal} style={{ display: btnIsShow?'block':'none' }}>{btnValue}</Button>
            <Modal
              title="Select a Application"
              visible={this.state.visible}
              onOk={this.handleOk}
              onCancel={this.handleCancel}
              footer={[
                <Button key="back" onClick={this.handleCancel}>
                  Cancel
                </Button>,
                <Button key="submit" type="primary" onClick={this.handleOk}>
                  Next
                </Button>
              ]}
            >
              <Select
                showSearch
                allowClear
                value={this.state.selectValue}
                style={{ width: '100%' }}
                placeholder="Select a Application"
                optionFilterProp="children"
                onChange={this.onChange}
                onSearch={this.onSearch}
                filterOption={(input, option) =>
                  option.children.toLowerCase().indexOf(input.toLowerCase()) >= 0
                }
              >
                <Option value="1">Application1</Option>
                <Option value="2">Application2</Option>
                <Option value="3">Application3</Option>
              </Select>
            </Modal>
          </Col>
        </Row>
      </PageContainer>
    );
  }
}

export default Trait;