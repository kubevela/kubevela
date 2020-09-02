import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Button, Row, Col, Modal, Select, message } from 'antd';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

const { Option } = Select;

@connect(({ loading, applist, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
  returnObj: applist.returnObj,
}))
class Trait extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      visible: false,
      selectValue: null,
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  getInitialData = async () => {
    if (this.props.currentEnv) {
      await this.props.dispatch({
        type: 'applist/getList',
        payload: {
          url: `/api/envs/${this.props.currentEnv}/apps/`,
        },
      });
    }
  };

  showModal = () => {
    this.setState({
      visible: true,
    });
  };

  handleOk = () => {
    const { selectValue } = this.state;
    if (selectValue) {
      this.setState({
        visible: false,
      });
      const { history } = this.props.propsObj;
      history.push({
        pathname: '/ApplicationList/ApplicationListDetail',
        state: {
          appName: selectValue,
          envName: this.props.currentEnv,
          traitType: this.props.propsObj.title,
        },
      });
    } else {
      message.warn('please select a application');
    }
  };

  handleCancel = () => {
    this.setState({
      visible: false,
    });
  };

  onChange = (value) => {
    this.setState({
      selectValue: value,
    });
  };

  onSearch = () => {};

  render() {
    const { btnValue, title, settings, btnIsShow, crdInfo, appliesTo } = this.props.propsObj;
    const appList = _.get(this.props, 'returnObj', []);
    return (
      <PageContainer>
        <Row>
          <Col span="11">
            <div className="deployment">
              <Row>
                <Col span="22">
                  <p className="title">{title}</p>
                  {crdInfo ? (
                    <p>
                      {crdInfo.apiVersion}
                      <span>,kind=</span>
                      {crdInfo.kind}
                    </p>
                  ) : (
                    <p />
                  )}
                </Col>
              </Row>
              <Row>
                <Col span="22">
                  <p className="title">Applies To</p>
                  <p>{Array.isArray(appliesTo) ? appliesTo.join(', ') : appliesTo}</p>
                </Col>
              </Row>
              <p className="title">Configurable Properties:</p>
              {settings.map((item, index) => {
                return (
                  <Row key={index.toString()}>
                    <Col span="8">
                      <p>{item.name}</p>
                    </Col>
                    <Col span="16">
                      <p>{item.default || item.usage}</p>
                    </Col>
                  </Row>
                );
              })}
            </div>
            <Button
              type="primary"
              className="create-button"
              onClick={this.showModal}
              style={{ display: btnIsShow ? 'block' : 'none' }}
            >
              {btnValue}
            </Button>
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
                </Button>,
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
                {appList.length ? (
                  appList.map((item) => {
                    return (
                      <Option key={item.name} value={item.name}>
                        {item.name}
                      </Option>
                    );
                  })
                ) : (
                  <Fragment />
                )}
              </Select>
            </Modal>
          </Col>
        </Row>
      </PageContainer>
    );
  }
}

export default Trait;
