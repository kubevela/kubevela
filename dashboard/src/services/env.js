import request from '@/utils/request';

/*
 * 初始化 env：Post /api/envs/
 */
export async function initialEnvs({ params }) {
  return request('/api/envs/', {
    method: 'post',
    data: {
      ...params,
    },
    headers: {
      'Content-Type': 'application/json',
    },
  });
}

/*
 * env 列表：Get /api/envs/
 */
export async function getEnvs() {
  return request('/api/envs/');
}

/*
 * 查询 env：Get /api/envs/:envName
 */
export async function searchEnvs({ envName }) {
  return request(`/api/envs/${envName}`);
}

/*
 * 删除 env：Delete /api/envs/:envName
 */
export async function deleteEnv({ envName }) {
  return request(`/api/envs/${envName}`, {
    method: 'delete',
  });
}

/*
 * 切换 env：Patch /api/envs/:envName
 */
export async function switchEnv({ currentEnv }) {
  return request(`/api/envs/${currentEnv}`, {
    method: 'Patch',
  });
}
