import { Menu, Dropdown, Button } from "antd";
import { DownOutlined } from "@ant-design/icons";
import React from "react";

export default class workSpaceDropDown extends React.Component {
  state = {
    workSpaceName: "default",
  };

  handleMenuClick = (e) => {
    this.setState({
      workSpaceName: e.key,
    });
  };

  render() {
    const menu = (
      <Menu onClick={this.handleMenuClick}>
        <Menu.Item key="default">default</Menu.Item>
        <Menu.Item key="am-system">oam-system</Menu.Item>
        <Menu.Item key="linkerd">linkerd</Menu.Item>
        <Menu.Item key="rio-system">rio-system</Menu.Item>
      </Menu>
    );
    return (
      <Dropdown overlay={menu}>
        <Button style={{ marginTop: "10px" }}>
          {this.state.workSpaceName} <DownOutlined />
        </Button>
      </Dropdown>
    );
  }
}
