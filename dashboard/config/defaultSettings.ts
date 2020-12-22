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
  logo: 'https://s4.aconvert.com/convert/p3r68-cdx67/ah3ui-ih2jp.svg',
  iconfontUrl: '',
};

export default Settings;
