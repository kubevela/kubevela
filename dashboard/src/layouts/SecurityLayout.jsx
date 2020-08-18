import React from 'react';
import { connect } from 'umi';

class SecurityLayout extends React.Component {
  render() {
    const { children } = this.props;
    return children;
  }
}

export default connect(({ loading }) => ({
  loading: loading.models.user,
}))(SecurityLayout);
