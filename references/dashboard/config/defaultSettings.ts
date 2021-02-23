import { Settings as LayoutSettings } from '@ant-design/pro-layout';

import themeSettings from './themeSettings';

const Settings: LayoutSettings & {
  pwa?: boolean;
  logo?: string;
} = {
  navTheme: 'dark',
  primaryColor: themeSettings.primaryColor, // 全局主色
  layout: 'mix',
  contentWidth: 'Fluid',
  fixedHeader: false,
  fixSiderbar: true,
  colorWeak: false,
  title: 'KubeVela',
  pwa: false,
  logo: 'https://raw.githubusercontent.com/oam-dev/kubevela/master/docs/resources/KubeVela-04.png',
  iconfontUrl: '',
};

export default Settings;
