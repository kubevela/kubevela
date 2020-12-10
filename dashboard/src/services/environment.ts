import { request } from 'umi';

const BASE_PATH = '/api/envs';

/*
 * 创建 env: post /api/envs/
 */
export async function createEnvironment(
  environment: API.Environment,
): Promise<API.VelaResponse<string>> {
  return request(`${BASE_PATH}/`, {
    method: 'post',
    data: environment,
  });
}

/*
 * env 列表: get /api/envs/
 */
export async function getEnvironments(): Promise<API.VelaResponse<API.Environment[]>> {
  return request(BASE_PATH);
}

/*
 * 删除 env: delete /api/envs/:envName
 */
export async function deleteEnvironment(envName: string): Promise<API.VelaResponse<string>> {
  return request(`${BASE_PATH}/${envName}`, {
    method: 'delete',
  });
}

/*
 * 切换 env: patch /api/envs/:envName
 */
export async function switchCurrentEnvironment(envName: string): Promise<API.VelaResponse<string>> {
  return request(`${BASE_PATH}/${envName}`, {
    method: 'patch',
  });
}

/*
 * 修改 env: put /api/envs/:envName
 */
export async function updateEnvironment(
  envName: string,
  body: API.EnvironmentBody,
): Promise<API.VelaResponse<string>> {
  return request(`${BASE_PATH}/${envName}`, {
    method: 'put',
    data: body,
  });
}
