// function getFakeCaptcha(req, res) {
//   return res.json("captcha-xxx");
// } // 代码中会兼容本地 service mock 以及部署站点的静态数据

export default {
  // 支持值为 Object 和 Array
  "GET /love/record": [{
    name: "app-foo",
    time: '2020-08-11 13:20:45',
    status: 1,
    workLoad: {
      name: 'Delopyment'
    },
    traits: []
  },{
    name: "app-foo1",
    time: '2020-08-11 13:20:45',
    status: 1,
    workLoad: {
      name: 'Delopyment'
    },
    traits: [{
      name: 'Autoscaling',
      id:'1'
    }]
  },{
    name: "app-foo2",
    time: '2020-08-11 13:20:45',
    status: 2,
    workLoad: {
      name: 'Delopyment'
    },
    traits: [{
      name: 'Autoscaling',
      id:'1'
    },{
      name: 'Rollout',
      id:'2'
    },]
  },{
    name: "app-foo3",
    time: '2020-08-11 13:20:45',
    status: 1,
    workLoad: {
      name: 'Delopyment'
    },
    traits: [{
      name: 'Autoscaling',
      id:'1'
    }]
  }]
};
