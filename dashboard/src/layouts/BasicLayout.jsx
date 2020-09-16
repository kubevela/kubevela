/**
 * Ant Design Pro v4 use `@ant-design/pro-layout` to handle Layout.
 * You can view component api by:
 * https://github.com/ant-design/ant-design-pro-layout
 */
import ProLayout from '@ant-design/pro-layout';
import React, { useEffect, useState } from 'react';
import { Link, useIntl, connect, history } from 'umi';
import RightContent from '@/components/GlobalHeader/RightContent';
import {
  MenuOutlined,
  BranchesOutlined,
  ApartmentOutlined,
  DeploymentUnitOutlined,
  SettingOutlined,
} from '@ant-design/icons';
import _ from 'lodash';

const AddIcon = (menuData) => {
  return menuData.map((item) => {
    const name = _.get(item, 'name', '');
    if (name) {
      if (name === 'Workload') {
        // eslint-disable-next-line no-param-reassign
        item.icon = <ApartmentOutlined />;
      } else if (name === 'Traits') {
        // eslint-disable-next-line no-param-reassign
        item.icon = <BranchesOutlined />;
      } else if (name === 'Capability') {
        // eslint-disable-next-line no-param-reassign
        item.icon = <DeploymentUnitOutlined />;
      } else if (name === 'System') {
        // eslint-disable-next-line no-param-reassign
        item.icon = <SettingOutlined />;
      } else {
        // eslint-disable-next-line no-param-reassign
        item.icon = <MenuOutlined />;
      }
    }
    return item;
  });
};

const BasicLayout = (props) => {
  const { settings, dispatch, menus } = props;
  const [currentSelectKeys, setCurrentSelectedKeys] = useState('');
  const getCurrentSelectKeys = () => {
    const pathnameCur = props.history.location.pathname;
    if (pathnameCur) {
      if (pathnameCur.includes('Application')) {
        setCurrentSelectedKeys(['applist']);
      } else if (pathnameCur.includes('Capability')) {
        setCurrentSelectedKeys(['Capability']);
      } else if (pathnameCur.includes('System/Env')) {
        setCurrentSelectedKeys(['Env']);
      } else if (pathnameCur.includes('Workload')) {
        const arr = pathnameCur.split('/');
        const key = arr[arr.length - 1];
        setCurrentSelectedKeys([key]);
      } else if (pathnameCur.includes('Traits')) {
        const arr = pathnameCur.split('/');
        const key = arr[arr.length - 1];
        setCurrentSelectedKeys([key]);
      }
    }
  };
  useEffect(() => {
    if (dispatch) {
      dispatch({
        type: 'menus/getMenuData',
      });
    }
    props.history.listen((route) => {
      getCurrentSelectKeys(route.pathname);
    });
    // setCurrentSelectedKeys('applist')
  }, []);

  const { formatMessage } = useIntl();
  return (
    <ProLayout
      formatMessage={formatMessage}
      onMenuHeaderClick={() => history.push('/')}
      menuItemRender={(menuItemProps, defaultDom) => {
        if (menuItemProps.isUrl || !menuItemProps.path) {
          return defaultDom;
        }
        // return <Link to={menuItemProps.path}>{defaultDom}</Link>;
        return (
          <div
            onClick={() => {
              setCurrentSelectedKeys([menuItemProps.key]);
              history.push(menuItemProps.path);
            }}
          >
            {defaultDom}
          </div>
        );
      }}
      selectedKeys={currentSelectKeys}
      breadcrumbRender={(routers = []) => [
        {
          path: '/',
          breadcrumbName: formatMessage({
            id: 'menu.home',
          }),
        },
        ...routers,
      ]}
      itemRender={(route, params, routes, paths) => {
        const first = routes.indexOf(route) === 0;
        return first ? (
          <Link to={paths.join('/')}>{route.breadcrumbName}</Link>
        ) : (
          <span>{route.breadcrumbName}</span>
        );
      }}
      // menuDataRender={menuDataRender}
      menuDataRender={() => AddIcon(menus.menuData)}
      rightContentRender={() => <RightContent />}
      {...props}
      {...settings}
    />
  );
};

export default connect(({ global, settings, menus }) => ({
  global,
  settings,
  menus,
}))(BasicLayout);
