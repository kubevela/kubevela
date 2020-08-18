// function getFakeCaptcha(req, res) {
//   return res.json("captcha-xxx");
// } // 代码中会兼容本地 service mock 以及部署站点的静态数据

export default {
  // 支持值为 Object 和 Array
  'GET /api/envs/default/apps': [
    {
      created: '2020-08-13 14:53:02 +0800 CST',
      name: 'poc2040',
      status: 'UNKNOWN',
      traits: ['manualscaler'],
      workload: 'ContainerizedWorkload',
    },
    {
      created: '2020-08-13 14:53:02 +0800 CST',
      name: 'poc2042',
      status: 'True',
      traits: ['manualscaler', 'manualscaler1'],
      workload: 'ContainerizedWorkload',
    },
    {
      created: '2020-08-13 14:53:02 +0800 CST',
      name: 'poc2042',
      status: 'False',
      traits: ['manualscaler', 'manualscaler1'],
      workload: 'ContainerizedWorkload',
    },
  ],
};
