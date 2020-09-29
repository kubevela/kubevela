import { Form, Input, Select, Button, Popconfirm } from 'antd';
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

  validateFields = () => {
    return this.formRefStep2.current.validateFields();
  };

  traitSelectChange = async (value, isType = 1) => {
    if (value) {
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
      } else {
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
    }
  };

  confirm = () => {
    this.props.deleteTraitItem(this.props.uniq);
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
        <div style={{ border: '1px solid #eee', margin: '16px 0px 8px' }}>
          <div style={{ padding: '16px 48px 0px 16px' }}>
            <Form.Item
              name="name"
              label="Trait"
              rules={[{ required: true, message: 'Please Select a Trait!' }]}
            >
              <Select placeholder="Select a Trait" onChange={this.traitSelectChange}>
                {availableTraitList.map((item) => {
                  return (
                    <Option value={item.name} key={item.name}>
                      {item.name}
                    </Option>
                  );
                })}
              </Select>
            </Form.Item>
            <Form.Item label="Properties" style={{ marginLeft: '-10px' }} />
          </div>
          <div className="relativeBox">
            {this.state.parameters ? (
              this.state.parameters.map((item) => {
                return item.type === 4 ? (
                  <Form.Item
                    name={item.name}
                    label={item.name}
                    key={item.name}
                    rules={[
                      {
                        required: item.required || false,
                        message: `Please input ${item.name} !`,
                      },
                      { pattern: /^[0-9]*$/, message: `${item.name} only use digits(0-9).` },
                    ]}
                  >
                    <Input />
                  </Form.Item>
                ) : (
                  <Form.Item
                    name={item.name}
                    label={item.name}
                    key={item.name}
                    rules={[
                      {
                        required: item.required || false,
                        message: `Please input ${item.name} !`,
                      },
                      { pattern: /^[^\s]*$/, message: 'Spaces are not allowed!' },
                    ]}
                  >
                    <Input />
                  </Form.Item>
                );
              })
            ) : (
              <></>
            )}
          </div>
          <div style={{ padding: '0px 48px 16px 0px', clear: 'both', height: '52px' }}>
            <Popconfirm
              title="Are you sure delete this trait?"
              onConfirm={this.confirm}
              okText="Yes"
              cancelText="No"
            >
              <Button
                danger
                style={{ float: 'right' }}
                onClick={(e) => {
                  e.stopPropagation();
                }}
              >
                Delete
              </Button>
            </Popconfirm>
          </div>
        </div>
      </Form>
    );
  }
}
