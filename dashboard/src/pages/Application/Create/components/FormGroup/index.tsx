import React from 'react';

import { Card } from 'antd';

const FormGroup: React.FC<{ title: React.ReactNode }> = ({ title, children }) => {
  return <Card title={title}>{children}</Card>;
};
export default FormGroup;
