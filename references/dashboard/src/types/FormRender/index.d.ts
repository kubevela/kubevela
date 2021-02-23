declare module 'form-render/lib/antd' {
import React from 'react';

    export interface FRProps {
    schema: object;
    formData?: object;
    onChange?: (data?: object) => void;
    onMount?: () => {};
    name?: string;
    column?: number;
    uiSchema?: object;
    widgets?: any;
    FieldUI?: any;
    fields?: any;
    mapping?: object;
    showDescIcon?: boolean;
    showValidate?: boolean;
    displayType?: string;
    onValidate?: any;
    readOnly?: boolean;
    labelWidth?: number | string;
  }
  const FormRender: React.FC<FRProps> = () => null;
  export default FormRender;
}
