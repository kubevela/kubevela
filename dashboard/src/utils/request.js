/**
 * request 网络请求工具
 * 更详细的 api 文档: https://github.com/umijs/umi-request
 */
import { extend } from 'umi-request';
import { message } from 'antd';

const codeMessage = {
  200: '服务器成功返回请求的数据。',
  201: '新建或修改数据成功。',
  202: '一个请求已经进入后台排队（异步任务）。',
  204: '删除数据成功。',
  400: '发出的请求有错误，服务器没有进行新建或修改数据的操作。',
  401: '用户没有权限（令牌、用户名、密码错误）。',
  403: '用户得到授权，但是访问是被禁止的。',
  404: '发出的请求针对的是不存在的记录，服务器没有进行操作。',
  406: '请求的格式不可得。',
  410: '请求的资源被永久删除，且不会再得到的。',
  422: '当创建一个对象时，发生一个验证错误。',
  500: '服务器发生错误，请检查服务器。',
  502: '网关错误。',
  503: '服务不可用，服务器暂时过载或维护。',
  504: '网关超时。',
};
/**
 * 异常处理程序
 */

const errorHandler = async (error) => {
  const { response } = error;
  if (response && response.status) {
    const errorText = codeMessage[response.status] || response.statusText;
    const { status, url } = response;
    message.error(`请求错误 ${status}:${url} ${errorText}`);
  } else if (!response) {
    message.error('网络异常: 您的网络发生异常，无法连接服务器');
  }

  // throw error; // 如果throw. 错误将继续抛出.
  // 如果return, 则将值作为返回. 'return;' 相当于return undefined, 在处理结果时判断response是否有值即可.
  // return {some: 'data'};
  // return response;
};

/**
 * 配置request请求时的默认参数
 */
const request = extend({
  errorHandler, // 默认错误处理
  credentials: 'include', // 默认请求是否带上cookie,
  // prefix: '/api/v1',
  // timeout: 1000,
});

/*
 *  1) code == 500
 *     data 为报错字符串
 *  2) code == 200
 *     如果 method = Get，data 类型 =  list/json
 *     否则，data 类型 =  string，存储的是操作成功的信息
 */
request.interceptors.response.use(async (response) => {
  if (response.status !== 200 && response.status !== 500) {
    errorHandler({ response });
    return;
  }
  const data = await response.clone().json();
  if (data && data.error) {
    message.error(`请求错误${response.status} ：${data.error}`);
    return;
    // eslint-disable-next-line no-else-return
  } else if (data && data.code === 500) {
    message.error(`请求错误${response.status} ：${data.data}`);
    return;
  }
  // eslint-disable-next-line consistent-return
  return data.data;
  // return response;
});
export default request;
