import { Menu, Dropdown, Button } from 'antd';
import { DownOutlined } from '@ant-design/icons';
import React from 'react';
import { connect } from 'dva';

@connect(() => ({}))
export default class WorkSpaceDropDown extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      workSpaceName: '',
      envs: [],
    };
  }

  async componentDidMount() {
    const envs = await this.props.dispatch({
      type: 'applist/getEnvs', // applist对应models层的命名空间namespace
    });
    const { name = 'test' } = envs.find((a) => {
      return a.current === '*';
    });
    this.setState({
      envs,
      workSpaceName: name,
    });
    this.props.dispatch({
      type: 'globalData/currentEnv',
      payload: {
        currentEnv: name,
      },
    });
  }

  handleMenuClick = (e) => {
    this.setState(
      {
        workSpaceName: e.key,
      },
      () => {
        this.props.dispatch({
          type: 'globalData/currentEnv',
          payload: {
            currentEnv: e.key,
          },
        });
      },
    );
  };

  render() {
    const menu = (
      <Menu onClick={this.handleMenuClick}>
        {/* <Menu.Item key="default">default</Menu.Item>
        <Menu.Item key="am-system">oam-system</Menu.Item>
        <Menu.Item key="linkerd">linkerd</Menu.Item>
        <Menu.Item key="rio-system">rio-system</Menu.Item> */}
        {this.state.envs.map((item) => {
          return <Menu.Item key={item.name}>{item.name}</Menu.Item>;
        })}
      </Menu>
    );
    return (
      <Dropdown overlay={menu}>
        <Button style={{ marginTop: '10px' }}>
          {this.state.workSpaceName} <DownOutlined />
        </Button>
      </Dropdown>
    );
  }
}
