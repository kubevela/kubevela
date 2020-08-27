import React, { Fragment } from 'react';
import { PageContainer } from '@ant-design/pro-layout';
import { Space, Modal, Button, Row, Col, message } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import './index.less';
import { connect } from 'dva';
import _ from 'lodash';

const { confirm } = Modal;

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      workloadList: [],
      traitList: [],
    };
  }

  componentDidMount() {
    this.getInitialData();
  }

  getInitialData = async () => {
    const res = await this.props.dispatch({
      type: 'capability/capabilityList',
    });
    if (res) {
      const workloadList = [];
      const traitList = [];
      if (Array.isArray(res)) {
        res.forEach((item) => {
          if (item.type === 'workload') {
            workloadList.push(item);
          } else if (item.type === 'trait') {
            traitList.push(item);
          }
        });
        this.setState({
          workloadList,
          traitList,
        });
      }
    }
  };

  gotoOtherPage = () => {
    // window.location.href = 'https://github.com/oam-dev/catalog/blob/master/workloads/cloneset/README.md';
    window.open('https://github.com/oam-dev/catalog/blob/master/workloads/cloneset/README.md');
  };

  installSignle = async (e, name) => {
    e.stopPropagation();
    const capabilityCenterName = _.get(this.props, 'location.state.name', '');
    const res = await this.props.dispatch({
      type: 'capability/syncOneCapability',
      payload: {
        capabilityCenterName,
        capabilityName: name,
      },
    });
    if (res) {
      message.success(res);
      this.getInitialData();
    }
  };

  syncAllSignle = async () => {
    const capabilityCenterName = _.get(this.props, 'location.state.name', '');
    if (capabilityCenterName) {
      const res = await this.props.dispatch({
        type: 'capability/syncCapability',
        payload: {
          capabilityCenterName,
        },
      });
      if (res) {
        message.success(res);
        this.getInitialData();
      }
    }
  };

  showDeleteConfirm = () => {
    // eslint-disable-next-line
    const _this = this;
    const capabilityCenterName = _.get(this.props, 'location.state.name', '');
    confirm({
      title: `Are you sure delete ${capabilityCenterName}?`,
      icon: <ExclamationCircleOutlined />,
      width: 500,
      content: (
        <div>
          <p style={{ margin: '0px' }}>您本次移除 {capabilityCenterName}，将会删除的应用列表：</p>
          <Space>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
            <span>abc</span>
          </Space>
          <p style={{ margin: '0px' }}>确认后，移除 {capabilityCenterName}，并且删除相应的应用？</p>
        </div>
      ),
      okText: 'Yes',
      okType: 'danger',
      cancelText: 'No',
      async onOk() {
        const res = await _this.props.dispatch({
          type: 'capability/deleteOneCapability',
          payload: {
            capabilityName: capabilityCenterName,
          },
        });
        if (res) {
          message.success(res);
          _this.props.history.push({ pathname: '/Capability' });
        }
      },
      onCancel() {
        // console.log('Cancel');
      },
    });
  };

  render() {
    const { workloadList = [], traitList = [] } = this.state;
    return (
      <PageContainer>
        <div style={{ marginBottom: '16px' }}>
          <Space>
            <Button type="primary" onClick={this.syncAllSignle}>
              Install all
            </Button>
            <Button type="default" onClick={this.showDeleteConfirm}>
              Remove
            </Button>
          </Space>
        </div>
        <div>
          <h3>Workloads</h3>
          <Row>
            {workloadList.length ? (
              workloadList.map((item) => {
                return (
                  <Col span="4" key={item.name}>
                    <div className="itemBox" onClick={this.gotoOtherPage}>
                      <img
                        src="https://ss0.bdstatic.com/70cFvHSh_Q1YnxGkpoWK1HF6hhy/it/u=1109866916,1852667152&fm=26&gp=0.jpg"
                        alt="workload"
                      />
                      <p>{item.name}</p>
                      {item.status === 'installed' ? (
                        <Button />
                      ) : (
                        <Button onClick={(e) => this.installSignle(e, item.name)}>install</Button>
                      )}
                    </div>
                  </Col>
                );
              })
            ) : (
              <Fragment />
            )}
          </Row>
        </div>
        <div>
          <h3>Traits</h3>
          <Row>
            {traitList.length ? (
              traitList.map((item) => {
                return (
                  <Col span="4" key={item.name}>
                    <div className="itemBox" onClick={this.gotoOtherPage}>
                      <img
                        src="https://ss0.bdstatic.com/70cFvHSh_Q1YnxGkpoWK1HF6hhy/it/u=1109866916,1852667152&fm=26&gp=0.jpg"
                        alt="workload"
                      />
                      <p>{item.name}</p>
                      {item.status === 'installed' ? (
                        <Button />
                      ) : (
                        <Button onClick={(e) => this.installSignle(e, item.name)}>install</Button>
                      )}
                    </div>
                  </Col>
                );
              })
            ) : (
              <Fragment />
            )}
          </Row>
        </div>
      </PageContainer>
    );
  }
}

export default TableList;
