import React, { useEffect, useState } from 'react';

import FormRender from 'form-render/lib/antd';

import { getCapabilityOpenAPISchema } from '@/services/capability';

import CapabilitySelector, { CapabilitySelectorProps } from '../CapabilitySelector';
import FormGroup from '../FormGroup';

export interface CapabilityFormItemData {
  capabilityType: string;
  data: object;
}
interface CapabilityFormItemProps extends CapabilitySelectorProps {
  onChange?: (currentData: CapabilityFormItemData, oldData?: CapabilityFormItemData) => void;
  onValidate?: (errorFields: { [field: string]: any }) => void;
}
const CapabilityFormItem: React.FC<CapabilityFormItemProps> = ({
  capability,
  onChange,
  onSelect,
  disableCapabilities,
  onValidate,
}) => {
  const [schema, setSchema] = useState<object>();
  const [value, setValue] = useState<string>();
  const [data, setData] = useState<CapabilityFormItemData>();
  useEffect(() => {
    if (value == null) {
      return;
    }
    getCapabilityOpenAPISchema(value).then((r) => setSchema(JSON.parse(r.data)));
  }, [value]);
  return (
    <div>
      <CapabilitySelector
        capability={capability}
        onSelect={(name) => {
          setValue(name);
          if (onSelect != null) {
            onSelect(name);
          }
        }}
        disableCapabilities={disableCapabilities}
      />
      {schema == null ? null : (
        <div style={{ marginTop: '10px' }}>
          <FormGroup title={value}>
            <FormRender
              schema={schema}
              formData={data?.data ?? {}}
              onChange={(fd) => {
                const newData = { capabilityType: value as string, data: fd ?? {} };
                setData(newData);
                if (onChange != null) {
                  onChange(newData, data);
                }
              }}
              displayType="column"
              onValidate={
                onValidate == null
                  ? null
                  : (errorFields: string[]) => {
                      const map = {};
                      errorFields.forEach((f) => {
                        map[`${value}.${f}`] = f;
                      });
                      onValidate(map);
                    }
              }
            />
          </FormGroup>
        </div>
      )}
    </div>
  );
};

export default CapabilityFormItem;
