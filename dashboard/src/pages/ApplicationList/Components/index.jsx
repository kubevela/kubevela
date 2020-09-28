import React from 'react';
import { Link } from 'umi';
import { Breadcrumb, Button, Menu, Spin, Popconfirm, message } from 'antd';
import { connect } from 'dva';
import _ from 'lodash';
import './index.less';
import ComponentDetail from '../ComponentDetail/index.jsx';

@connect(({ loading, globalData }) => ({
  loadingAll: loading.models.applist,
  currentEnv: globalData.currentEnv,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      envName: '',
      componentName: '',
      defaultSelectedKeys: '',
      appName: '',
      compList: [],
    };
  }

  componentDidMount() {
    this.getInitData();
  }

  getInitData = async () => {
    let appName = '';
    let description = '';
    let envName = this.props.currentEnv;
    if (this.props.location.state) {
      appName = _.get(this.props, 'location.state.appName', '');
      description = _.get(this.props, 'location.state.description', '');
      envName = _.get(this.props, 'location.state.envName', this.props.currentEnv);
      sessionStorage.setItem('appName', appName);
      sessionStorage.setItem('description', description);
      sessionStorage.setItem('envName', envName);
    } else {
      appName = sessionStorage.getItem('appName');
      description = sessionStorage.getItem('description');
      envName = sessionStorage.getItem('envName');
    }
    this.setState({
      appName,
      envName,
    });
    const res = await this.props.dispatch({
      type: 'applist/getAppDetail',
      payload: {
        envName,
        appName,
      },
    });
    if (res) {
      const compData = _.get(res, 'components', []);
      const compList = [];
      compData.forEach((item) => {
        compList.push({
          compName: item.name,
        });
      });
      this.setState({
        compList,
      });
      if (compList.length) {
        this.changeComponent({
          key: compList[0].compName,
        });
      }
    }
  };

  changeComponent = ({ key }) => {
    this.setState({
      componentName: key,
      defaultSelectedKeys: key,
    });
  };

  deleteApp = async (e) => {
    e.stopPropagation();
    const { envName } = this.state;
    const { appName } = this.state;
    if (appName && envName) {
      const res = await this.props.dispatch({
        type: 'applist/deleteApp',
        payload: {
          appName,
          envName,
        },
      });
      if (res) {
        message.success(res);
        this.props.history.push({ pathname: '/ApplicationList' });
      }
    }
  };

  cancel = (e) => {
    e.stopPropagation();
  };

  render() {
    const { envName, componentName, defaultSelectedKeys, appName, compList } = this.state;
    const { loadingAll } = this.props;
    return (
      <div style={{ height: '100%' }}>
        <div className="breadCrumb">
          <Breadcrumb>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Home</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>
              <Link to="/ApplicationList">Applications</Link>
            </Breadcrumb.Item>
            <Breadcrumb.Item>{appName}</Breadcrumb.Item>
            <Breadcrumb.Item>Components</Breadcrumb.Item>
            <Breadcrumb.Item>{componentName}</Breadcrumb.Item>
          </Breadcrumb>
        </div>
        <Spin spinning={loadingAll}>
          <div className="appComponent">
            <div className="left">
              <Menu
                mode="inline"
                onClick={this.changeComponent}
                defaultSelectedKeys={[defaultSelectedKeys]}
                selectedKeys={defaultSelectedKeys}
              >
                <Menu.ItemGroup key="g1" title="Components">
                  {compList.map((item) => {
                    return <Menu.Item key={item.compName}>{item.compName}</Menu.Item>;
                  })}
                </Menu.ItemGroup>
              </Menu>
              <div className="addComp">
                <Link
                  to={{
                    pathname: `/ApplicationList/${appName}/createComponent`,
                    state: { appName, envName, isCreate: false },
                  }}
                >
                  add a new comp
                </Link>
              </div>
            </div>
            {defaultSelectedKeys ? (
              <div className="right">
                <div className="btn">
                  <Popconfirm
                    title="Are you sure delete this app?"
                    placement="bottom"
                    onConfirm={(e) => this.deleteApp(e)}
                    onCancel={this.cancel}
                    okText="Yes"
                    cancelText="No"
                  >
                    <Button type="primary">Delete App</Button>
                  </Popconfirm>
                </div>
                <ComponentDetail
                  appName={appName}
                  compName={componentName}
                  envName={envName}
                  getInitCompList={this.getInitData}
                />
              </div>
            ) : (
              <div style={{ width: '100%', height: '100%', background: '#FFFFFF' }} />
            )}
          </div>
        </Spin>
      </div>
    );
  }
}

export default TableList;
