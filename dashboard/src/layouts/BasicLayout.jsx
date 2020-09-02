/**
 * Ant Design Pro v4 use `@ant-design/pro-layout` to handle Layout.
 * You can view component api by:
 * https://github.com/ant-design/ant-design-pro-layout
 */
import ProLayout from '@ant-design/pro-layout';
import React, { useEffect } from 'react';
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

// const menuDataRender = (menuList) => {
//   return menuList.map((item) => {
//     const localItem = {
//       ...item,
//       children: item.children ? menuDataRender(item.children) : undefined,
//     };
//     return localItem;
//   });
// };
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
  useEffect(() => {
    if (dispatch) {
      // dispatch({
      //     type: 'user/fetchCurrent',
      // });
      // dispatch({
      //     type: 'settings/getSetting',
      // });
      dispatch({
        type: 'menus/getMenuData',
      });
    }
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

        return <Link to={menuItemProps.path}>{defaultDom}</Link>;
      }}
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
