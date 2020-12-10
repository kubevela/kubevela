import React from 'react';

import { Menu, message, Spin, Typography } from 'antd';
import classNames from 'classnames';
import { useModel } from 'umi';

import { DownOutlined } from '@ant-design/icons';

import HeaderDropdown from '../HeaderDropdown';
import styles from './index.less';

export default () => {
  const {
    environments,
    currentEnvironment,
    switchCurrentEnvironment: switchEnvironment,
  } = useModel('useEnvironmentModel');

  const menu = (
    <Menu
      className={styles.menu}
      selectedKeys={currentEnvironment == null ? undefined : [currentEnvironment.envName]}
      onClick={(e) => {
        switchEnvironment(e.key.toString()).then((env) => {
          if (env == null) {
            return;
          }
          message.success({
            content: `Set environment succeed, current environment is ${env.envName}, namespace is ${env.namespace}`,
            key: 'switchEnvironment',
          });
        });
      }}
    >
      {environments &&
        environments.map((item) => {
          return (
            <Menu.Item key={item.envName} title={item.namespace}>
              <div>
                <Typography.Text>{item.envName}</Typography.Text>
              </div>
              <div>
                <Typography.Text type="secondary">
                  <small>{item.namespace}</small>
                </Typography.Text>
              </div>
            </Menu.Item>
          );
        })}
    </Menu>
  );

  return (
    <HeaderDropdown overlay={menu}>
      <div className={`${styles.action}`}>
        {environments == null || currentEnvironment == null ? (
          <Spin size="small" />
        ) : (
          <>
            <span className={`${styles.name} anticon`}>{currentEnvironment?.envName}</span>
            <DownOutlined style={{ marginLeft: '5px' }} />
          </>
        )}
      </div>
    </HeaderDropdown>
  );
};
