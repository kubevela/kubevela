import React from 'react';
import { Link } from 'umi';
import { Breadcrumb, Button, Menu, Spin } from 'antd';
import { connect } from 'dva';
import _ from 'lodash';
import './index.less';
import ComponentDetail from '../ComponentDetail/index.jsx';

@connect(({ loading }) => ({
  loadingAll: loading.models.applist,
}))
class TableList extends React.PureComponent {
  constructor(props) {
    super(props);
    this.state = {
      envName: '',
      componentName: '',
      defaultSelectedKeys: 'a1',
    };
  }

  UNSAFE_componentWillMount() {
    let appName = '';
    let envName = '';
    if (this.props.location.state) {
      appName = _.get(this.props, 'location.state.appName', '');
      envName = _.get(this.props, 'location.state.envName', '');
      sessionStorage.setItem('appName', appName);
      sessionStorage.setItem('envName', envName);
    } else {
      appName = sessionStorage.getItem('appName');
      envName = sessionStorage.getItem('envName');
    }
    this.setState({
      envName,
      componentName: appName,
    });
    if (appName === 'test33') {
      this.setState({
        defaultSelectedKeys: 'a1',
      });
    } else if (appName === 'test01') {
      this.setState({
        defaultSelectedKeys: 'c2',
      });
    } else {
      this.setState({
        defaultSelectedKeys: 'c3',
      });
    }
  }

  changeComponent = ({ key }) => {
    if (key === 'a1') {
      this.setState({
        componentName: 'test33',
      });
    } else if (key === 'c2') {
      this.setState({
        componentName: 'test01',
      });
    } else {
      this.setState({
        componentName: 'testoo',
      });
    }
  };

  render() {
    const { envName, componentName, defaultSelectedKeys } = this.state;
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
            <Breadcrumb.Item>appname</Breadcrumb.Item>
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
              >
                <Menu.ItemGroup key="g1" title="Components">
                  <Menu.Item key="a1">a1(containerized)</Menu.Item>
                  <Menu.Item key="c2">c2(deploy)</Menu.Item>
                  <Menu.Item key="c3">c3(webserver)</Menu.Item>
                </Menu.ItemGroup>
              </Menu>
              <div className="addComp">
                <Link
                  to={{
                    // pathname: '/ApplicationList/CreateApplication',
                    pathname: `/ApplicationList/${componentName}/createComponent`,
                    state: { appName: componentName, envName },
                  }}
                >
                  add a new comp
                </Link>
              </div>
            </div>
            <div className="right">
              <div style={{ margin: '10px', float: 'right' }}>
                <Button type="primary">Delete App</Button>
              </div>
              <ComponentDetail appName={componentName} envName={envName} />
            </div>
          </div>
        </Spin>
      </div>
    );
  }
}

export default TableList;
