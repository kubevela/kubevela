import React from 'react';
import Trait from '../../../components/Trait';

class TableList extends React.PureComponent {
  render() {
    const propsObj = {
      title: 'Autoscaling',
      settings: [
        {
          name: 'Max Instance',
          value: 'description,schema',
        },
        {
          name: 'Min Instance',
          value: 'description,schema',
        },
      ],
      pathname: '/ApplicationList/CreateApplication',
      state: {
        activeStep: 1,
        TraitType: 'autoscaling',
      },
      btnValue: 'Attach to',
      hrefAddress: '#',
      btnIsShow: true,
      history: this.props.history,
    };
    return <Trait propsObj={propsObj} />;
  }
}

export default TableList;
