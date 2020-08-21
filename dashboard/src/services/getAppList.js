import request from '@/utils/request';

export async function getapplist({ url }) {
  return request(url);
}
export async function createApp({ params, url }) {
  return request(url, {
    method: 'POST',
    // body:JSON.stringify(params),
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}
export async function getEnvs() {
  return request('/api/envs/');
}
