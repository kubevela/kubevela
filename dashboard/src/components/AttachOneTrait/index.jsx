import { Form, Input, Select } from 'antd';
import { connect } from 'dva';
import React from 'react';
import _ from 'lodash';

const { Option } = Select;
const layout = {
  labelCol: {
    span: 8,
  },
  wrapperCol: {
    span: 16,
  },
};

@connect(() => ({}))
export default class CreateTraitItem extends React.PureComponent {
  formRefStep2 = React.createRef();

  constructor(props) {
    super(props);
    this.state = {
      parameters: [],
    };
  }

  componentDidMount() {
    this.props.onRef(this);
    if (this.props.initialValues && this.props.initialValues.name) {
      this.traitSelectChange(this.props.initialValues.name, 2);
    }
  }

  getSelectValue = () => {
    return this.formRefStep2.current.getFieldsValue();
  };

  setDefaultValue = (traitType) => {
    this.formRefStep2.current.setFieldsValue({ name: traitType });
    this.traitSelectChange(traitType);
  };

  traitSelectChange = async (value, isType = 1) => {
    const res = await this.props.dispatch({
      type: 'trait/getTraitByName',
      payload: {
        traitName: value,
      },
    });
    this.setState({
      parameters: res.parameters,
    });
    if (isType === 2) {
      this.formRefStep2.current.setFieldsValue(this.props.initialValues);
    } else if (isType) {
      // 进行默认值填写
      const parameters = _.get(res, 'parameters', []);
      if (parameters.length) {
        const initialObj = {};
        parameters.forEach((item) => {
          if (item.default) {
            initialObj[item.name] = item.default;
          }
        });
        this.formRefStep2.current.setFieldsValue(initialObj);
      }
    }
  };

  render() {
    const { availableTraitList } = this.props;
    return (
      <Form
        labelAlign="left"
        {...layout}
        ref={this.formRefStep2}
        name="control-ref"
        className="traitItem"
      >
        <Form.Item name="name" label="Trait">
          <Select placeholder="Select a Trait" allowClear onChange={this.traitSelectChange}>
            {availableTraitList.map((item) => {
              return (
                <Option value={item.name} key={item.name}>
                  {item.name}
                </Option>
              );
            })}
          </Select>
        </Form.Item>
        <Form.Item label="Properties" />
        <div className="relativeBox">
          {this.state.parameters ? (
            this.state.parameters.map((item) => {
              return (
                <Form.Item name={item.name} label={item.name} key={item.name}>
                  <Input />
                </Form.Item>
              );
            })
          ) : (
            <></>
          )}
        </div>
      </Form>
    );
  }
}
