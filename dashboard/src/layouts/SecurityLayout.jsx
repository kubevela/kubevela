import React from "react";
import { PageLoading } from "@ant-design/pro-layout";
import { Redirect, connect } from "umi";
import { stringify } from "querystring";

class SecurityLayout extends React.Component {
  render() {
    const { children, loading } = this.props; // You can replace it to your authentication rule (such as check token exists)
    // 你可以把它替换成你自己的登录认证规则（比如判断 token 是否存在）
    const queryString = stringify({
      redirect: window.location.href,
    });
    return children;
  }
}

export default connect(({ loading }) => ({
  loading: loading.models.user,
}))(SecurityLayout);
