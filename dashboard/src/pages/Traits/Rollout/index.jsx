import React from 'react';
import Trait from '../../../components/Trait';

class TableList extends React.PureComponent {
  render() {
    const propsObj = {
      title: 'Rollout',
      settings: [
        {
          name: 'Strategy',
          value: 'description,schema',
        },
        {
          name: 'Step',
          value: 'description,schema',
        },
      ],
      pathname: '/ApplicationList/CreateApplication',
      state: {
        activeStep: 1,
        TraitType: 'rollout',
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
